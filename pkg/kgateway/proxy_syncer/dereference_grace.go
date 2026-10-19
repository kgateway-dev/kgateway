package proxy_syncer

import (
	"context"
	"time"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
)

// dereferenceState tracks one client's de-reference windows under the publish
// gate's lock.
type dereferenceState struct {
	// since maps each published-but-not-emitted cluster to when it left the set.
	// Re-referencing deletes the entry, so a flapping route keeps its cluster.
	since map[string]time.Time
	// wrap is the latest build to re-resolve at expiry; time alone produces no KRT event.
	wrap  XdsSnapWrapper
	timer *time.Timer
	// epoch identifies the latest arming. Timer.Stop cannot recall a started
	// callback, so a callback acts only if its state and epoch are current.
	epoch uint64
}

// graceDereferencedClustersLocked returns the published clusters this build
// stopped emitting that are still in their grace, and the time to the soonest
// expiry. Removing one at once can let CDS remove it before RDS updates the
// route still targeting it, causing 503 NC. The grace gives the route update
// time to land whatever the delivery order; elapsing does not confirm Envoy
// applied it. Ordered ADS cannot replace the grace: it sends CDS first.
// Callers must hold the gate lock. A nonpositive grace disables retention.
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
		// Records exist only for published clusters. A leftover record for a
		// dropped (errored) cluster would keep the timer re-publishing a stale
		// state.wrap and deny the cluster's next de-reference its window.
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
			// Still emitted: drop any record, so a later de-reference starts fresh.
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
			// Expired: drop it; a later re-reference is a fresh addition.
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

// armDereferenceTimerLocked schedules the re-publish that removes expired
// graced clusters; time passing triggers nothing else. Each publish that graces
// re-arms it to the soonest expiry, so later de-references cannot extend
// earlier windows. Callers must hold the gate lock.
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

// retainsDereferenced reports whether the grace is on; if not, coherent builds
// take the plain publish path.
func (g *publishGate) retainsDereferenced() bool { return g.dereferenceGrace > 0 }

// publishWithDereferenceGrace publishes a coherent snapshot, carrying forward
// the clusters whose grace window is still open.
//
// A removal is coherent by construction, so it never reaches the deferred
// resolution path; this is the removal side's equivalent, and it holds nothing:
// routes, listeners and secrets are published as built. The only difference
// from a plain publish is that clusters which just left the emitted set, and
// their ClusterLoadAssignments, are still in the snapshot.
func (g *publishGate) publishWithDereferenceGrace(
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
		// Nothing published on this connection: nothing to carry or grace.
		return g.publishLocked(ctx, cache, snapWrap.proxyKey, snapWrap.snap, snapWrap.synthesizedEndpoints, snapWrap.endpointHashes)
	}

	graced, soonest := g.graceDereferencedClustersLocked(
		snapWrap.proxyKey, published, snapWrap.snap.Resources[envoycachetypes.Cluster], time.Now())
	if len(graced) == 0 {
		return g.publishLocked(ctx, cache, snapWrap.proxyKey, snapWrap.snap, snapWrap.synthesizedEndpoints, snapWrap.endpointHashes)
	}

	resolved, _, edsHashes := resolveDeferredPerCluster(snapWrap, published, false, graced, g.lastPublishedEDSLocked(snapWrap.proxyKey))
	// Through publishLocked like any other publish without a hold: this build
	// supersedes a pending first publish or flip release, whose timer would
	// otherwise re-publish its older build when it fires.
	if err := g.publishLocked(ctx, cache, snapWrap.proxyKey, resolved, snapWrap.synthesizedEndpoints, edsHashes); err != nil {
		return err
	}
	g.armDereferenceTimerLocked(ctx, cache, snapWrap.proxyKey, snapWrap, soonest)
	return nil
}
