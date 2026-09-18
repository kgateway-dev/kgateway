package proxy_syncer

import (
	"context"
	"sync"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// publishGate bounds how long per-client publication may be withheld while
// referenced clusters are unready. Unreadiness is not guaranteed to converge
// (#14352) — a plugin can contribute an EDS cluster whose endpoints row
// never appears, and translation backlog can stretch "briefly behind" past
// any probe window — so every withhold needs a bound. The gate owns two,
// governed by the same budget (KGW_PER_CLIENT_PUBLISH_BUDGET; 0 disables
// both):
//
//   - first publish (offerColdLocked/firePending): how long a client that
//     has NEVER been published a snapshot waits for its referenced clusters
//     to become ready;
//   - flip release (publishHeldLocked/fireFlipRelease): how long a held
//     route flip may pin the client's route/listener/secret updates at their
//     published versions while a newly-referenced cluster stays unready.
//
// First publish: the per-cluster resolution (resolveDeferred) withholds a
// deferred snapshot from a never-published client — there is no last-good
// config to hold or carry, and publishing routes to unready clusters would
// 503 them.
// That is correct for the brief converging case, but with no bound a
// permanently-unready reference leaves the pod with no listeners at all — it
// never reports Ready and crash-loops, and each restart reconnects as a new
// client that re-triggers the per-client fan-out. So the gate arms one timer
// per never-published client. If the client's inputs cohere within the
// budget, the coherent publish wins and the timer is discarded. At expiry the
// latest deferred snapshot is published as-is — it is always internally
// consistent (missing EDS references carry synthesized empty assignments), so
// the pod binds its listeners and becomes Ready while routes to still-unready
// clusters return 503 until they cohere.
//
// One exception: a client that reported a prior accepted xDS version on
// connect is warm — an Envoy that reconnected (or outlived a controller
// restart) and is still serving config from its previous stream. Publishing
// a snapshot whose routes name clusters absent from its CDS would replace
// that working config with one that NCs those routes, so warm clients stay
// withheld while any referenced cluster is MISSING (the #13868
// make-before-break property). A controller-local cache miss alone cannot
// prove a client is cold; the client-reported version can.
//
// The warm withhold deliberately does NOT extend to gaps that are only
// underived CLAs (missingEndpointsReferenced). syncXds events only fire
// after the informer chain has synced (krt registration semantics), so a CLA
// still underived a full budget later is either pathological backlog or a
// plugin that never derives endpoints for its EDS cluster — states with no
// convergence guarantee. Withholding on them would freeze the warm client
// indefinitely after a controller restart — no route change, cert rotation,
// or endpoint update would ever reach it — and would starve any cold pod
// that shares its UCC key (the prior-version mark is key-level). So at
// expiry, if every referenced cluster is present in CDS, the snapshot
// publishes with the synthesized empties as the best-known state; if the gap
// was lag after all, the very next build repairs it through per-cluster
// resolution (the cache is populated from here on).
//
// Flip release: resolveDeferredPerCluster holds routes/listeners/secrets at
// their published versions while a route flip targets a newly-referenced
// cluster that is not yet ready. The hold is per resource type, so while it
// lasts EVERY route/listener/secret update for the client is pinned — fine
// for a backend whose translation or endpoints catch up in seconds, but a
// reference that never resolves (a plugin that contributes an EDS cluster
// with no endpoints source; a cluster that never reaches CDS) would pin
// them forever. So the first held publish of an episode arms a release timer; if
// the client is still holding at expiry, the latest held wrapper is
// re-resolved with holds disabled and published: the flip goes out, routes
// to still-unready clusters fail until those clusters become ready — the
// truthful steady-state answer — and pinned updates resume. Repeated holds
// refresh the pending wrapper but do not extend the deadline; the episode
// ends when a publish without a hold goes out.
//
// All snapshot-cache mutations for gated clients go through the gate's lock,
// so an expiring timer can never overwrite a newer publish.
type publishGate struct {
	// budget is how long a withhold (first publish or held flip) may last
	// before its bounded publish; 0 disables both bounds (withhold/hold until
	// coherent, with no deadline). Written once at construction.
	budget time.Duration
	// checkConsistency runs Snapshot.Consistent() on every publish, recording
	// (never withholding on) violations — an invariant monitor for test/CI
	// environments (KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK). Written once at
	// construction.
	checkConsistency bool

	mu           sync.Mutex
	pending      map[string]*pendingFirstPublish
	pendingFlips map[string]*pendingFlipRelease
}

type pendingFirstPublish struct {
	// snap is the latest deferred snapshot; the timer publishes whatever is
	// latest at expiry.
	snap *envoycache.Snapshot
	// The latest recorded gaps: missing decides whether a warm client
	// publishes or stays withheld at expiry; both feed the expiry log.
	missing          []string
	missingEndpoints []string
	timer            *time.Timer
}

type pendingFlipRelease struct {
	// wrap is the latest deferred wrapper whose flip was held; at expiry it
	// is re-resolved against the then-published snapshot with holds disabled.
	wrap XdsSnapWrapper
	// blocking are the latest flip-blocking cluster names, for the expiry log.
	blocking []string
	timer    *time.Timer
}

func newPublishGate(budget time.Duration, checkConsistency bool) *publishGate {
	return &publishGate{
		budget:           budget,
		checkConsistency: checkConsistency,
		pending:          make(map[string]*pendingFirstPublish),
		pendingFlips:     make(map[string]*pendingFlipRelease),
	}
}

// setSnapshot is the single funnel through which the gate writes to the
// snapshot cache. With checkConsistency enabled it verifies go-control-plane's
// Snapshot.Consistent() invariant — every EDS resource matched to a CDS
// reference and every RDS resource to an LDS reference — which the publication
// paths maintain by construction; a violation is recorded and logged but the
// snapshot is still published (a publish-blocking check would reintroduce
// unbounded withholds).
func (g *publishGate) setSnapshot(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, snap *envoycache.Snapshot) error {
	if g.checkConsistency {
		if err := snapshotConsistencyError(proxyKey, snap); err != nil {
			logger.Error("BUG: per-client snapshot failed consistency check before publish; publishing anyway",
				"proxy_key", proxyKey, "error", err)
			recordInconsistentSnapshot(proxyKey)
		}
	}
	return cache.SetSnapshot(ctx, proxyKey, snap)
}

// publish publishes a coherent (or per-cluster-resolved, no-hold) snapshot
// and cancels any pending bounded first publish and any pending flip release
// — a publish without a hold means the episode resolved — under one lock so
// a concurrently-expiring timer cannot overwrite the newer snapshot.
func (g *publishGate) publish(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, snap *envoycache.Snapshot) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.publishLocked(ctx, cache, proxyKey, snap)
}

func (g *publishGate) publishLocked(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, snap *envoycache.Snapshot) error {
	if st := g.pending[proxyKey]; st != nil {
		if st.timer != nil {
			st.timer.Stop()
		}
		delete(g.pending, proxyKey)
	}
	g.cancelFlipReleaseLocked(proxyKey)
	return g.setSnapshot(ctx, cache, proxyKey, snap)
}

// resolveDeferred decides how a deferred wrapper publishes. The cache read,
// the per-cluster resolution, and the resulting publish all happen under one
// lock acquisition so an expiring budget timer cannot interleave: a
// first-publish timer firing between an unlocked cache check and the cold
// offer would strand the newest deferred snapshot unpublished until the next
// build event, and a flip-release timer firing between an unlocked cache read
// and the held publish would be overwritten by a hold composed against the
// pre-release snapshot (re-pinning the just-released routes for another full
// budget).
func (g *publishGate) resolveDeferred(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
	hasPriorXDSVersion func(string) bool,
) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	published, err := cache.GetSnapshot(snapWrap.proxyKey)
	if err != nil {
		// Never-published client: there is no last-good to hold or carry,
		// and publishing an incoherent snapshot would 503 the unready
		// routes — so withhold (cold-start make-before-break, unchanged
		// from the whole-snapshot gate), but only up to the first-publish
		// budget: a pod with no config at all never goes Ready and
		// crash-loops, so past the budget the latest deferred snapshot is
		// published anyway. Clients that reported a prior accepted xDS
		// version are warm, not cold: they stay withheld at expiry only
		// while clusters are missing from CDS; when the only gaps are
		// underived CLAs, that is the backends' steady state (#14352) and
		// the snapshot publishes as their truth rather than freezing the
		// client indefinitely.
		g.offerColdLocked(ctx, cache, snapWrap, hasPriorXDSVersion)
		return nil
	}
	resolved, heldBlocking := resolveDeferredPerCluster(snapWrap, published, true)
	if len(heldBlocking) > 0 {
		// The held snapshot still publishes (CDS/EDS keep flowing); the
		// gate additionally arms the flip-release bound for the episode.
		return g.publishHeldLocked(ctx, cache, snapWrap, resolved, heldBlocking)
	}
	return g.publishLocked(ctx, cache, snapWrap.proxyKey, resolved)
}

// offerColdLocked records the latest deferred snapshot for a never-published
// client and arms the budget timer once per episode; the timer publishes
// whatever is latest unless the client turns out to be warm (prior xDS
// version) with clusters missing from CDS, or a coherent publish got there
// first. Callers must hold g.mu.
func (g *publishGate) offerColdLocked(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
	hasPriorXDSVersion func(string) bool,
) {
	if g.budget <= 0 {
		return // bound disabled: withhold until coherent
	}
	proxyKey := snapWrap.proxyKey
	st := g.pending[proxyKey]
	if st == nil {
		st = &pendingFirstPublish{}
		g.pending[proxyKey] = st
		logger.Info("withholding first publish until referenced clusters are ready or the budget expires",
			"proxy_key", proxyKey, "budget", g.budget,
			"missing_clusters", snapWrap.missingReferenced,
			"missing_endpoint_clusters", snapWrap.missingEndpointsReferenced,
		)
	}
	st.snap = snapWrap.snap
	st.missing = snapWrap.missingReferenced
	st.missingEndpoints = snapWrap.missingEndpointsReferenced
	if st.timer != nil {
		return // already armed; it will publish the latest snap
	}
	st.timer = time.AfterFunc(g.budget, func() {
		g.firePending(ctx, cache, proxyKey, hasPriorXDSVersion)
	})
}

// firePending runs at budget expiry: publish the latest deferred snapshot,
// unless the pending entry was canceled or drained, or the client reported a
// prior xDS version (warm reconnect) AND some referenced cluster is missing
// from CDS — publishing that would NC routes the proxy is still serving. A
// warm client
// whose only gaps are clusters with no derived CLA publishes anyway: by
// expiry that gap is the backends' steady state (#14352), and withholding
// would freeze the client indefinitely (see the type comment). The check and
// publish happen under the gate lock, so they cannot interleave with
// publish().
func (g *publishGate) firePending(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	proxyKey string,
	hasPriorXDSVersion func(string) bool,
) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.pending[proxyKey]
	if st == nil || st.snap == nil {
		return
	}
	// Keep the entry so a later deferred wrapper of the same episode re-arms
	// the timer instead of restarting the accounting.
	snap := st.snap
	st.snap = nil
	st.timer = nil
	if _, err := cache.GetSnapshot(proxyKey); err == nil {
		// Defensive: every publish path deletes or resolves the pending entry
		// under this same lock, so a published snapshot alongside a pending
		// snap should be unreachable — but publishing over it would regress a
		// newer snapshot, so bail.
		return
	}
	mode := boundedPublishFirstPublish
	if hasPriorXDSVersion != nil && hasPriorXDSVersion(proxyKey) {
		if len(st.missing) > 0 {
			logger.Warn("first-publish budget expired, but client reported a prior xDS version and referenced clusters are missing; withholding deferred snapshot",
				"proxy_key", proxyKey, "missing_clusters", st.missing, "missing_endpoint_clusters", st.missingEndpoints)
			recordDeferredWithheld(proxyKey)
			return
		}
		// Warm client, but every referenced cluster is present in CDS and
		// the only gaps are clusters with underived CLAs: by now that is
		// backlog or a plugin gap with no convergence guarantee (#14352),
		// and withholding would freeze this client's config indefinitely.
		mode = boundedPublishWarmTruth
		logger.Warn("first-publish budget expired for warm client; remaining gaps are clusters with no derived endpoints, publishing their current truth",
			"proxy_key", proxyKey, "missing_endpoint_clusters", st.missingEndpoints)
	} else {
		logger.Warn("first-publish budget expired; publishing deferred snapshot so the client can start",
			"proxy_key", proxyKey, "missing_clusters", st.missing, "missing_endpoint_clusters", st.missingEndpoints)
	}
	if err := g.setSnapshot(ctx, cache, proxyKey, snap); err != nil {
		logger.Error("failed to set xds snapshot", "proxy_key", proxyKey, "error", err)
		return
	}
	recordBoundedPublish(proxyKey, mode)
}

// publishHeldLocked publishes a snapshot whose route flip is held at the
// published versions and arms the flip-release bound once per episode. If the
// client is still holding at budget expiry, fireFlipRelease publishes the
// latest held wrapper re-resolved with holds disabled, so a newly-referenced
// cluster that never becomes ready (a steady-state-empty backend, #14352)
// pins the client's route/listener/secret updates for at most one budget
// instead of forever. budget<=0 disables the bound: the hold lasts until the
// flip resolves. Callers must hold g.mu.
func (g *publishGate) publishHeldLocked(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
	held *envoycache.Snapshot,
	blocking []string,
) error {
	proxyKey := snapWrap.proxyKey
	if err := g.setSnapshot(ctx, cache, proxyKey, held); err != nil {
		return err
	}
	if g.budget <= 0 {
		return nil // bound disabled: hold until the flip resolves
	}
	pf := g.pendingFlips[proxyKey]
	if pf == nil {
		pf = &pendingFlipRelease{}
		g.pendingFlips[proxyKey] = pf
		pf.timer = time.AfterFunc(g.budget, func() {
			g.fireFlipRelease(ctx, cache, proxyKey)
		})
		logger.Info("holding route flip; will release at budget expiry if still unready",
			"proxy_key", proxyKey, "budget", g.budget, "flip_blocking", blocking)
	}
	pf.wrap = snapWrap
	pf.blocking = blocking
	return nil
}

// fireFlipRelease runs at flip-hold budget expiry: re-resolve the latest held
// wrapper against the currently-published snapshot with holds disabled and
// publish it. The flip goes out; routes to still-unready clusters fail until
// those clusters become ready, which is the truthful steady-state answer —
// and pinned route/listener/secret updates resume flowing. Runs under the
// gate lock, so it cannot interleave with publish() or resolveDeferred().
func (g *publishGate) fireFlipRelease(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	pf := g.pendingFlips[proxyKey]
	if pf == nil {
		return // released by a no-hold publish while the timer was in flight
	}
	delete(g.pendingFlips, proxyKey)
	published, err := cache.GetSnapshot(proxyKey)
	if err != nil {
		return // nothing published to resolve against; nothing was held
	}
	released, _ := resolveDeferredPerCluster(pf.wrap, published, false)
	logger.Warn("flip-hold budget expired; publishing held route flip, routes to still-unready clusters will fail until they become ready",
		"proxy_key", proxyKey, "flip_blocking", pf.blocking)
	if err := g.setSnapshot(ctx, cache, proxyKey, released); err != nil {
		logger.Error("failed to set xds snapshot", "proxy_key", proxyKey, "error", err)
		return
	}
	recordBoundedPublish(proxyKey, boundedPublishFlipRelease)
}

func (g *publishGate) cancelFlipReleaseLocked(proxyKey string) {
	if pf := g.pendingFlips[proxyKey]; pf != nil {
		if pf.timer != nil {
			pf.timer.Stop()
		}
		delete(g.pendingFlips, proxyKey)
	}
}

// clientDeparted cancels any pending bounded first publish or flip release
// for a client whose wrapper row was deleted, so a timer cannot publish to a
// key after its client left. A still-connected client re-arms the gate with
// its next deferred wrapper.
func (g *publishGate) clientDeparted(proxyKey string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if st := g.pending[proxyKey]; st != nil {
		if st.timer != nil {
			st.timer.Stop()
		}
		delete(g.pending, proxyKey)
	}
	g.cancelFlipReleaseLocked(proxyKey)
}

// snapshotConsistencyError checks the dynamic graph together with the gateway's
// known bootstrap EDS cluster. The bootstrap cluster is absent from CDS, but its
// CLA is intentionally published for clients that have subscribed to it.
// The extra reference exists only in this validation copy, never in xDS output.
func snapshotConsistencyError(proxyKey string, snap *envoycache.Snapshot) error {
	details := getDetailsFromXDSClientResourceName(proxyKey)
	if details.Gateway == "" || details.Namespace == "" {
		return snap.Consistent()
	}
	name := ir.LocalClusterName(details.Gateway, details.Namespace)
	if _, exists := snap.Resources[envoycachetypes.Endpoint].Items[name]; !exists {
		return snap.Consistent()
	}
	if _, dynamic := snap.Resources[envoycachetypes.Cluster].Items[name]; dynamic {
		return snap.Consistent()
	}
	checked := *snap
	clusters := snap.Resources[envoycachetypes.Cluster]
	clusters.Items = cloneResourceItems(clusters.Items)
	clusters.Items[name] = envoycachetypes.ResourceWithTTL{Resource: &envoyclusterv3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
	}}
	checked.Resources[envoycachetypes.Cluster] = clusters
	return checked.Consistent()
}
