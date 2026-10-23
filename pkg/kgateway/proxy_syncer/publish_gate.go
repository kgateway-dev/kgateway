package proxy_syncer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// publishGate bounds every per-client publication withhold, because
// unreadiness may never converge (#14352): a plugin EDS cluster's endpoints
// row may never appear, and translation backlog can outlast any probe window.
// The publish budget (KGW_PER_CLIENT_PUBLISH_BUDGET; nonpositive disables both)
// sets two bounds:
//
//   - first publish (offerColdLocked/firePending): how long a client not yet
//     published on its current connection waits for referenced clusters;
//   - flip release (publishHeldLocked/fireFlipRelease): how long a held route
//     flip may pin route/listener/secret updates while a newly-referenced
//     cluster stays unready.
//
// The gate resolves only against snapshots it published on the client's
// current connection (connectionSnapshotLocked). After a reconnect the cache
// entry can predate config the client got from another replica: holding
// against it would republish old routes, and carrying from it would resurrect
// deleted backends. A reconnected client goes through first publish.
//
// First publish: an unpublished client has nothing to hold or carry, and
// publishing routes to unready clusters would 503 them, so it is withheld.
// Unbounded, a permanently unready reference leaves the pod without listeners:
// it never goes Ready and crash-loops, re-triggering the per-client fan-out on
// every restart. The budget starts when the client's first deferred build
// reaches the gate, and resumes across a brief departure (coldEpisodes). At
// expiry the latest deferred snapshot, which is internally consistent,
// publishes so the pod gets listeners; unready routes 503 until they cohere.
//
// A warm client's first listener request on this connection carried a
// previously accepted version, so it may still serve config from an earlier
// stream. That version is the only warmth signal and does not prove the config
// is active, but a controller-local cache miss cannot prove the client cold.
// Publishing routes to clusters missing from its CDS would NC routes it may
// serve, so it stays withheld past the budget while any is missing (#13868
// make-before-break), for at most warmWithholdBound
// (KGW_PER_CLIENT_WARM_WITHHOLD_BOUND, minutes) from the budget's expiry. A
// cluster missing that long is not restart catch-up, and holding longer would
// freeze routes, listeners and certificates until the proxy restarts.
// Reconnects resume the episode. A cold proxy sharing the key (pod locality
// off) lifts the withhold once it has waited a budget (firePending).
//
// Gaps that are only underived CLAs (missingEndpointsReferenced) do not extend
// the warm withhold: after a budget they have no convergence guarantee, and
// withholding would freeze the client indefinitely. At expiry the snapshot
// publishes without those CLAs, and the client keeps the endpoints it holds
// (settleSynthesizedEndpoints).
//
// Flip release: resolveDeferredPerCluster holds routes, listeners and secrets
// at their published versions while a flip targets a newly-referenced unready
// cluster. The hold is per resource type, so it pins every such update, and a
// reference that never resolves would pin them forever. The first held publish
// of an episode arms a release timer. At expiry the latest held wrapper is
// re-resolved with holds disabled and published: routes to unready clusters
// fail until ready, and pinned updates resume. Later holds do not extend the
// deadline; a publish without a hold ends the episode.
//
// All snapshot-cache writes for gated clients happen under the gate's lock, so
// an expiring timer cannot overwrite a newer publish, and each timer acts only
// on the episode that armed it.
type publishGate struct {
	// budget bounds first-publish withholds and held flips; nonpositive
	// disables both. Written once at construction.
	budget time.Duration
	// warmWithholdBound (KGW_PER_CLIENT_WARM_WITHHOLD_BOUND) bounds warm
	// withholds on clusters missing from CDS and how long missing clusters are
	// carried. Nonpositive removes it; it applies only while budget is
	// positive. Written before first use.
	warmWithholdBound time.Duration
	// warmWithheld counts, per gateway, warm clients withheld past the budget
	// (warmWithheldSince set), a subset of firstPublishPending. The
	// withheld_clients gauge reports them as state=warm_withheld, the rest as
	// within_budget. Guarded by mu; created on first use.
	warmWithheld map[resourceNameDetails]int
	// firstPublishPending counts, per gateway, clients whose first publish on
	// their connection is withheld (pendingFirstPublish.counted). Guarded by
	// mu; created on first use.
	firstPublishPending map[resourceNameDetails]int
	// warmEpisodes keeps a warm withhold's start across departures, so a
	// connection that resets faster than the bound is still released. Entries
	// expire a bound after departure; any publish removes them. Guarded by mu;
	// created on first use.
	warmEpisodes map[string]warmEpisode
	// carriedSince records, per client, when each carried cluster started
	// being carried (boundCarryLocked). Guarded by mu; created on first use.
	carriedSince map[string]map[string]time.Time
	// carryExpiry holds each client's timer for its earliest carry deadline,
	// for when no build arrives (armCarryExpiryLocked). Guarded by mu; created
	// on first use.
	carryExpiry map[string]*carryExpiry
	// coldEpisodes is warmEpisodes for the first-publish budget: a client whose
	// connection or snapshot row resets faster than the budget still gets its
	// bounded first publish. Entries expire a budget after departure; any
	// publish removes them. Guarded by mu; created on first use.
	coldEpisodes map[string]warmEpisode
	// checkConsistency checks every publish, recording violations without
	// withholding (KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK). Written once at
	// construction.
	checkConsistency bool
	// clientState reports warm clients (prior accepted xDS version); nil treats
	// every client as cold. Written once at construction.
	clientState krtcollections.XDSClientState
	// afterPublish, if set, runs under mu after each install so the reclaimer can
	// catch entries re-created after departure. Set before first use.
	afterPublish func(proxyKey string)

	// dereferenceGrace is how long a de-referenced cluster stays published;
	// nonpositive disables retention, and it is 0 outside REFERENCED mode.
	// Written once at construction.
	dereferenceGrace time.Duration

	// referenceAhead is how long configuration naming a newly-emitted cluster
	// is held; nonpositive disables it. Zero without scoped CDS. Written once at
	// construction.
	referenceAhead time.Duration

	mu           sync.Mutex
	pending      map[string]*pendingFirstPublish
	pendingFlips map[string]*pendingFlipRelease
	// published marks clients the gate has published to on their current
	// connection. clientDeparted clears the mark; snapshotWithdrawn keeps it.
	published map[string]struct{}
	// connGen holds each key's fence generation, adopted in connectionStarted.
	// setSnapshot installs only while it still matches the fence. Without start
	// hooks it stays zero.
	connGen map[string]uint64
	// fence orders installs against connection starts (see connectionFence).
	fence connectionFence
	// beforeInstall is a test hook run under mu just before setSnapshot installs.
	beforeInstall func(proxyKey string)
	// connectionReset is a test hook run under mu between reset and adoption.
	connectionReset func(proxyKey string)
	// currentBuild returns a client's current row, or nil, for beginBuildLocked.
	// ProxySyncer.Start sets it before first use; nil skips the check.
	currentBuild func(proxyKey string) *XdsSnapWrapper
	// publishedEDS holds each client's contributions as last installed on its
	// current connection. Guarded by mu.
	publishedEDS map[string]map[string]uint64
	dereferenced map[string]*dereferenceState
	// aheadSent records, per client, when the gate first published each cluster
	// in an open reference-ahead window, timed before installation; sending is
	// not observed. clientDeparted clears it.
	aheadSent map[string]map[string]time.Time
}

type pendingFirstPublish struct {
	// wrap is the latest deferred wrapper, published at expiry unless a warm
	// client stays withheld for its missing clusters. A nil wrap.snap means
	// nothing is pending.
	wrap  XdsSnapWrapper
	timer *time.Timer
	// since starts the budget; it survives brief departures (coldEpisodes).
	since time.Time
	// Preserve the warm withhold start across budget rechecks.
	warmWithheldSince time.Time
	// counted means the client is in firstPublishPending, until its first
	// publish or the entry ends.
	counted bool
}

// warmEpisode keeps an episode's start across a brief departure (warm
// withhold or first-publish budget).
type warmEpisode struct {
	since      time.Time
	departedAt time.Time
}

// carryExpiry re-resolves the latest deferred build when the client's earliest
// carry reaches the warm withhold bound (armCarryExpiryLocked).
type carryExpiry struct {
	// wrap is the latest deferred build, whose resolution carried.
	wrap  XdsSnapWrapper
	timer *time.Timer
}

type pendingFlipRelease struct {
	// wrap is the latest held wrapper; expiry re-resolves it with holds
	// disabled against the then-published snapshot.
	wrap XdsSnapWrapper
	// blocking are the latest unready cluster names; ahead the latest clusters
	// in open reference-ahead windows. At least one is non-empty.
	blocking []string
	ahead    []string
	// since is when the hold episode began, zero while its row is withdrawn;
	// unreadySince when its current run of unready holds began, zero without
	// one. flipReleaseDeadline uses both.
	since        time.Time
	unreadySince time.Time
	timer        *time.Timer
	// deadline is when the timer fires; epoch identifies its arming, so a stale
	// callback does nothing. capped records that the episode budget cut the
	// deadline short.
	deadline time.Time
	epoch    uint64
	capped   bool
}

func newPublishGate(budget time.Duration, checkConsistency bool, clientState krtcollections.XDSClientState, scoping clusterScoping) *publishGate {
	return &publishGate{
		budget:           budget,
		checkConsistency: checkConsistency,
		clientState:      clientState,
		dereferenceGrace: scoping.DereferenceGrace(),
		referenceAhead:   scoping.ReferenceAhead(),
		pending:          make(map[string]*pendingFirstPublish),
		pendingFlips:     make(map[string]*pendingFlipRelease),
		published:        make(map[string]struct{}),
		publishedEDS:     make(map[string]map[string]uint64),
		dereferenced:     make(map[string]*dereferenceState),
		aheadSent:        make(map[string]map[string]time.Time),
	}
}

// warm reports a prior accepted listener version: the client may serve config
// from an earlier stream this gate never published.
func (g *publishGate) warm(proxyKey string) bool {
	return g.clientState != nil && g.clientState.HasPriorXDSVersion(proxyKey)
}

// coldStreamReader optionally reports a cold proxy sharing the client key.
type coldStreamReader interface {
	HasColdStreamSince(resourceName string, d time.Duration) bool
}

// connectionSnapshotLocked returns the snapshot last published on the client's
// current connection, or nil. An entry from an earlier connection says nothing
// about what the client runs, so it is ignored. Callers must hold g.mu.
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

// setSnapshot is the gate's only snapshot-cache write. It settles synthesized
// CLAs (settleSynthesizedEndpoints) and, with checkConsistency, logs and
// counts snapshotConsistencyError violations but still publishes: blocking
// would reintroduce unbounded withholds. Callers must hold g.mu.
func (g *publishGate) setSnapshot(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, snap *envoycache.Snapshot, synthesized []string, edsHashes map[string]uint64) error {
	published := g.connectionSnapshotLocked(cache, proxyKey)
	settled := settleSynthesizedEndpoints(snap, synthesized, published)
	if settled != snap {
		// Republished assignments keep their contribution; omitted ones add nothing.
		hashes := endpointSetHashes(settled.Resources[envoycachetypes.Endpoint].Items, edsHashes, nil)
		if published != nil {
			previous := published.GetResourcesAndTTL(envoyresourcev3.EndpointType)
			for _, name := range synthesized {
				if _, included := hashes[name]; !included {
					continue
				}
				if _, ok := previous[name]; ok {
					if h, ok := g.publishedEDS[proxyKey][name]; ok {
						hashes[name] = h
					}
				}
			}
		}
		edsHashes = hashes
		settled.Resources[envoycachetypes.Endpoint] = versionEndpointSet(settled.Resources[envoycachetypes.Endpoint], hashes)
	}
	snap = settled
	if g.checkConsistency {
		if err := snapshotConsistencyError(proxyKey, snap); err != nil {
			logger.Error("BUG: per-client snapshot failed consistency check before publish; publishing anyway",
				"proxy_key", proxyKey, "error", err)
			recordInconsistentSnapshot()
		}
	}
	if g.beforeInstall != nil {
		g.beforeInstall(proxyKey)
	}
	if err := g.fence.install(ctx, cache, proxyKey, g.connGen[proxyKey], snap); err != nil {
		if errors.Is(err, errConnectionSuperseded) {
			return err // computed for the client's previous connection
		}
		// go-control-plane installs the snapshot before answering watches, and only
		// answering can fail. Later requests are served from the installed snapshot,
		// so the bookkeeping below (episodes, gauges, flip bounds, EDS contributions)
		// follows installation even on that error.
		if installed, getErr := cache.GetSnapshot(proxyKey); getErr != nil || installed != envoycache.ResourceSnapshot(snap) {
			return err
		}
		logger.Error("installed xds snapshot but failed to answer open watches", "proxy_key", proxyKey, "error", err)
	}
	g.publishedEDS[proxyKey] = edsHashes
	g.published[proxyKey] = struct{}{}
	// A publish ends the first-publish and warm withhold episodes.
	delete(g.coldEpisodes, proxyKey)
	delete(g.warmEpisodes, proxyKey)
	if g.afterPublish != nil {
		g.afterPublish(proxyKey)
	}
	return nil
}

// lastPublishedEDSLocked looks up proxyKey's remembered contributions; nil if none.
func (g *publishGate) lastPublishedEDSLocked(proxyKey string) func(string) (uint64, bool) {
	hashes := g.publishedEDS[proxyKey]
	if hashes == nil {
		return nil
	}
	return func(name string) (uint64, bool) {
		h, ok := hashes[name]
		return h, ok
	}
}

// publish publishes a coherent snapshot and cancels pending first-publish and
// flip-release timers, since a no-hold publish ends the episode. One lock
// covers both, so an expiring timer cannot overwrite the new snapshot.
func (g *publishGate) publish(ctx context.Context, cache envoycache.SnapshotCache, snapWrap XdsSnapWrapper) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.beginBuildLocked(snapWrap) {
		return nil
	}
	return g.publishLocked(ctx, cache, snapWrap.proxyKey, snapWrap.snap, snapWrap.synthesizedEndpoints, snapWrap.endpointHashes)
}

func (g *publishGate) publishLocked(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, snap *envoycache.Snapshot, synthesized []string, edsHashes map[string]uint64) error {
	if st := g.pending[proxyKey]; st != nil {
		if st.timer != nil {
			st.timer.Stop()
		}
		g.unmarkFirstPublishPendingLocked(proxyKey, st)
		g.unmarkWarmWithheldLocked(proxyKey, st)
		delete(g.pending, proxyKey)
	}
	g.cancelFlipReleaseLocked(proxyKey)
	// This build supersedes the pending carry expiry's; a deferred build
	// re-arms it after publishing.
	g.cancelCarryExpiryLocked(proxyKey)
	// Keep the de-reference timer: carried graced clusters still need it.
	// graceDereferencedClustersLocked cancels it when the last window closes.
	return g.setSnapshot(ctx, cache, proxyKey, snap, synthesized, edsHashes)
}

// resolveDeferred reads the cache, resolves per cluster and publishes under one
// lock. Otherwise a timer firing in between could strand the newest deferred
// snapshot until the next build (first publish), or have its flip release
// overwritten by a hold composed before it, re-pinning the released routes
// for another budget.
func (g *publishGate) resolveDeferred(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.beginBuildLocked(snapWrap) {
		return nil
	}
	return g.resolveDeferredLocked(ctx, cache, snapWrap)
}

// beginBuildLocked reports whether to publish a new build. Timers that
// re-publish a stored build skip it. Callers must hold g.mu.
//
// Only the client's current row is published. The connection-start hook
// publishes that row ahead of queued events, so an older queued build would
// replace it and answer the new stream's watches with older routes. The fence
// cannot catch this: the gate already adopted the new connection. The check
// and the publication share the gate lock. A skipped build loses nothing,
// since later row changes have their own events.
//
// A build that has a carried cluster again ends that carry episode, so a later
// disappearance gets a fresh warm withhold bound. boundCarryLocked does this
// only for deferred builds, and recoveries are usually coherent. Without this,
// the next brief disappearance would be dropped at once instead of carried.
func (g *publishGate) beginBuildLocked(snapWrap XdsSnapWrapper) bool {
	if g.currentBuild != nil {
		if current := g.currentBuild(snapWrap.proxyKey); current == nil || !current.Equals(snapWrap) {
			logger.Debug("not publishing a build the client's snapshot row has moved past", "proxy_key", snapWrap.proxyKey)
			return false
		}
	}
	since := g.carriedSince[snapWrap.proxyKey]
	if len(since) == 0 {
		return true
	}
	missing := stringSet(snapWrap.missingReferenced)
	for name := range since {
		if _, stillMissing := missing[name]; !stillMissing {
			delete(since, name)
		}
	}
	if len(since) == 0 {
		delete(g.carriedSince, snapWrap.proxyKey)
	}
	return true
}

// resolveDeferredLocked is resolveDeferred for callers that hold g.mu.
func (g *publishGate) resolveDeferredLocked(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
) error {
	published := g.connectionSnapshotLocked(cache, snapWrap.proxyKey)
	if published == nil {
		// Not yet published on this connection: nothing to hold or carry, and
		// publishing would 503 unready routes. Withhold up to the first-publish
		// budget (firePending).
		g.offerColdLocked(ctx, cache, snapWrap)
		return nil
	}
	candidate := snapWrap
	snapWrap = g.boundCarryLocked(snapWrap, published, time.Now())
	// Keep de-referenced clusters for their grace, giving the route update time
	// to land first.
	now := time.Now()
	// A deferred build can add a new cluster too (a retarget alongside an
	// unrelated gap) and gets the same hold.
	newlyEmitted, aheadFor := g.referenceAheadLocked(snapWrap.proxyKey, snapWrap, published, now)
	var pending []string
	if pf := g.pendingFlips[snapWrap.proxyKey]; pf != nil {
		pending = pf.blocking
	}
	holding := len(newlyEmitted) > 0 || len(flipBlockingClusters(snapWrap, published, pending)) > 0
	graced, opened := g.graceDereferencedClustersLocked(
		snapWrap.proxyKey, published, snapWrap.snap.Resources[envoycachetypes.Cluster], snapWrap.translatedClusters, holding, now)
	resolved, heldBlocking, edsHashes := resolveDeferredPerCluster(snapWrap, published, true, pending, graced, g.lastPublishedEDSLocked(snapWrap.proxyKey))
	var publishErr error
	switch {
	case len(heldBlocking) > 0:
		// Publish CDS/EDS while holding the flip, and arm its release timer.
		publishErr = g.publishHeldLocked(ctx, cache, snapWrap, resolved, heldBlocking, newlyEmitted, aheadFor, edsHashes)
	case len(newlyEmitted) > 0:
		held, aheadHashes := g.referenceAheadHoldLocked(snapWrap, published, graced)
		publishErr = g.publishHeldLocked(ctx, cache, snapWrap, held, nil, newlyEmitted, aheadFor, aheadHashes)
	default:
		publishErr = g.publishLocked(ctx, cache, snapWrap.proxyKey, resolved, snapWrap.synthesizedEndpoints, edsHashes)
	}
	// Arm after publishing: publishLocked and publishHeldLocked cancel the carry
	// expiry. The de-reference timer survives publishes.
	g.settleDereferenceWindowsLocked(ctx, cache, snapWrap.proxyKey, snapWrap, opened, publishErr == nil)
	g.armCarryExpiryLocked(ctx, cache, candidate)
	return publishErr
}

// offerColdLocked stores an unpublished client's latest deferred wrapper and
// arms one budget timer per episode. The timer publishes the latest wrapper
// unless a coherent publish came first or the client is warm with clusters
// missing from CDS. Callers must hold g.mu.
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
		now := time.Now()
		st = &pendingFirstPublish{since: now}
		if ep, ok := g.coldEpisodes[proxyKey]; ok {
			delete(g.coldEpisodes, proxyKey)
			if now.Sub(ep.departedAt) < g.budget {
				st.since = ep.since
			}
		}
		g.pending[proxyKey] = st
		st.counted = true
		g.adjustFirstPublishPendingLocked(proxyKey, +1)
		logger.Info("withholding first publish until referenced clusters are ready or the budget expires",
			"proxy_key", proxyKey, "budget", g.budget, "remaining", max(0, g.budget-now.Sub(st.since)),
			"missing_clusters", snapWrap.missingReferenced,
			"missing_endpoint_clusters", snapWrap.missingEndpointsReferenced,
		)
	}
	st.wrap = snapWrap
	if st.timer != nil {
		return // already armed; it will publish the latest wrapper
	}
	st.timer = time.AfterFunc(max(0, g.budget-time.Since(st.since)), func() {
		g.firePending(ctx, cache, proxyKey, st)
	})
}

// firePending publishes the latest deferred snapshot at first-publish expiry,
// unless the entry was canceled, superseded or drained. A warm client with
// clusters missing from CDS stays withheld, up to warmWithholdBound, since
// publishing would NC routes it serves; underived CLAs alone do not withhold
// it. Runs under the gate lock.
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
	// Keep the entry so a later deferred wrapper re-arms the timer instead of
	// restarting the budget.
	wrap := st.wrap
	st.wrap = XdsSnapWrapper{}
	st.timer = nil
	if _, published := g.published[proxyKey]; published {
		// Defensive: every publish deletes or resolves the pending entry under
		// this lock. Publishing here would regress a newer snapshot.
		return
	}
	mode := boundedPublishFirstPublish
	warm := g.warm(proxyKey)
	// A cold proxy sharing a warm key must get its bounded first publish.
	// setSnapshot still preserves the warm proxies' underived endpoints.
	if cold, ok := g.clientState.(coldStreamReader); ok && cold.HasColdStreamSince(proxyKey, g.budget) {
		warm = false
	}
	switch {
	case warm && len(wrap.missingReferenced) > 0:
		now := time.Now()
		if st.warmWithheldSince.IsZero() {
			g.markWarmWithheldLocked(proxyKey, st, now)
		}
		withheldFor := now.Sub(st.warmWithheldSince)
		if g.warmWithholdBound <= 0 || withheldFor < g.warmWithholdBound {
			logger.Warn("first-publish budget expired, but client reported a prior xDS version and referenced clusters are missing; withholding deferred snapshot",
				"proxy_key", proxyKey, "missing_clusters", wrap.missingReferenced, "missing_endpoint_clusters", wrap.missingEndpointsReferenced,
				"withheld_for", withheldFor, "warm_withhold_bound", g.warmWithholdBound)
			// Re-check every budget, since with static inputs no build may
			// arrive. This enforces the warm bound and publishes once CDS
			// recovers or the key stops being warm (warm streams closed, or a
			// cold proxy waited a budget).
			next := g.budget
			if g.warmWithholdBound > 0 {
				next = min(next, g.warmWithholdBound-withheldFor)
			}
			st.wrap = wrap
			st.timer = time.AfterFunc(next, func() {
				g.firePending(ctx, cache, proxyKey, st)
			})
			return
		}
		// Withheld for the whole bound, so this is not restart catch-up.
		// Release routes, listeners and certificates; routes to the missing
		// clusters fail until they appear.
		mode = boundedPublishWarmRelease
		logger.Warn("warm client withheld for the full warm withhold bound; publishing deferred snapshot, routes to the missing clusters will fail until they appear",
			"proxy_key", proxyKey, "missing_clusters", wrap.missingReferenced, "missing_endpoint_clusters", wrap.missingEndpointsReferenced,
			"withheld_for", withheldFor)
	case warm:
		// CDS is complete; only underived CLAs remain, and they may never
		// arrive (#14352). Publish without them rather than freeze the
		// client, which keeps its endpoints for them.
		mode = boundedPublishWarmTruth
		logger.Warn("first-publish budget expired for warm client; publishing without the CLAs that are not derived yet",
			"proxy_key", proxyKey, "missing_endpoint_clusters", wrap.missingEndpointsReferenced)
	default:
		logger.Warn("first-publish budget expired; publishing deferred snapshot so the client can start",
			"proxy_key", proxyKey, "missing_clusters", wrap.missingReferenced, "missing_endpoint_clusters", wrap.missingEndpointsReferenced)
	}
	if err := g.setSnapshot(ctx, cache, proxyKey, wrap.snap, wrap.synthesizedEndpoints, wrap.endpointHashes); err != nil {
		logPublishFailure(proxyKey, err)
		return
	}
	g.unmarkFirstPublishPendingLocked(proxyKey, st)
	g.unmarkWarmWithheldLocked(proxyKey, st)
	recordBoundedPublish(proxyKey, mode)
}

// markWarmWithheldLocked records when a warm client's withhold started, for
// the warm bound and the withheld_clients gauge. Callers must hold g.mu.
func (g *publishGate) markWarmWithheldLocked(proxyKey string, st *pendingFirstPublish, now time.Time) {
	st.warmWithheldSince = now
	if ep, ok := g.warmEpisodes[proxyKey]; ok {
		delete(g.warmEpisodes, proxyKey)
		if g.warmWithholdBound > 0 && now.Sub(ep.departedAt) < g.warmWithholdBound {
			st.warmWithheldSince = ep.since
		}
	}
	g.adjustWarmWithheldLocked(proxyKey, +1)
}

// unmarkWarmWithheldLocked clears the warm-withheld mark when the episode
// publishes or ends. Callers must hold g.mu.
func (g *publishGate) unmarkWarmWithheldLocked(proxyKey string, st *pendingFirstPublish) {
	if st.warmWithheldSince.IsZero() {
		return
	}
	st.warmWithheldSince = time.Time{}
	g.adjustWarmWithheldLocked(proxyKey, -1)
}

// unmarkFirstPublishPendingLocked removes the client from the withheld_clients
// gauge. Call it before unmarkWarmWithheldLocked, so a scrape in between never
// shows a warm client as within budget. Callers must hold g.mu.
func (g *publishGate) unmarkFirstPublishPendingLocked(proxyKey string, st *pendingFirstPublish) {
	if !st.counted {
		return
	}
	st.counted = false
	g.adjustFirstPublishPendingLocked(proxyKey, -1)
}

func (g *publishGate) adjustFirstPublishPendingLocked(proxyKey string, delta int) {
	if g.firstPublishPending == nil {
		g.firstPublishPending = make(map[resourceNameDetails]int)
	}
	cd := getDetailsFromXDSClientResourceName(proxyKey)
	labels := resourceNameDetails{Namespace: cd.Namespace, Gateway: cd.Gateway}
	n := g.firstPublishPending[labels] + delta
	if n <= 0 {
		delete(g.firstPublishPending, labels)
	} else {
		g.firstPublishPending[labels] = n
	}
	recordWithheldClients(labels, g.firstPublishPending[labels], g.warmWithheld[labels])
}

func (g *publishGate) adjustWarmWithheldLocked(proxyKey string, delta int) {
	if g.warmWithheld == nil {
		g.warmWithheld = make(map[resourceNameDetails]int)
	}
	cd := getDetailsFromXDSClientResourceName(proxyKey)
	labels := resourceNameDetails{Namespace: cd.Namespace, Gateway: cd.Gateway}
	n := g.warmWithheld[labels] + delta
	if n <= 0 {
		delete(g.warmWithheld, labels)
	} else {
		g.warmWithheld[labels] = n
	}
	recordWithheldClients(labels, g.firstPublishPending[labels], g.warmWithheld[labels])
}

// publishHeldLocked publishes a snapshot with its route flip held and arms
// the flip release once per episode. At expiry fireFlipRelease publishes the
// latest held wrapper with holds disabled, so a cluster that never becomes
// ready (#14352) pins route/listener/secret updates for at most one budget.
// With budget<=0 an unready hold lasts until the flip resolves; a hold kept
// only for reference-ahead windows still releases when they close. Callers
// must hold g.mu.
func (g *publishGate) publishHeldLocked(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
	held *envoycache.Snapshot,
	unready []string,
	ahead []string,
	aheadFor time.Duration,
	edsHashes map[string]uint64,
) error {
	proxyKey := snapWrap.proxyKey
	// Like any publish, this supersedes the pending carry expiry; a deferred
	// build re-arms it after publishing.
	g.cancelCarryExpiryLocked(proxyKey)
	if err := g.setSnapshot(ctx, cache, proxyKey, held, snapWrap.synthesizedEndpoints, edsHashes); err != nil {
		return err
	}
	// The hold's clock starts once the held snapshot is installed, so a slow
	// install does not use up the budget.
	now := time.Now()
	pf := g.pendingFlips[proxyKey]
	if pf == nil {
		pf = &pendingFlipRelease{since: now}
		g.pendingFlips[proxyKey] = pf
		logger.Info("holding route flip until its clusters are ready and delivered",
			"proxy_key", proxyKey, "flip_blocking", unready, "reference_ahead", ahead)
	}
	if pf.since.IsZero() {
		pf.since = now // the row returned after a withdrawal
	}
	pf.wrap = snapWrap
	pf.blocking = unready
	pf.ahead = ahead
	if len(unready) == 0 {
		pf.unreadySince = time.Time{}
	} else if pf.unreadySince.IsZero() {
		pf.unreadySince = now
	}
	deadline, capped, bounded := g.flipReleaseDeadline(pf, now, aheadFor)
	pf.capped = capped
	if !bounded {
		// Bound disabled: hold until the flip resolves. Stop cannot recall a
		// callback already waiting on g.mu; the new epoch makes it do nothing.
		if pf.timer != nil {
			pf.timer.Stop()
			pf.timer = nil
			pf.epoch++
		}
		return nil
	}
	g.armFlipReleaseLocked(ctx, cache, proxyKey, pf, deadline)
	return nil
}

// flipReleaseDeadline is when a hold is released if no build resolves it first,
// re-evaluated on every held publish. An unready cluster gets the budget from
// unreadySince, since it may never become ready (#14352). New clusters wait for
// the last open reference-ahead window. The routing types release together at
// the later deadline, capped at the budget from the episode's start. capped
// reports that cap; bounded is false for an unready hold with the budget
// disabled.
func (g *publishGate) flipReleaseDeadline(pf *pendingFlipRelease, now time.Time, aheadFor time.Duration) (deadline time.Time, capped, bounded bool) {
	if len(pf.blocking) > 0 {
		if g.budget <= 0 {
			return time.Time{}, false, false
		}
		deadline = pf.unreadySince.Add(g.budget)
	}
	if len(pf.ahead) > 0 {
		if end := now.Add(aheadFor); end.After(deadline) {
			deadline = end
		}
	}
	if g.budget > 0 {
		if limit := pf.since.Add(g.budget); deadline.After(limit) {
			return limit, true, true
		}
	}
	return deadline, false, true
}

// armFlipReleaseLocked arms pf for deadline, retaining an unchanged timer.
// Callers must hold g.mu.
func (g *publishGate) armFlipReleaseLocked(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, pf *pendingFlipRelease, deadline time.Time) {
	if pf.timer != nil && pf.deadline.Equal(deadline) {
		return
	}
	if pf.timer != nil {
		pf.timer.Stop()
	}
	pf.epoch++
	epoch := pf.epoch
	pf.deadline = deadline
	pf.timer = time.AfterFunc(time.Until(deadline), func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.pendingFlips[proxyKey] != pf || pf.epoch != epoch {
			return // re-armed, released by a no-hold publish, or a later episode's hold
		}
		g.releaseFlipLocked(ctx, cache, proxyKey, pf)
	})
}

// fireFlipRelease releases pf now, as its timer does: under g.mu, it publishes
// the latest held build re-resolved with holds disabled. Held updates resume,
// and routes to still-unready clusters fail until they are ready. It does
// nothing if pf is no longer the client's hold.
func (g *publishGate) fireFlipRelease(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, pf *pendingFlipRelease) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pendingFlips[proxyKey] != pf {
		return // released by a no-hold publish, or a later episode's hold
	}
	g.releaseFlipLocked(ctx, cache, proxyKey, pf)
}

// releaseFlipLocked releases pf after the caller checks it is the current hold.
// Callers must hold g.mu.
func (g *publishGate) releaseFlipLocked(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, pf *pendingFlipRelease) {
	delete(g.pendingFlips, proxyKey)
	// Released routes now name these clusters; closing their windows keeps a
	// capped window from re-holding them.
	if sent := g.aheadSent[proxyKey]; sent != nil {
		for _, name := range pf.ahead {
			delete(sent, name)
		}
	}
	published := g.connectionSnapshotLocked(cache, proxyKey)
	if published == nil {
		return // nothing published to resolve against; nothing was held
	}
	// The release publishes the held routes, so their de-references' windows
	// open now.
	graced, opened := g.graceDereferencedClustersLocked(
		proxyKey, published, pf.wrap.snap.Resources[envoycachetypes.Cluster], pf.wrap.translatedClusters, false, time.Now())
	released, _, edsHashes := resolveDeferredPerCluster(g.boundCarryLocked(pf.wrap, published, time.Now()), published, false, nil, graced, g.lastPublishedEDSLocked(proxyKey))
	// A reference-ahead window closing on its own is expected. Warn and count a
	// bounded publish only when the budget cut one short.
	mode := boundedPublishFlipRelease
	switch {
	case len(pf.blocking) > 0:
		logger.Warn("flip-hold budget expired; publishing held route flip, routes to still-unready clusters will fail until they become ready",
			"proxy_key", proxyKey, "flip_blocking", pf.blocking)
	case pf.capped:
		mode = boundedPublishReferenceAhead
		logger.Warn("reference-ahead window outlasted the publish budget; publishing the held configuration before its new clusters' windows closed",
			"proxy_key", proxyKey, "reference_ahead", pf.ahead)
	default:
		mode = ""
		logger.Debug("reference-ahead windows closed; publishing the held configuration",
			"proxy_key", proxyKey, "reference_ahead", pf.ahead)
	}
	err := g.setSnapshot(ctx, cache, proxyKey, released, pf.wrap.synthesizedEndpoints, edsHashes)
	// The release may grace clusters; arm their prune, or they linger until a later build.
	g.settleDereferenceWindowsLocked(ctx, cache, proxyKey, pf.wrap, opened, err == nil)
	if err != nil {
		logPublishFailure(proxyKey, err)
		return
	}
	if mode != "" {
		recordBoundedPublish(proxyKey, mode)
	}
	g.armCarryExpiryLocked(ctx, cache, pf.wrap)
}

func (g *publishGate) cancelFlipReleaseLocked(proxyKey string) {
	if pf := g.pendingFlips[proxyKey]; pf != nil {
		if pf.timer != nil {
			pf.timer.Stop()
		}
		delete(g.pendingFlips, proxyKey)
	}
}

// clientDeparted cancels the client's pending timers, so none publishes after
// it left, and forgets what was published on the connection.
func (g *publishGate) clientDeparted(proxyKey string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.forgetConnectionLocked(proxyKey)
}

// forgetConnectionLocked is clientDeparted for callers that hold g.mu.
func (g *publishGate) forgetConnectionLocked(proxyKey string) {
	g.withdrawLocked(proxyKey)
	g.cancelFlipReleaseLocked(proxyKey)
	delete(g.published, proxyKey)
	delete(g.carriedSince, proxyKey)
	delete(g.publishedEDS, proxyKey)
	delete(g.aheadSent, proxyKey)
}

// connectionStarted forgets the previous connection, as clientDeparted does,
// and adopts the fence's new generation in one critical section. A build
// decided between the two could arm a first-publish timer. The fence rejects
// the build's install, but the timer fires after adoption and publishes the
// old build to the new connection over the newer connection-start replay.
func (g *publishGate) connectionStarted(proxyKey string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.forgetConnectionLocked(proxyKey)
	if g.connectionReset != nil {
		g.connectionReset(proxyKey)
	}
	if g.connGen == nil {
		g.connGen = make(map[string]uint64)
	}
	g.connGen[proxyKey] = g.fence.current(proxyKey)
}

// snapshotWithdrawn handles a row deleted while its client stays connected,
// such as during GatewayClass recreation. It cancels pending timers but keeps
// the published mark: the client still runs that config, so a returning build
// carries and holds flips against it instead of restarting first publish.
// Pending routing blockers and their budget start times survive the withdrawal;
// listeners published during a hold do not resolve its missing resources.
func (g *publishGate) snapshotWithdrawn(proxyKey string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.withdrawLocked(proxyKey)
}

// withdrawLocked cancels the client's pending first publish, flip release, carry
// expiry and de-reference prune, keeping the first-publish and warm episodes for
// a reconnect. Routing blockers and their budget timing remain until a new
// build or connection departure. Callers must hold g.mu.
func (g *publishGate) withdrawLocked(proxyKey string) {
	if st := g.pending[proxyKey]; st != nil {
		if st.timer != nil {
			st.timer.Stop()
		}
		g.rememberColdEpisodeLocked(proxyKey, st.since)
		g.rememberWarmEpisodeLocked(proxyKey, st.warmWithheldSince)
		g.unmarkFirstPublishPendingLocked(proxyKey, st)
		g.unmarkWarmWithheldLocked(proxyKey, st)
		delete(g.pending, proxyKey)
	}
	if pf := g.pendingFlips[proxyKey]; pf != nil {
		if pf.timer != nil {
			pf.timer.Stop()
			pf.timer = nil
		}
		// Stop cannot recall a callback already waiting on g.mu. Invalidate it
		// while retaining the blockers and original deadline for a returning row.
		pf.epoch++
		// The returning row starts a new episode. An old start would cap a new
		// cluster's reference-ahead window at a time already past; blockers
		// still release from unreadySince.
		pf.since = time.Time{}
	}
	g.cancelCarryExpiryLocked(proxyKey)
	g.cancelDereferenceTimerLocked(proxyKey)
}

// disconnectedRunner runs fn only while no stream of the key is connected,
// excluding admission until fn returns. krtcollections.XDSClientState
// implements it.
type disconnectedRunner interface {
	RunIfDisconnected(resourceName string, fn func()) bool
}

// reclaimDeparted clears a departed client's cache entry and reports whether it
// did. It holds the gate lock, so no publish interleaves, and clears only under
// RunIfDisconnected: ClearSnapshot drops open watches and would strand a newly
// admitted stream.
func (g *publishGate) reclaimDeparted(cache envoycache.SnapshotCache, proxyKey string, clients disconnectedRunner) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return clients.RunIfDisconnected(proxyKey, func() {
		cache.ClearSnapshot(proxyKey)
		// Reconnect allocates a fresh generation; old generations are never reused.
		g.fence.forget(proxyKey)
		delete(g.connGen, proxyKey)
		delete(g.publishedEDS, proxyKey)
	})
}

// settleSynthesizedEndpoints decides what the client gets for each CLA that
// snapshotPerClient synthesized because its per-client row, rebuilt from
// scratch on every connect, is still being built. Envoy keeps a cluster's
// endpoints when its CLA is absent but drops them for an empty one, and a new
// EDS cluster without a CLA waits up to its initial fetch timeout. So:
//
//   - a CLA published on this connection is republished;
//   - a cluster published on this connection without a CLA stays without one;
//   - before the first publish on this connection the CLA is left out, so a
//     reconnecting proxy keeps its endpoints and a new proxy does not go Ready
//     with zero hosts behind, say, ext_authz. This does not depend on warm
//     detection;
//   - a cluster new to a published client gets the empty CLA, so it activates
//     without waiting for the fetch timeout.
//
// Derived CLAs, even empty, always publish, as do synthesized ones with no
// endpoints source (not in synthesized). published is from
// connectionSnapshotLocked.
func settleSynthesizedEndpoints(snap *envoycache.Snapshot, synthesized []string, published envoycache.ResourceSnapshot) *envoycache.Snapshot {
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
	for _, name := range synthesized {
		if _, ok := eds.Items[name]; !ok {
			continue
		}
		prev, republish := publishedEndpoints[name]
		leaveAlone := false
		if !republish {
			if published == nil {
				leaveAlone = true
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
		} else {
			delete(items, name)
		}
	}
	if items == nil {
		return snap
	}
	settled := *snap
	// Version is unset; setSnapshot assigns it.
	settled.Resources[envoycachetypes.Endpoint] = envoycache.Resources{Items: items}
	return &settled
}

// snapshotConsistencyError reports CLAs whose cluster is not an EDS cluster in
// the snapshot's CDS (except the bootstrap local cluster's, which subscribed
// clients get without a dynamic cluster), RouteConfigurations no listener
// names, and listeners naming a missing RouteConfiguration. Envoy requests
// CLAs and RouteConfigurations by the names CDS and LDS give it, so it never
// receives unreferenced ones; with an ADS cache they also froze that type
// (#14471), which newSnapshotCache no longer does (#14865). A listener missing
// its RouteConfiguration warms until RDS answers, then serves no routes. An
// EDS cluster without a CLA is fine: Envoy keeps its endpoints
// (settleSynthesizedEndpoints).
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
	var missing []string
	for name := range references[envoyresourcev3.RouteType] {
		if _, ok := snap.Resources[envoycachetypes.Route].Items[name]; !ok {
			missing = append(missing, "RouteConfiguration "+name)
		}
	}
	switch {
	case len(unreferenced) == 0 && len(missing) == 0:
		return nil
	case len(missing) == 0:
		slices.Sort(unreferenced)
		return fmt.Errorf("snapshot publishes resources no other resource references: %v", unreferenced)
	case len(unreferenced) == 0:
		slices.Sort(missing)
		return fmt.Errorf("snapshot lacks resources its listeners name: %v", missing)
	default:
		slices.Sort(unreferenced)
		slices.Sort(missing)
		return fmt.Errorf("snapshot publishes resources no other resource references: %v; and lacks resources its listeners name: %v", unreferenced, missing)
	}
}

// boundCarryLocked limits how long a referenced cluster missing from the
// builds is carried from the published snapshot. Carrying is make-before-break
// for lagging translation, but nothing else ends it: a deleted backend would
// be served forever, at addresses that may belong to other workloads by then.
// After warmWithholdBound, the gate's bound for a missing cluster that is not
// coming back, the cluster is dropped and its routes fail until it returns.
// Expired clusters move from missingReferenced to expiredCarry. Applies only
// with a non-zero budget; armCarryExpiryLocked covers quiet inputs. Callers
// must hold g.mu.
func (g *publishGate) boundCarryLocked(snapWrap XdsSnapWrapper, published envoycache.ResourceSnapshot, now time.Time) XdsSnapWrapper {
	key := snapWrap.proxyKey
	if g.budget <= 0 || g.warmWithholdBound <= 0 {
		delete(g.carriedSince, key)
		return snapWrap
	}
	publishedClusters := published.GetResourcesAndTTL(envoyresourcev3.ClusterType)
	missing := stringSet(snapWrap.missingReferenced)
	since := g.carriedSince[key]
	for name := range since {
		_, stillMissing := missing[name]
		_, carryable := publishedClusters[name]
		if !stillMissing || !carryable {
			delete(since, name) // it came back, or it is no longer there to carry
		}
	}
	var expired map[string]struct{}
	var carriedFor time.Duration
	for name := range missing {
		if _, carryable := publishedClusters[name]; !carryable {
			continue
		}
		start, recorded := since[name]
		if !recorded {
			if since == nil {
				since = make(map[string]time.Time)
				if g.carriedSince == nil {
					g.carriedSince = make(map[string]map[string]time.Time)
				}
				g.carriedSince[key] = since
			}
			since[name] = now
			continue
		}
		if age := now.Sub(start); age >= g.warmWithholdBound {
			if expired == nil {
				expired = make(map[string]struct{})
			}
			expired[name] = struct{}{}
			delete(since, name)
			carriedFor = max(carriedFor, age)
		}
	}
	if len(since) == 0 {
		delete(g.carriedSince, key)
	}
	if len(expired) == 0 {
		return snapWrap
	}
	logger.Warn("referenced clusters missing from CDS for the whole warm withhold bound; no longer carrying their last published config, routes to them fail until they reappear",
		"proxy_key", key, "clusters", slices.Sorted(maps.Keys(expired)), "carried_for", carriedFor)
	recordBoundedPublish(key, boundedPublishCarryExpired)
	bounded := snapWrap
	bounded.missingReferenced = slices.DeleteFunc(slices.Clone(snapWrap.missingReferenced), func(name string) bool {
		_, gone := expired[name]
		return gone
	})
	// A held flip carries its whole published closure; it must not carry these
	// back, or the next build would restart them.
	bounded.expiredCarry = slices.Sorted(maps.Keys(expired))
	return bounded
}

// armCarryExpiryLocked schedules the client's earliest carry deadline.
// boundCarryLocked runs only when a build resolves, and with quiet inputs none
// may arrive, so a deleted backend would be carried forever. At the deadline
// the timer re-resolves wrap, the latest deferred build, dropping expired
// carries and re-arming for the rest. It never re-publishes an older build:
// every publish cancels it, a deferred build re-arms it with itself, and
// departure or withdrawal cancels it. Callers must hold g.mu.
func (g *publishGate) armCarryExpiryLocked(ctx context.Context, cache envoycache.SnapshotCache, wrap XdsSnapWrapper) {
	key := wrap.proxyKey
	g.cancelCarryExpiryLocked(key)
	var earliest time.Time
	for _, since := range g.carriedSince[key] {
		if earliest.IsZero() || since.Before(earliest) {
			earliest = since
		}
	}
	if earliest.IsZero() {
		return // nothing carried (or the bound is off, which records nothing)
	}
	ce := &carryExpiry{wrap: wrap}
	if g.carryExpiry == nil {
		g.carryExpiry = make(map[string]*carryExpiry)
	}
	g.carryExpiry[key] = ce
	ce.timer = time.AfterFunc(max(0, time.Until(earliest.Add(g.warmWithholdBound))), func() {
		g.fireCarryExpiry(ctx, cache, key, ce)
	})
}

func (g *publishGate) cancelCarryExpiryLocked(proxyKey string) {
	if ce := g.carryExpiry[proxyKey]; ce != nil {
		ce.timer.Stop()
		delete(g.carryExpiry, proxyKey)
	}
}

// fireCarryExpiry re-resolves the latest deferred build at a carry deadline,
// under the gate lock, only for the arming that scheduled it.
func (g *publishGate) fireCarryExpiry(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, ce *carryExpiry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.carryExpiry[proxyKey] != ce {
		return // superseded by a later build, or the client left
	}
	delete(g.carryExpiry, proxyKey)
	if g.connectionSnapshotLocked(cache, proxyKey) == nil {
		return // defensive: a reset connection cancels the timer
	}
	if err := g.resolveDeferredLocked(ctx, cache, ce.wrap); err != nil {
		logPublishFailure(proxyKey, err)
	}
}

// rememberColdEpisodeLocked keeps a departing unpublished client's episode
// start for a reconnect (coldEpisodes). Callers must hold g.mu.
func (g *publishGate) rememberColdEpisodeLocked(proxyKey string, since time.Time) {
	if since.IsZero() || g.budget <= 0 {
		return
	}
	now := time.Now()
	if g.coldEpisodes == nil {
		g.coldEpisodes = make(map[string]warmEpisode)
	}
	for key, ep := range g.coldEpisodes {
		if now.Sub(ep.departedAt) >= g.budget {
			delete(g.coldEpisodes, key)
		}
	}
	g.coldEpisodes[proxyKey] = warmEpisode{since: since, departedAt: now}
}

// rememberWarmEpisodeLocked keeps a departing warm client's episode start for
// a reconnect (warmEpisodes); without a bound there is nothing to resume.
// Callers must hold g.mu.
func (g *publishGate) rememberWarmEpisodeLocked(proxyKey string, since time.Time) {
	if since.IsZero() || g.warmWithholdBound <= 0 {
		return
	}
	now := time.Now()
	if g.warmEpisodes == nil {
		g.warmEpisodes = make(map[string]warmEpisode)
	}
	for key, ep := range g.warmEpisodes {
		if now.Sub(ep.departedAt) >= g.warmWithholdBound {
			delete(g.warmEpisodes, key)
		}
	}
	g.warmEpisodes[proxyKey] = warmEpisode{since: since, departedAt: now}
}
