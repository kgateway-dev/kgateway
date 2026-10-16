package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

// routedClusterWrapperV is a coherent build whose configuration names exactly
// the clusters it emits: the shape a retarget produces in REFERENCED mode.
func routedClusterWrapperV(version string, clusters ...string) XdsSnapWrapper {
	return clusterWrapperV(version, clusters...)
}

func newReferenceAheadTranslator(t *testing.T, ahead time.Duration) *ProxyTranslator {
	pt := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, time.Hour, true, scopedClustersWithReferenceAhead(ahead))
	return &pt
}

func publishedListenerVersionOf(t *testing.T, pt *ProxyTranslator) string {
	t.Helper()
	snap, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	require.NoError(t, err)
	return snap.GetVersion(resourcev3.ListenerType)
}

// TestReferenceAheadDeliversTheClusterBeforeTheRoute is the addition-side
// guarantee. Publishing a new cluster and the route that targets it in one
// snapshot does not make Envoy apply CDS first: after a CDS response is sent
// its watch is closed until Envoy ACKs, so a route landing in that window
// reaches the wire on the still-open RDS watch, before its destination. The new
// cluster must therefore go out while the routing types stay where they were.
func TestReferenceAheadDeliversTheClusterBeforeTheRoute(t *testing.T) {
	pt := newReferenceAheadTranslator(t, 80*time.Millisecond)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	// The route retargets onto a cluster this client has never been sent.
	pt.syncXds(ctx, routedClusterWrapperV("v2", "old", "new"))

	assert.ElementsMatch(t, []string{"old", "new"}, publishedClusterNames(t, pt),
		"the new destination must be delivered immediately")
	assert.Equal(t, "v1", publishedListenerVersionOf(t, pt),
		"the routing types must stay at the version the client already has until the cluster has landed")

	require.Eventually(t, func() bool {
		return publishedListenerVersionOf(t, pt) == "v2"
	}, 2*time.Second, 5*time.Millisecond,
		"the hold must release on its own window, not wait for the publish budget")
	assert.ElementsMatch(t, []string{"old", "new"}, publishedClusterNames(t, pt))
}

// TestReferenceAheadIsDisabledByZero: an operator who accepts the retarget blip
// gets cluster and route in one snapshot, exactly as before.
func TestReferenceAheadIsDisabledByZero(t *testing.T) {
	pt := newReferenceAheadTranslator(t, 0)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	pt.syncXds(ctx, routedClusterWrapperV("v2", "old", "new"))

	assert.Equal(t, "v2", publishedListenerVersionOf(t, pt), "no hold is applied")
	assert.ElementsMatch(t, []string{"old", "new"}, publishedClusterNames(t, pt))
}

// TestReferenceAheadDoesNotHoldARetargetOntoAnAlreadySentCluster is what keeps
// the hold from taxing ordinary route edits: the destination is already in the
// client's CDS, so there is nothing to deliver ahead of anything.
func TestReferenceAheadDoesNotHoldARetargetOntoAnAlreadySentCluster(t *testing.T) {
	pt := newReferenceAheadTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "a", "b"))
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	// Routes change, but every destination was already delivered.
	pt.syncXds(ctx, routedClusterWrapperV("v2", "a", "b"))

	assert.Equal(t, "v2", publishedListenerVersionOf(t, pt),
		"a retarget between clusters the client already has must not be held")
}

// TestReferenceAheadIgnoresTheBlackhole: routes whose backends fail resolution
// target the blackhole cluster. Holding on it would delay exactly the
// configurations that are already broken.
func TestReferenceAheadIgnoresTheBlackhole(t *testing.T) {
	g := newPublishGate(0, false, nil, scopedClustersWithReferenceAhead(time.Minute))
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v0", nil)

	ahead, _ := g.referenceAheadLocked(publishGateTestClient, routedClusterWrapperV("v1", wellknown.BlackholeClusterName), published, time.Now())
	assert.Empty(t, ahead)
}

// TestReferenceAheadWindowIsPerCluster: a cluster stays held for its whole
// window from when it was first sent, even once it is in the published
// snapshot, and a cluster first sent later gets its own window.
func TestReferenceAheadWindowIsPerCluster(t *testing.T) {
	g := newPublishGate(0, false, nil, scopedClustersWithReferenceAhead(time.Minute))
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v0", []envoycachetypes.ResourceWithTTL{
		{Resource: edsClusterProto("old")},
	})
	start := time.Now()

	ahead, longest := g.referenceAheadLocked(publishGateTestClient, routedClusterWrapperV("v1", "old", "first"), published, start)
	assert.Equal(t, []string{"first"}, ahead)
	assert.Equal(t, time.Minute, longest)

	// "first" is now published, but has had it for 20s of its minute.
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: edsClusterProto("old")}, {Resource: edsClusterProto("first")},
	})
	ahead, longest = g.referenceAheadLocked(publishGateTestClient, routedClusterWrapperV("v2", "old", "first", "second"), published, start.Add(20*time.Second))
	assert.Equal(t, []string{"first", "second"}, ahead)
	assert.Equal(t, time.Minute, longest, "the hold lasts until the last window closes")

	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v2", []envoycachetypes.ResourceWithTTL{
		{Resource: edsClusterProto("old")}, {Resource: edsClusterProto("first")}, {Resource: edsClusterProto("second")},
	})
	ahead, longest = g.referenceAheadLocked(publishGateTestClient, routedClusterWrapperV("v3", "old", "first", "second"), published, start.Add(70*time.Second))
	assert.Equal(t, []string{"second"}, ahead, "first's window closed; second's has 10s left")
	assert.Equal(t, 10*time.Second, longest)
}

// TestReferenceAheadHoldIsBoundedByTheBudget: the window paces delivery, the
// budget bounds it. A window longer than the budget must not outlast the
// budget, or a misconfigured value would pin a client's routing updates.
func TestReferenceAheadHoldIsBoundedByTheBudget(t *testing.T) {
	translator := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, 60*time.Millisecond, true, scopedClustersWithReferenceAhead(time.Hour))
	pt := &translator
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	pt.syncXds(ctx, routedClusterWrapperV("v2", "old", "new"))
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	require.Eventually(t, func() bool {
		return publishedListenerVersionOf(t, pt) == "v2"
	}, 2*time.Second, 5*time.Millisecond,
		"a reference-ahead window longer than the budget must still release at the budget")
}

// A rebuild inside the window, such as the new backend's endpoints arriving,
// must keep the routes held: the cluster being in the published snapshot only
// means it was installed for delivery.
func TestReferenceAheadHoldSurvivesARebuildInsideTheWindow(t *testing.T) {
	pt := newReferenceAheadTranslator(t, 150*time.Millisecond)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	pt.syncXds(ctx, routedClusterWrapperV("v2", "old", "new"))
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	rebuilt := routedClusterWrapperV("v3", "old", "new")
	rebuilt.snap.Resources[envoycachetypes.Endpoint].Version = "v3-endpoints"
	pt.syncXds(ctx, rebuilt)
	assert.Equal(t, "v1", publishedListenerVersionOf(t, pt), "the routes stay held for the new cluster's window")

	require.Eventually(t, func() bool { return publishedListenerVersionOf(t, pt) == "v3" }, 2*time.Second, 5*time.Millisecond)
}

// A cluster new to the client that only a listener filter names (an ext_authz
// server, say) is delivered ahead of the listener update too.
func TestReferenceAheadHoldsForAClusterOnlyAFilterNames(t *testing.T) {
	pt := newReferenceAheadTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "backend"))
	pt.syncXds(ctx, clusterWrapperV("v2", "backend", "authz"))

	assert.Contains(t, publishedClusterNames(t, pt), "authz", "the new cluster is delivered")
	assert.Equal(t, "v1", publishedListenerVersionOf(t, pt), "the listener that calls it waits")
}

// routedBuild is a build whose one route config names routeTo, emitting
// clusters, with listeners at version.
func routedBuild(t *testing.T, version, routeTo string, clusters ...string) XdsSnapWrapper {
	t.Helper()
	wrap := clusterWrapperV(version, clusters...)
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	listeners.Version = version
	wrap.snap.Resources[envoycachetypes.Listener] = listeners
	wrap.snap.Resources[envoycachetypes.Route] = routeResourcesForClusters(routeTo)
	return wrap
}

// While a retarget onto a new cluster is held, the held (published) routes
// still name the cluster the build dropped. It must stay published with them,
// whatever the de-reference grace: here the grace is 0.
func TestReferenceAheadHoldCarriesTheHeldConfigsClusters(t *testing.T) {
	pt := newReferenceAheadTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	pt.syncXds(ctx, routedBuild(t, "v2", "new", "new"))

	served := servedSnapshot(t, pt)
	require.Equal(t, "v1", served.GetVersion(resourcev3.ListenerType), "the retarget is held")
	assert.ElementsMatch(t, []string{"old", "new"}, publishedClusterNames(t, pt),
		"the held routes' destination stays published alongside the new cluster")
	assertSnapshotCoherent(t, served)
}

// A deferred build (here: a previously-referenced cluster whose endpoints are
// being rebuilt, which does not block) that also introduces a new cluster
// holds the configuration naming it like a coherent build does.
func TestReferenceAheadAppliesToDeferredBuilds(t *testing.T) {
	pt := newReferenceAheadTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	deferred := routedBuild(t, "v2", "old", "old", "authz")
	deferred.missingEndpointsReferenced = []string{"old"}
	deferred.synthesizedEndpoints = []string{"old"}
	pt.syncXds(ctx, deferred)

	assert.Contains(t, publishedClusterNames(t, pt), "authz")
	assert.Equal(t, "v1", publishedListenerVersionOf(t, pt), "the configuration naming the new cluster waits for it")
}

// Each hold reason keeps its own deadline. A reference-ahead hold (short
// window) that is joined by a flip onto an unready cluster must not release
// that flip at the reference-ahead window: the unready cluster gets its budget.
func TestHoldDeadlinePerReason_UnreadyFlipOutlastsTheAheadWindow(t *testing.T) {
	translator := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, 600*time.Millisecond, true,
		scopedClustersWithReferenceAhead(50*time.Millisecond))
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	pt.syncXds(ctx, routedBuild(t, "v2", "new", "old", "new")) // reference-ahead hold
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	unready := routedBuild(t, "v3", "unready", "old", "new", "unready")
	unready.missingEndpointsReferenced = []string{"unready"}
	unready.synthesizedEndpoints = []string{"unready"}
	pt.syncXds(ctx, unready)

	time.Sleep(250 * time.Millisecond) // well past the 50ms window
	assert.Equal(t, "v1", publishedListenerVersionOf(t, pt), "the unready flip is held for its budget, not the window")
	require.Eventually(t, func() bool { return publishedListenerVersionOf(t, pt) == "v3" }, 2*time.Second, 10*time.Millisecond,
		"the budget still releases it")
}

// And the other way round: an unready-cluster hold whose cluster becomes ready
// while a reference-ahead window is open releases at that window, not at the
// budget the unready hold armed.
func TestHoldDeadlinePerReason_ReadyFlipReleasesAtTheAheadWindow(t *testing.T) {
	translator := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, time.Hour, true,
		scopedClustersWithReferenceAhead(100*time.Millisecond))
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	unready := routedBuild(t, "v2", "new", "old", "new")
	unready.missingEndpointsReferenced = []string{"new"}
	unready.synthesizedEndpoints = []string{"new"}
	pt.syncXds(ctx, unready)
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	pt.syncXds(ctx, routedBuild(t, "v3", "new", "old", "new")) // endpoints derived
	require.Eventually(t, func() bool { return publishedListenerVersionOf(t, pt) == "v3" }, 2*time.Second, 10*time.Millisecond,
		"once only the reference-ahead window holds, it releases at the window, not the 1h budget")
}

// A hold kept only for reference-ahead windows ends as designed when they
// close; it is not a bounded publish. One the budget cut short is, under its
// own mode, not flip_release (which means a cluster never became ready).
func TestReferenceAheadReleaseIsNotABudgetExpiry(t *testing.T) {
	const metricName = "kgateway_xds_snapshot_perclient_bounded_publishes_total"
	ctx := context.Background()
	reset := func() {
		ResetMetrics()
		for _, mode := range []string{boundedPublishFlipRelease, boundedPublishReferenceAhead} {
			snapshotPerClientBoundedPublishesTotal.Add(0, labelsWith("mode", mode)...)
		}
	}
	bounded := func(mode string) float64 {
		t.Helper()
		return metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels(metricName, labelsWith("mode", mode))
	}

	reset()
	pt := newReferenceAheadTranslator(t, 30*time.Millisecond)
	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	pt.syncXds(ctx, routedClusterWrapperV("v2", "old", "new"))
	require.Eventually(t, func() bool { return publishedListenerVersionOf(t, pt) == "v2" }, 2*time.Second, 5*time.Millisecond)
	assert.Zero(t, bounded(boundedPublishFlipRelease), "a window closing is not a cluster that never became ready")
	assert.Zero(t, bounded(boundedPublishReferenceAhead), "nor a budget expiry")

	reset()
	translator := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, 30*time.Millisecond, true, scopedClustersWithReferenceAhead(time.Hour))
	capped := &translator
	capped.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	capped.syncXds(ctx, routedClusterWrapperV("v2", "old", "new"))
	require.Eventually(t, func() bool { return publishedListenerVersionOf(t, capped) == "v2" }, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, float64(1), bounded(boundedPublishReferenceAhead), "the budget cutting a window short is counted, under its own mode")
	assert.Zero(t, bounded(boundedPublishFlipRelease))
}
