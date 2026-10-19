package proxy_syncer

import (
	"context"
	"fmt"
	"maps"
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
// One exception: a client that reported a prior accepted listener version on
// connect is warm — an Envoy that reconnected (or outlived a controller
// restart) and is still serving config from its previous stream. Publishing
// a snapshot whose routes name clusters absent from its CDS would replace
// that working config with one that NCs those routes, so warm clients stay
// withheld while any referenced cluster is MISSING (the #13868
// make-before-break property). A controller-local cache miss alone cannot
// prove a client is cold; the client-reported version can.
// That withhold has its own last-resort bound (warmWithholdBound,
// KGW_PER_CLIENT_WARM_WITHHOLD_BOUND, minutes rather than seconds): a cluster
// still missing that long after a restart is not translation catching up, and
// holding longer would freeze the client's routes, listeners and
// certificates until its proxy restarts. The perclient_withheld_clients gauge
// shows the clients currently held, under state=warm_withheld.
// A reconnect resumes the episode rather than restarting the bound. And when
// proxies share a key (pod locality off), a proxy under it that holds no config
// lifts the withhold once it has waited a budget: it has no listeners until the
// shared snapshot publishes (see firePending).
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
	// budget bounds first-publish withholds and held flips; 0 disables both.
	// Written once at construction.
	budget time.Duration
	// warmWithholdBound (KGW_PER_CLIENT_WARM_WITHHOLD_BOUND) bounds warm
	// withholds on clusters missing from CDS and how long missing clusters are
	// carried. 0 removes it; it applies only while budget is non-zero. Written
	// before first use.
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

	mu           sync.Mutex
	pending      map[string]*pendingFirstPublish
	pendingFlips map[string]*pendingFlipRelease
	// published marks clients the gate has published to on their current
	// connection. clientDeparted clears the mark; snapshotWithdrawn keeps it.
	published map[string]struct{}
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

// warm reports a prior accepted listener version: the client may serve config
// from an earlier stream this gate never published.
func (g *publishGate) warm(proxyKey string) bool {
	return g.clientState != nil && g.clientState.HasPriorXDSVersion(proxyKey)
}

// coldStreamReader optionally reports a cold proxy sharing the client key.
type coldStreamReader interface {
	HasColdStreamSince(resourceName string, d time.Duration) bool
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
// resource its other resources do not reference, and that every
// RouteConfiguration its listeners name is present, which the publication
// paths maintain by construction; a violation is recorded and logged but the
// snapshot is still published (a publish-blocking check would reintroduce
// unbounded withholds). Callers must hold g.mu.
func (g *publishGate) setSnapshot(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, snap *envoycache.Snapshot, synthesized []string) error {
	snap = settleSynthesizedEndpoints(snap, synthesized, g.connectionSnapshotLocked(cache, proxyKey))
	if g.checkConsistency {
		if err := snapshotConsistencyError(proxyKey, snap); err != nil {
			logger.Error("BUG: per-client snapshot failed consistency check before publish; publishing anyway",
				"proxy_key", proxyKey, "error", err)
			recordInconsistentSnapshot()
		}
	}
	if err := cache.SetSnapshot(ctx, proxyKey, snap); err != nil {
		// go-control-plane installs the snapshot before answering the
		// node's open watches, and only answering can fail (the context
		// ended). Later requests are answered from the installed snapshot,
		// so the bookkeeping below follows installation: an error here must
		// not leave an episode, gauge or flip bound behind for a snapshot
		// the cache already serves.
		if installed, getErr := cache.GetSnapshot(proxyKey); getErr != nil || installed != envoycache.ResourceSnapshot(snap) {
			return err
		}
		logger.Error("installed xds snapshot but failed to answer open watches", "proxy_key", proxyKey, "error", err)
	}
	// A successful publish ends the first-publish and warm withhold episodes.
	delete(g.coldEpisodes, proxyKey)
	delete(g.warmEpisodes, proxyKey)
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
	g.beginBuildLocked(snapWrap)
	return g.publishLocked(ctx, cache, snapWrap.proxyKey, snapWrap.snap, snapWrap.synthesizedEndpoints)
}

func (g *publishGate) publishLocked(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, snap *envoycache.Snapshot, synthesized []string) error {
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
	return g.setSnapshot(ctx, cache, proxyKey, snap, synthesized)
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
	g.beginBuildLocked(snapWrap)
	return g.resolveDeferredLocked(ctx, cache, snapWrap)
}

// beginBuildLocked runs for every new build that reaches the gate, before it
// is published. Timers that re-publish a stored build do not run it: that
// build already began.
//
// A carry record belongs to one episode of a cluster missing from the builds.
// A build that has the cluster again ends the episode, so a later
// disappearance is carried for a fresh warm withhold bound. boundCarryLocked
// also forgets recovered clusters, but only deferred builds run it, and a
// recovery is usually a coherent build: without this, the earlier episode's
// start would survive it, and the next disappearance, however brief, would be
// dropped at once instead of carried. Callers must hold g.mu.
func (g *publishGate) beginBuildLocked(snapWrap XdsSnapWrapper) {
	since := g.carriedSince[snapWrap.proxyKey]
	if len(since) == 0 {
		return
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
	resolved, heldBlocking := resolveDeferredPerCluster(snapWrap, published, true)
	var publishErr error
	if len(heldBlocking) > 0 {
		// The held snapshot still publishes (CDS/EDS keep flowing); the
		// gate additionally arms the flip-release bound for the episode.
		publishErr = g.publishHeldLocked(ctx, cache, snapWrap, resolved, heldBlocking)
	} else {
		publishErr = g.publishLocked(ctx, cache, snapWrap.proxyKey, resolved, snapWrap.synthesizedEndpoints)
	}
	// Armed after publishing, which cancels it on the no-hold path.
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
	if err := g.setSnapshot(ctx, cache, proxyKey, wrap.snap, wrap.synthesizedEndpoints); err != nil {
		logger.Error("failed to set xds snapshot", "proxy_key", proxyKey, "error", err)
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
// budget<=0 holds until the flip resolves. Callers must hold g.mu.
func (g *publishGate) publishHeldLocked(
	ctx context.Context,
	cache envoycache.SnapshotCache,
	snapWrap XdsSnapWrapper,
	held *envoycache.Snapshot,
	blocking []string,
) error {
	proxyKey := snapWrap.proxyKey
	// Like any publish, this supersedes the pending carry expiry; a deferred
	// build re-arms it after publishing.
	g.cancelCarryExpiryLocked(proxyKey)
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
	released, _ := resolveDeferredPerCluster(g.boundCarryLocked(pf.wrap, published, time.Now()), published, false)
	logger.Warn("flip-hold budget expired; publishing held route flip, routes to still-unready clusters will fail until they become ready",
		"proxy_key", proxyKey, "flip_blocking", pf.blocking)
	if err := g.setSnapshot(ctx, cache, proxyKey, released, pf.wrap.synthesizedEndpoints); err != nil {
		logger.Error("failed to set xds snapshot", "proxy_key", proxyKey, "error", err)
		return
	}
	recordBoundedPublish(proxyKey, boundedPublishFlipRelease)
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
	g.withdrawLocked(proxyKey)
	delete(g.published, proxyKey)
	delete(g.carriedSince, proxyKey)
}

// snapshotWithdrawn handles a client whose wrapper row was deleted while it
// stays connected (its gateway's translation briefly produced nothing: a
// GatewayClass deleted and recreated, a route lookup error). There is nothing
// left to publish, so pending bounded publishes are canceled, but the client
// still runs what the gate published on this connection: when the row
// returns, a deferred build resolves against it (carrying the clusters the
// client serves, holding flips) instead of starting over as a first publish.
func (g *publishGate) snapshotWithdrawn(proxyKey string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.withdrawLocked(proxyKey)
}

// withdrawLocked cancels the client's pending bounded first publish and flip
// release, keeping the warm episode for a reconnect to resume. Callers must
// hold g.mu.
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
	g.cancelFlipReleaseLocked(proxyKey)
	g.cancelCarryExpiryLocked(proxyKey)
}

// settleSynthesizedEndpoints decides what the client receives for each CLA in
// snap that snapshotPerClient synthesized while the client's per-client row
// for it is still being built: some backend derives endpoints for it, and the
// per-client rows are rebuilt from scratch whenever a client connects or
// reconnects, so the row is on its way. Envoy keeps a cluster's endpoints when
// its CLA is absent from a response but drops them for an empty one, and an
// EDS cluster without a CLA waits for it (up to its initial fetch timeout)
// rather than serving with no hosts. So a synthesized CLA is published only
// where the cluster is new to an already-published client:
//
//   - the client was published a CLA for it on this connection: republish
//     that CLA;
//   - the client was published the cluster on this connection without a CLA:
//     leave it out again;
//   - the client has not been published a snapshot on this connection: leave
//     it out. A reconnecting proxy keeps the endpoints it holds; a new proxy
//     waits for the row instead of going Ready with zero hosts behind, say,
//     its ext_authz cluster. This does not depend on the client reporting a
//     prior version, so it holds even without connection state;
//   - the cluster is new to the client: publish the empty CLA, so the cluster
//     activates without waiting for its initial fetch timeout.
//
// A derived CLA, even an empty one, is the backend's truth and always
// publishes, and so does a synthesized one with no endpoints source (it is
// not in synthesized). published is the snapshot returned by
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
	var settledHash uint64
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
// It also reports a listener that names a RouteConfiguration the snapshot
// lacks. No publication path leaves one out on purpose (a held flip keeps
// routes and listeners at the same published build), and Envoy warms such a
// listener until RDS answers, then serves it with no routes.
//
// The converse for EDS, an EDS cluster without a CLA, is not reported: Envoy
// keeps the cluster's current endpoints when its CLA is absent, which is how
// a client keeps endpoints that have not been derived yet (see
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

// boundCarryLocked bounds how long a referenced cluster missing from the
// builds is carried forward from the snapshot the client was published.
//
// Carrying keeps the client's last config for a cluster whose translation is
// lagging (make-before-break), but nothing else ends it: a cluster whose
// backend is gone while routes still name it would be served its last cluster
// and endpoints forever, and endpoint addresses that old can belong to other
// workloads by then. After the warm withhold bound (the gate's existing bound
// on "a referenced cluster missing from CDS is not coming back") the cluster is
// no longer carried and routes to it fail until it reappears. The returned
// wrapper lists only the missing clusters still carried, and the expired ones
// in expiredCarry; the bound, like every other, applies only while the publish
// budget is non-zero. A build applies it when it is resolved, and
// armCarryExpiryLocked when no build arrives. Callers must hold g.mu.
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

// armCarryExpiryLocked schedules the end of the client's earliest carry.
// boundCarryLocked enforces the warm withhold bound only when a build is
// resolved, and with quiet inputs none may arrive: a cluster whose backend was
// deleted while routes still name it would be carried for as long as nothing
// else changed. At the earliest carried cluster's deadline the timer
// re-resolves wrap, the client's latest deferred build, through the same path
// as a new build, which stops carrying whatever has been carried for the bound
// and re-arms for the rest.
//
// The timer never re-publishes an older build: every publish cancels it
// (publishLocked, publishHeldLocked), a deferred build then re-arms it with
// itself, and a departure or withdrawn row cancels it. Callers must hold g.mu.
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
		logger.Error("failed to set xds snapshot", "proxy_key", proxyKey, "error", err)
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
