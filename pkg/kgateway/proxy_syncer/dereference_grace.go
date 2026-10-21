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
//
// Only a cluster scoping stopped emitting is graced: translated must still
// have it (nil translated means unknown, and graces every one). A cluster that
// is no longer translated is gone, not de-referenced: its backend was deleted
// or failed. With scoping off it would leave at once, and keeping it would keep
// sending traffic to that backend, which a request-time selector would still
// pick by name. Callers must hold the gate lock. A nonpositive grace disables
// retention.
//
// A window opens when the routes that stop naming the cluster are published.
// holding means this publish keeps the routing types at their published
// versions (a held flip or a reference-ahead hold), so it opens no window:
// timed from the held build, the hold would use up the grace, and the cluster
// could leave right behind the route update. Every hold keeps all published
// clusters, so nothing is lost meanwhile. Windows already open keep running.
func (g *publishGate) graceDereferencedClustersLocked(
	proxyKey string,
	published envoycache.ResourceSnapshot,
	building envoycache.Resources,
	translated map[string]envoycachetypes.ResourceWithTTL,
	holding bool,
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
		if _, stillTranslated := translated[name]; translated != nil && !stillTranslated {
			// Gone: removed at once, and a later return is a fresh addition.
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
			if holding {
				continue // the hold keeps it; its window opens when the routes ship
			}
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

// fireDereferencePrune re-publishes the client's latest build when a grace
// window closes, under g.mu. A callback whose timer was re-armed or cancelled,
// or whose state was replaced, does nothing.
//
// It goes through publishBuildLocked, so the prune repeats a new build's full
// hold decision. A grace expiring must not release a flip held for an unready
// cluster or an open reference-ahead window. Expired clusters drop out and the
// prune is re-armed for any window still open.
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
	if g.connectionSnapshotLocked(cache, proxyKey) == nil {
		g.cancelDereferenceTimerLocked(proxyKey)
		return
	}
	logger.Debug("de-reference grace elapsed; re-publishing the latest build without the expired clusters", "proxy_key", proxyKey)
	if err := g.publishBuildLocked(ctx, cache, state.wrap); err != nil {
		logPublishFailure(proxyKey, err)
	}
}

// publishBuildLocked publishes a stored build through the path a new build of
// its kind takes, so timer re-publishes make the same hold decision. Callers
// must hold g.mu.
func (g *publishGate) publishBuildLocked(ctx context.Context, cache envoycache.SnapshotCache, wrap XdsSnapWrapper) error {
	if wrap.deferred() {
		return g.resolveDeferredLocked(ctx, cache, wrap)
	}
	return g.publishWithTransitionGracesLocked(ctx, cache, wrap)
}

// retainsDereferenced reports whether the grace is on; if not, coherent builds
// take the plain publish path.
func (g *publishGate) retainsDereferenced() bool { return g.dereferenceGrace > 0 }

// publishWithTransitionGraces publishes a coherent build, carrying
// de-referenced clusters through their grace and holding configuration that
// names a cluster new to the published snapshot for its reference-ahead
// window. A transition alone leaves nothing missing, so it never reaches
// deferred resolution.
func (g *publishGate) publishWithTransitionGraces(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	// beginBuildLocked is for new builds; timers re-publishing a stored build
	// call the Locked form.
	if !g.beginBuildLocked(snapWrap) {
		return nil
	}
	return g.publishWithTransitionGracesLocked(ctx, cache, snapWrap)
}

// publishWithTransitionGracesLocked is publishWithTransitionGraces for callers
// that hold g.mu.
func (g *publishGate) publishWithTransitionGracesLocked(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
) error {
	published := g.connectionSnapshotLocked(cache, snapWrap.proxyKey)
	if published == nil {
		// Nothing published on this connection: nothing to carry or grace.
		return g.publishLocked(ctx, cache, snapWrap.proxyKey, snapWrap.snap, snapWrap.synthesizedEndpoints, snapWrap.endpointHashes)
	}

	// Routes, listeners and secrets stay published while the new cluster's CDS
	// goes out; the flip-release timer sends them when the window closes.
	now := time.Now()
	newlyEmitted, aheadFor := g.referenceAheadLocked(snapWrap.proxyKey, snapWrap, published, now)
	graced, soonest := g.graceDereferencedClustersLocked(
		snapWrap.proxyKey, published, snapWrap.snap.Resources[envoycachetypes.Cluster], snapWrap.translatedClusters,
		len(newlyEmitted) > 0, now)

	if len(graced) == 0 && len(newlyEmitted) == 0 {
		return g.publishLocked(ctx, cache, snapWrap.proxyKey, snapWrap.snap, snapWrap.synthesizedEndpoints, snapWrap.endpointHashes)
	}

	var publishErr error
	if len(newlyEmitted) > 0 {
		held, edsHashes := g.referenceAheadHoldLocked(snapWrap, published, graced)
		publishErr = g.publishHeldLocked(ctx, cache, snapWrap, held, nil, newlyEmitted, aheadFor, edsHashes)
	} else {
		resolved, _, edsHashes := resolveDeferredPerCluster(snapWrap, published, false, graced, g.lastPublishedEDSLocked(snapWrap.proxyKey))
		// publishLocked cancels a pending first publish or flip release, whose
		// timer would otherwise re-publish an older build.
		publishErr = g.publishLocked(ctx, cache, snapWrap.proxyKey, resolved, snapWrap.synthesizedEndpoints, edsHashes)
	}
	if publishErr != nil {
		return publishErr
	}
	g.armDereferenceTimerLocked(ctx, cache, snapWrap.proxyKey, snapWrap, soonest)
	return nil
}
