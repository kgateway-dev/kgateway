package proxy_syncer

import (
	"context"
	"time"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
)

// dereferenceState is one client's record of when each published cluster left
// the emitted set. It lives under the publish gate's lock, alongside the other
// per-client timed publication state.
type dereferenceState struct {
	// since is keyed by cluster name. An entry exists only while the cluster is
	// published but no longer emitted; re-referencing deletes it, so a route
	// that flaps keeps its cluster continuously rather than oscillating.
	since map[string]time.Time
	// wrap is the latest snapshot for this client, re-resolved when a window
	// closes; nothing else recomputes it merely because time passed.
	wrap  XdsSnapWrapper
	timer *time.Timer
	// epoch identifies the latest arming. A callback whose timer was
	// re-armed or cancelled after it started acts only if both its state and
	// its epoch are still current.
	epoch uint64
}

// graceDereferencedClusters decides which of the currently-published clusters a
// build has stopped emitting should still be published, and reports when the
// oldest of them expires.
//
// Removing a de-referenced cluster the moment the last route stops naming it is
// unsafe in the opposite direction from adding one. Envoy is delivered CDS
// before RDS, so the cluster would go out before the route that still targets
// it, and requests in that window get 503 NC. Retaining it briefly makes the
// emitted set "referenced now, plus recently de-referenced", which puts the
// route update first without depending on delivery order at all.
//
// The window is the removal-side fix regardless of how CDS and RDS are ordered
// on the wire, which is why it is not replaced by ordered ADS: ordered ADS
// fixes CDS before RDS, and that is exactly the wrong order here.
//
// Callers must hold the gate lock. A zero grace disables retention, and a
// client with no state allocates none.
func (g *publishGate) graceDereferencedClustersLocked(
	proxyKey string,
	published envoycache.ResourceSnapshot,
	building envoycache.Resources,
	now time.Time,
) (map[string]struct{}, time.Duration) {
	if g.dereferenceGrace <= 0 {
		return nil, 0
	}

	publishedClusters := published.GetResourcesAndTTL(envoyresourcev3.ClusterType)
	state := g.dereferenced[proxyKey]
	if state != nil {
		// A record outlives its cluster when the carry declined to keep it (it
		// errored, and errored clusters are never resurrected): the cluster is
		// gone from the client, but its record would keep the state, and the
		// timer that re-publishes state.wrap, alive past later publishes that
		// never refreshed it, and would deny the cluster's next genuine
		// de-reference its window. Records exist only for published clusters.
		for name := range state.since {
			if _, stillPublished := publishedClusters[name]; !stillPublished {
				delete(state.since, name)
			}
		}
	}

	var graced map[string]struct{}
	var soonest time.Duration
	for name := range publishedClusters {
		if _, stillEmitted := building.Items[name]; stillEmitted {
			// Re-referenced (or never de-referenced): drop any record, so the
			// next de-reference starts a fresh window rather than inheriting an
			// expired one.
			if state != nil {
				delete(state.since, name)
			}
			continue
		}

		if state == nil {
			state = &dereferenceState{since: make(map[string]time.Time)}
			g.dereferenced[proxyKey] = state
		}
		left, recorded := state.since[name]
		if !recorded {
			left = now
			state.since[name] = left
		}

		remaining := g.dereferenceGrace - now.Sub(left)
		if remaining <= 0 {
			// Grace elapsed: stop publishing it, and forget it so a later
			// re-reference is a fresh addition rather than an expired removal.
			delete(state.since, name)
			continue
		}
		if graced == nil {
			graced = make(map[string]struct{})
		}
		graced[name] = struct{}{}
		if soonest == 0 || remaining < soonest {
			soonest = remaining
		}
	}

	if state != nil && len(state.since) == 0 {
		g.cancelDereferenceTimerLocked(proxyKey)
	}
	return graced, soonest
}

// armDereferenceTimerLocked schedules the re-publish that actually removes a
// graced cluster. Without it, a cluster whose grace expires would linger until
// something else about the client changed: nothing recomputes a client's
// snapshot merely because time passed.
//
// The timer is re-armed to the soonest remaining expiry on every publish, so a
// second de-reference during an open window does not extend the first one's.
// Callers must hold the gate lock.
func (g *publishGate) armDereferenceTimerLocked(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	proxyKey string,
	wrap XdsSnapWrapper,
	in time.Duration,
) {
	state := g.dereferenced[proxyKey]
	if state == nil || in <= 0 {
		return
	}
	state.wrap = wrap
	if state.timer != nil {
		state.timer.Stop()
	}
	state.epoch++
	epoch := state.epoch
	state.timer = time.AfterFunc(in, func() {
		g.fireDereferencePrune(ctx, cache, proxyKey, state, epoch)
	})
}

func (g *publishGate) cancelDereferenceTimerLocked(proxyKey string) {
	if state := g.dereferenced[proxyKey]; state != nil {
		if state.timer != nil {
			state.timer.Stop()
		}
		delete(g.dereferenced, proxyKey)
	}
}

// fireDereferencePrune re-publishes the client's latest snapshot now that a
// grace window has closed, under the same lock as every other publication, so
// pruning cannot race a coherent publish. It acts only for the arming that
// scheduled it: a callback already waiting on the lock when its timer was
// re-armed or cancelled, or its state replaced, does nothing.
func (g *publishGate) fireDereferencePrune(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, state *dereferenceState, epoch uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.dereferenced[proxyKey] != state || state.epoch != epoch {
		return // superseded: re-armed, cancelled, or every graced cluster was re-referenced
	}
	if state.wrap.snap == nil {
		// Defensive: every arming stores the build it re-publishes.
		g.cancelDereferenceTimerLocked(proxyKey)
		return
	}
	published := g.connectionSnapshotLocked(cache, proxyKey)
	if published == nil {
		g.cancelDereferenceTimerLocked(proxyKey)
		return
	}

	wrap := state.wrap
	graced, soonest := g.graceDereferencedClustersLocked(
		proxyKey, published, wrap.snap.Resources[envoycachetypes.Cluster], time.Now())
	// Keep a held flip held: its own bound (the flip release) decides when it
	// goes out. Re-resolving with holds disabled would release it at this
	// grace instead, onto a cluster that may still be unready.
	holding := g.pendingFlips[proxyKey] != nil
	resolved, _, edsHashes := resolveDeferredPerCluster(wrap, published, holding, graced, g.lastPublishedEDSLocked(proxyKey))
	logger.Debug("de-reference grace elapsed; re-publishing without the expired clusters",
		"proxy_key", proxyKey, "still_graced", len(graced))
	if err := g.setSnapshot(ctx, cache, proxyKey, resolved, wrap.synthesizedEndpoints, edsHashes); err != nil {
		logPublishFailure(proxyKey, err)
		return
	}
	g.armDereferenceTimerLocked(ctx, cache, proxyKey, wrap, soonest)
}

// retainsDereferenced reports whether any cluster can outlive its last
// reference. False is the whole feature switched off, and the coherent publish
// path stays exactly as it was.
func (g *publishGate) retainsDereferenced() bool { return g.dereferenceGrace > 0 }

// publishWithTransitionGraces publishes a coherent snapshot with both emitted-set
// transitions made safe: clusters whose de-reference window is still open are
// carried forward, and a route update that retargets onto a cluster the client
// has never been sent is held back until that cluster has had time to land.
//
// Both transitions produce coherent builds -- nothing is missing, the emitted
// set simply changed -- so neither reaches the deferred resolution path that
// handles unready clusters. This is their equivalent.
func (g *publishGate) publishWithTransitionGraces(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	// A new build, like those publish and resolveDeferred take.
	if !g.beginBuildLocked(snapWrap) {
		return nil
	}

	published := g.connectionSnapshotLocked(cache, snapWrap.proxyKey)
	if published == nil {
		// Never published: there is nothing to carry forward, and nothing can
		// have been de-referenced yet.
		return g.publishLocked(ctx, cache, snapWrap.proxyKey, snapWrap.snap, snapWrap.synthesizedEndpoints, snapWrap.endpointHashes)
	}

	graced, soonest := g.graceDereferencedClustersLocked(
		snapWrap.proxyKey, published, snapWrap.snap.Resources[envoycachetypes.Cluster], time.Now())

	// The addition side rides the same path, for the same reason: a retarget
	// onto a cluster the client has never been sent is a coherent build too.
	// Holding routes, listeners and secrets at their published versions
	// publishes the new cluster's CDS by itself, and the existing flip-release
	// timer sends the route once the window closes.
	newlyEmitted, _ := g.referenceAheadLocked(snapWrap.proxyKey, snapWrap, published, time.Now())

	if len(graced) == 0 && len(newlyEmitted) == 0 {
		return g.publishLocked(ctx, cache, snapWrap.proxyKey, snapWrap.snap, snapWrap.synthesizedEndpoints, snapWrap.endpointHashes)
	}

	resolved, _, edsHashes := resolveDeferredPerCluster(snapWrap, published, false, graced, g.lastPublishedEDSLocked(snapWrap.proxyKey))
	var publishErr error
	if len(newlyEmitted) > 0 {
		held := holdRoutingTypes(resolved, published)
		recordFlipHeld(snapWrap.proxyKey)
		publishErr = g.publishHeldLocked(ctx, cache, snapWrap, held, newlyEmitted, edsHashes, true)
	} else {
		// Through publishLocked like any other publish without a hold: this
		// build supersedes a pending first publish or flip release, whose
		// timer would otherwise re-publish its older build when it fires.
		publishErr = g.publishLocked(ctx, cache, snapWrap.proxyKey, resolved, snapWrap.synthesizedEndpoints, edsHashes)
	}
	if publishErr != nil {
		return publishErr
	}
	g.armDereferenceTimerLocked(ctx, cache, snapWrap.proxyKey, snapWrap, soonest)
	return nil
}
