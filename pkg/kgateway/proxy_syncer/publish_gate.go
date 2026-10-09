package proxy_syncer

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
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
//     has not been published a snapshot on its current connection waits for
//     its referenced clusters to become ready;
//   - flip release (publishHeldLocked/fireFlipRelease): how long a held
//     route flip may pin the client's route/listener/secret updates at their
//     published versions while a newly-referenced cluster stays unready.
//
// The gate resolves only against snapshots it published on the client's
// current connection (connectionSnapshotLocked). The snapshot cache keeps a
// client's entry after the client leaves, so after a reconnect the entry can
// predate config the client received from another controller replica:
// holding a flip against it would republish its old routes, and carrying
// from it would resurrect deleted backends with their old endpoints. A
// reconnected client therefore goes through first publish, like a new one.
//
// First publish: the per-cluster resolution (resolveDeferred) withholds a
// deferred snapshot from a client not yet published on this connection —
// there is no known config to hold or carry, and publishing routes to
// unready clusters would 503 them.
// That is correct for the brief converging case, but with no bound a
// permanently-unready reference leaves the pod with no listeners at all — it
// never reports Ready and crash-loops, and each restart reconnects as a new
// client that re-triggers the per-client fan-out. So the gate arms one timer
// per unpublished client. If the client's inputs cohere within the budget,
// the coherent publish wins and the timer is discarded. At expiry the latest
// deferred snapshot is published as-is — it is always internally consistent
// — so the pod binds its listeners and becomes Ready while routes to
// still-unready clusters return 503 until they cohere.
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
// underived CLAs (missingEndpointsReferenced). A CLA still underived a full
// budget after the client connected is backlog or a plugin that never
// derives endpoints for its EDS cluster — states with no convergence
// guarantee. Withholding on them would freeze the warm client indefinitely —
// no route change, cert rotation, or endpoint update would ever reach it —
// and would starve any cold pod that shares its UCC key (the prior-version
// mark is key-level). So at expiry, if every referenced cluster is present
// in CDS, the snapshot publishes without those CLAs; the client keeps the
// endpoints it holds for them (settleSynthesizedEndpoints), and the next
// build publishes each CLA as soon as it is derived.
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
// so an expiring timer can never overwrite a newer publish, and each timer
// acts only on the episode that armed it.
type publishGate struct {
	// budget is how long a withhold (first publish or held flip) may last
	// before its bounded publish; 0 disables both bounds (withhold/hold until
	// coherent, with no deadline). Written once at construction.
	budget time.Duration
	// checkConsistency verifies every publish, recording (never withholding
	// on) violations — an invariant monitor for test/CI environments
	// (KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK). Written once at construction.
	checkConsistency bool
	// clientState reports whether a client reported a prior accepted xDS
	// version (warm). May be nil: every client is then treated as cold.
	// Written once at construction.
	clientState krtcollections.XDSClientState

	mu           sync.Mutex
	pending      map[string]*pendingFirstPublish
	pendingFlips map[string]*pendingFlipRelease
	// published holds the clients the gate has published a snapshot to on
	// their current connection; clientDeparted removes them.
	published map[string]struct{}
}

type pendingFirstPublish struct {
	// wrap is the latest deferred wrapper; the timer publishes whatever is
	// latest at expiry. Its missing clusters decide whether a warm client
	// publishes or stays withheld. A nil wrap.snap means nothing is pending.
	wrap  XdsSnapWrapper
	timer *time.Timer
}

type pendingFlipRelease struct {
	// wrap is the latest deferred wrapper whose flip was held; at expiry it
	// is re-resolved against the then-published snapshot with holds disabled.
	wrap XdsSnapWrapper
	// blocking are the latest flip-blocking cluster names, for the expiry log.
	blocking []string
	timer    *time.Timer
}

func newPublishGate(budget time.Duration, checkConsistency bool, clientState krtcollections.XDSClientState) *publishGate {
	return &publishGate{
		budget:           budget,
		checkConsistency: checkConsistency,
		clientState:      clientState,
		pending:          make(map[string]*pendingFirstPublish),
		pendingFlips:     make(map[string]*pendingFlipRelease),
		published:        make(map[string]struct{}),
	}
}

// warm reports whether the client reported a prior accepted xDS version: it
// may be serving config from an earlier stream that this gate never published.
func (g *publishGate) warm(proxyKey string) bool {
	return g.clientState != nil && g.clientState.HasPriorXDSVersion(proxyKey)
}

// connectionSnapshotLocked returns the snapshot the gate last published to the
// client on its current connection, or nil if it has published none. A cache
// entry left over from an earlier connection is ignored: it says nothing about
// what the client runs now. Callers must hold g.mu.
func (g *publishGate) connectionSnapshotLocked(cache envoycache.SnapshotCache, proxyKey string) envoycache.ResourceSnapshot {
	if _, ok := g.published[proxyKey]; !ok {
		return nil
	}
	snap, err := cache.GetSnapshot(proxyKey)
	if err != nil {
		return nil
	}
	return snap
}

// setSnapshot is the single funnel through which the gate writes to the
// snapshot cache. It first settles the snapshot's synthesized CLAs against
// what the client already holds (settleSynthesizedEndpoints). With
// checkConsistency enabled it then verifies that the snapshot publishes no
// resource its other resources do not reference, which the publication paths
// maintain by construction; a violation is recorded and logged but the
// snapshot is still published (a publish-blocking check would reintroduce
// unbounded withholds). Callers must hold g.mu.
func (g *publishGate) setSnapshot(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, snap *envoycache.Snapshot, synthesized []string) error {
	snap = settleSynthesizedEndpoints(snap, synthesized, g.connectionSnapshotLocked(cache, proxyKey), g.warm(proxyKey))
	if g.checkConsistency {
		if err := snapshotConsistencyError(proxyKey, snap); err != nil {
			logger.Error("BUG: per-client snapshot failed consistency check before publish; publishing anyway",
				"proxy_key", proxyKey, "error", err)
			recordInconsistentSnapshot(proxyKey)
		}
	}
	if err := cache.SetSnapshot(ctx, proxyKey, snap); err != nil {
		return err
	}
	g.published[proxyKey] = struct{}{}
	return nil
}

// publish publishes a coherent (or per-cluster-resolved, no-hold) snapshot
// and cancels any pending bounded first publish and any pending flip release
// — a publish without a hold means the episode resolved — under one lock so
// a concurrently-expiring timer cannot overwrite the newer snapshot.
func (g *publishGate) publish(ctx context.Context, cache envoycache.SnapshotCache, snapWrap XdsSnapWrapper) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.publishLocked(ctx, cache, snapWrap.proxyKey, snapWrap.snap, snapWrap.synthesizedEndpoints)
}

func (g *publishGate) publishLocked(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, snap *envoycache.Snapshot, synthesized []string) error {
	if st := g.pending[proxyKey]; st != nil {
		if st.timer != nil {
			st.timer.Stop()
		}
		delete(g.pending, proxyKey)
	}
	g.cancelFlipReleaseLocked(proxyKey)
	return g.setSnapshot(ctx, cache, proxyKey, snap, synthesized)
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
) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	published := g.connectionSnapshotLocked(cache, snapWrap.proxyKey)
	if published == nil {
		// Not yet published on this connection: there is no known config to
		// hold or carry, and publishing an incoherent snapshot would 503 the
		// unready routes — so withhold (cold-start make-before-break,
		// unchanged from the whole-snapshot gate), but only up to the
		// first-publish budget: a pod with no config at all never goes Ready
		// and crash-loops, so past the budget the latest deferred snapshot is
		// published anyway. Clients that reported a prior accepted xDS
		// version are warm, not cold: they stay withheld at expiry only
		// while clusters are missing from CDS; when the only gaps are
		// underived CLAs, the snapshot publishes without them rather than
		// freezing the client indefinitely.
		g.offerColdLocked(ctx, cache, snapWrap)
		return nil
	}
	resolved, heldBlocking := resolveDeferredPerCluster(snapWrap, published, true)
	if len(heldBlocking) > 0 {
		// The held snapshot still publishes (CDS/EDS keep flowing); the
		// gate additionally arms the flip-release bound for the episode.
		return g.publishHeldLocked(ctx, cache, snapWrap, resolved, heldBlocking)
	}
	return g.publishLocked(ctx, cache, snapWrap.proxyKey, resolved, snapWrap.synthesizedEndpoints)
}

// offerColdLocked records the latest deferred wrapper for a client not yet
// published on this connection and arms the budget timer once per episode;
// the timer publishes whatever is latest unless the client turns out to be
// warm (prior xDS version) with clusters missing from CDS, or a coherent
// publish got there first. Callers must hold g.mu.
func (g *publishGate) offerColdLocked(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
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
	st.wrap = snapWrap
	if st.timer != nil {
		return // already armed; it will publish the latest wrapper
	}
	st.timer = time.AfterFunc(g.budget, func() {
		g.firePending(ctx, cache, proxyKey, st)
	})
}

// firePending runs at budget expiry: publish the latest deferred snapshot,
// unless the pending entry was canceled, superseded by a later episode, or
// drained, or the client reported a prior xDS version (warm reconnect) AND
// some referenced cluster is missing from CDS — publishing that would NC
// routes the proxy is still serving. A warm client whose only gaps are
// clusters with no derived CLA publishes anyway: by expiry that gap has no
// convergence guarantee (#14352), and withholding would freeze the client
// indefinitely (see the type comment). The check and publish happen under
// the gate lock, so they cannot interleave with publish().
func (g *publishGate) firePending(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	proxyKey string,
	st *pendingFirstPublish,
) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending[proxyKey] != st || st.wrap.snap == nil {
		return
	}
	// Keep the entry so a later deferred wrapper of the same episode re-arms
	// the timer instead of restarting the accounting.
	wrap := st.wrap
	st.wrap = XdsSnapWrapper{}
	st.timer = nil
	if _, published := g.published[proxyKey]; published {
		// Defensive: every publish path deletes or resolves the pending entry
		// under this same lock, so a publish on this connection alongside a
		// pending wrapper should be unreachable — but publishing over it
		// would regress a newer snapshot, so bail.
		return
	}
	mode := boundedPublishFirstPublish
	if g.warm(proxyKey) {
		if len(wrap.missingReferenced) > 0 {
			logger.Warn("first-publish budget expired, but client reported a prior xDS version and referenced clusters are missing; withholding deferred snapshot",
				"proxy_key", proxyKey, "missing_clusters", wrap.missingReferenced, "missing_endpoint_clusters", wrap.missingEndpointsReferenced)
			recordDeferredWithheld(proxyKey)
			return
		}
		// Warm client, but every referenced cluster is present in CDS and
		// the only gaps are clusters with underived CLAs: by now that is
		// backlog or a plugin gap with no convergence guarantee (#14352),
		// and withholding would freeze this client's config indefinitely.
		// Their CLAs are left out, so the client keeps its endpoints for them.
		mode = boundedPublishWarmTruth
		logger.Warn("first-publish budget expired for warm client; publishing without the CLAs that are not derived yet",
			"proxy_key", proxyKey, "missing_endpoint_clusters", wrap.missingEndpointsReferenced)
	} else {
		logger.Warn("first-publish budget expired; publishing deferred snapshot so the client can start",
			"proxy_key", proxyKey, "missing_clusters", wrap.missingReferenced, "missing_endpoint_clusters", wrap.missingEndpointsReferenced)
	}
	if err := g.setSnapshot(ctx, cache, proxyKey, wrap.snap, wrap.synthesizedEndpoints); err != nil {
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
	if err := g.setSnapshot(ctx, cache, proxyKey, held, snapWrap.synthesizedEndpoints); err != nil {
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
			g.fireFlipRelease(ctx, cache, proxyKey, pf)
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
// and pinned route/listener/secret updates resume. It acts only on the
// episode that armed it, and runs under the gate lock, so it cannot
// interleave with publish() or resolveDeferred().
func (g *publishGate) fireFlipRelease(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, pf *pendingFlipRelease) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pendingFlips[proxyKey] != pf {
		return // released by a no-hold publish, or a later episode's hold
	}
	delete(g.pendingFlips, proxyKey)
	published := g.connectionSnapshotLocked(cache, proxyKey)
	if published == nil {
		return // nothing published to resolve against; nothing was held
	}
	released, _ := resolveDeferredPerCluster(pf.wrap, published, false)
	logger.Warn("flip-hold budget expired; publishing held route flip, routes to still-unready clusters will fail until they become ready",
		"proxy_key", proxyKey, "flip_blocking", pf.blocking)
	if err := g.setSnapshot(ctx, cache, proxyKey, released, pf.wrap.synthesizedEndpoints); err != nil {
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
// key after its client left, and forgets what the gate published on the
// client's connection. A still-connected client re-arms the gate with its next
// deferred wrapper.
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
	delete(g.published, proxyKey)
}

// settleSynthesizedEndpoints decides what the client receives for each CLA in
// snap that snapshotPerClient synthesized because no row was derived for it.
// Whether such a backend has endpoints is unknown, and the per-client rows are
// rebuilt from scratch whenever a client reconnects, so a missing row usually
// means that rebuild has not finished. Envoy keeps a cluster's endpoints when
// its CLA is absent from a response but drops them for an empty one, so a
// synthesized CLA is published only where the client cannot hold endpoints
// for the cluster:
//
//   - the client was published a CLA for it on this connection: republish
//     that CLA;
//   - the client was published the cluster on this connection without a CLA:
//     leave its endpoints alone again;
//   - the client has not been published a snapshot on this connection:
//     leave its endpoints alone if it reported a prior accepted version (it
//     may hold them from an earlier stream), otherwise publish the empty CLA;
//   - the cluster is new to the client: publish the empty CLA, so the cluster
//     activates without waiting for its initial fetch timeout.
//
// A derived CLA, even an empty one, is the backend's truth and always
// publishes. published is the snapshot returned by connectionSnapshotLocked.
func settleSynthesizedEndpoints(snap *envoycache.Snapshot, synthesized []string, published envoycache.ResourceSnapshot, warm bool) *envoycache.Snapshot {
	if len(synthesized) == 0 {
		return snap
	}
	var publishedEndpoints map[string]envoycachetypes.ResourceWithTTL
	var publishedEDSNames map[string]struct{}
	if published != nil {
		publishedEndpoints = published.GetResourcesAndTTL(envoyresourcev3.EndpointType)
		publishedEDSNames = make(map[string]struct{})
		for _, item := range published.GetResourcesAndTTL(envoyresourcev3.ClusterType) {
			if name, ok := endpointResourceNameForCluster(item); ok {
				publishedEDSNames[name] = struct{}{}
			}
		}
	}
	eds := snap.Resources[envoycachetypes.Endpoint]
	var items map[string]envoycachetypes.ResourceWithTTL
	var settledHash uint64
	for _, name := range synthesized {
		if _, ok := eds.Items[name]; !ok {
			continue
		}
		prev, republish := publishedEndpoints[name]
		leaveAlone := false
		if !republish {
			if published == nil {
				leaveAlone = warm
			} else {
				_, leaveAlone = publishedEDSNames[name]
			}
		}
		if !republish && !leaveAlone {
			continue
		}
		if items == nil {
			items = cloneResourceItems(eds.Items)
		}
		if republish {
			items[name] = prev
			settledHash ^= utils.HashString("republished:"+name) ^ utils.HashProto(prev.Resource)
		} else {
			delete(items, name)
			settledHash ^= utils.HashString("left out:" + name)
		}
	}
	if items == nil {
		return snap
	}
	settled := *snap
	settled.Resources[envoycachetypes.Endpoint] = envoycache.Resources{
		Version: fmt.Sprintf("%s-settled-%d", eds.Version, settledHash),
		Items:   items,
	}
	return &settled
}

// snapshotConsistencyError reports resources the snapshot publishes that none
// of its other resources reference: a CLA whose cluster is not an EDS cluster
// in its CDS, or a RouteConfiguration no listener names. Envoy requests those
// types by the names CDS and LDS give it, and go-control-plane withholds an
// ADS response that carries a resource the client did not request (#14471),
// freezing that type for the client. The gateway's bootstrap local cluster is
// the one intended exception: its CLA is published without a dynamic cluster
// to clients that subscribed to it.
//
// The converse, an EDS cluster without a CLA, is not reported: Envoy keeps
// the cluster's current endpoints when its CLA is absent, which is how a
// client keeps endpoints that have not been derived yet (see
// settleSynthesizedEndpoints).
func snapshotConsistencyError(proxyKey string, snap *envoycache.Snapshot) error {
	references := envoycache.GetAllResourceReferences(snap.Resources)
	bootstrapEndpoint := ""
	if details := getDetailsFromXDSClientResourceName(proxyKey); details.Gateway != "" && details.Namespace != "" {
		bootstrapEndpoint = ir.LocalClusterName(details.Gateway, details.Namespace)
	}
	var unreferenced []string
	for name := range snap.Resources[envoycachetypes.Endpoint].Items {
		if !references[envoyresourcev3.EndpointType][name] && name != bootstrapEndpoint {
			unreferenced = append(unreferenced, "ClusterLoadAssignment "+name)
		}
	}
	for name := range snap.Resources[envoycachetypes.Route].Items {
		if !references[envoyresourcev3.RouteType][name] {
			unreferenced = append(unreferenced, "RouteConfiguration "+name)
		}
	}
	if len(unreferenced) == 0 {
		return nil
	}
	slices.Sort(unreferenced)
	return fmt.Errorf("snapshot publishes resources no other resource references: %v", unreferenced)
}
