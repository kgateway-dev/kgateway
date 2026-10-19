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

// routedClusterWrapperV stands for a REFERENCED-mode retarget build. It emits
// clusters but carries no routes; its listener version marks the configuration
// change.
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

// TestReferenceAheadDeliversTheClusterBeforeTheRoute: a retarget onto a new
// cluster publishes the cluster at once and holds the routing types until its
// window closes. It checks cached snapshots, not wire order.
func TestReferenceAheadDeliversTheClusterBeforeTheRoute(t *testing.T) {
	pt := newReferenceAheadTranslator(t, 80*time.Millisecond)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	// Retarget onto a cluster never sent to this client.
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

// TestReferenceAheadIsDisabledByZero: a 0 window publishes cluster and route
// together.
func TestReferenceAheadIsDisabledByZero(t *testing.T) {
	pt := newReferenceAheadTranslator(t, 0)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	pt.syncXds(ctx, routedClusterWrapperV("v2", "old", "new"))

	assert.Equal(t, "v2", publishedListenerVersionOf(t, pt), "no hold is applied")
	assert.ElementsMatch(t, []string{"old", "new"}, publishedClusterNames(t, pt))
}

// TestReferenceAheadDoesNotHoldARetargetOntoAnAlreadySentCluster: ordinary
// route edits are not held.
func TestReferenceAheadDoesNotHoldARetargetOntoAnAlreadySentCluster(t *testing.T) {
	pt := newReferenceAheadTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "a", "b"))
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	// Routes change; every destination is already published.
	pt.syncXds(ctx, routedClusterWrapperV("v2", "a", "b"))

	assert.Equal(t, "v2", publishedListenerVersionOf(t, pt),
		"a retarget between clusters the client already has must not be held")
}

// TestReferenceAheadIgnoresTheBlackhole: holding on it would delay only routes
// whose backends already failed resolution.
func TestReferenceAheadIgnoresTheBlackhole(t *testing.T) {
	g := newPublishGate(0, false, nil, scopedClustersWithReferenceAhead(time.Minute))
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v0", nil)

	ahead, _ := g.referenceAheadLocked(publishGateTestClient, routedClusterWrapperV("v1", wellknown.BlackholeClusterName), published, time.Now())
	assert.Empty(t, ahead)
}

// TestReferenceAheadWindowIsPerCluster: each cluster is held for a full window
// from the publish that first carries it, even once the published snapshot has
// it.
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

	// "first" is now published, 20s into its one-minute window.
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

// TestReferenceAheadHoldIsBoundedByTheBudget: a window longer than the budget
// still releases at the budget.
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

// A rebuild inside the window keeps the routes held: being in the published
// snapshot means installed, not applied.
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

// A new cluster only a listener filter names (ext_authz, say) holds the
// listener update too.
func TestReferenceAheadHoldsForAClusterOnlyAFilterNames(t *testing.T) {
	pt := newReferenceAheadTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "backend"))
	pt.syncXds(ctx, clusterWrapperV("v2", "backend", "authz"))

	assert.Contains(t, publishedClusterNames(t, pt), "authz", "the new cluster is delivered")
	assert.Equal(t, "v1", publishedListenerVersionOf(t, pt), "the listener that calls it waits")
}

// routedBuild emits clusters, routes to routeTo and versions its listeners.
func routedBuild(t *testing.T, version, routeTo string, clusters ...string) XdsSnapWrapper {
	t.Helper()
	wrap := clusterWrapperV(version, clusters...)
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	listeners.Version = version
	wrap.snap.Resources[envoycachetypes.Listener] = listeners
	wrap.snap.Resources[envoycachetypes.Route] = routeResourcesForClusters(routeTo)
	return wrap
}

// The held routes still name the cluster the build dropped, so it stays
// published even with a de-reference grace of 0.
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

// A deferred build that adds a cluster holds the configuration naming it, as a
// coherent build does. Here the deferral is a rebuilding CLA, which does not
// block.
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

// Each hold reason keeps its own deadline: an unready flip joining a short
// reference-ahead hold waits for its budget, not the window.
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

// Conversely, once the unready cluster becomes ready, the hold releases at the
// open window, not the unready budget.
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

// Windows closing on their own are not a bounded publish. A budget-capped one
// counts as reference_ahead, not flip_release, which means a cluster never
// became ready.
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

// A de-reference grace (20ms) closing during a reference-ahead hold (1h) must
// not release the retarget: the prune repeats the full hold decision.
func TestDereferencePruneKeepsAReferenceAheadHold(t *testing.T) {
	scoping := scopedClustersWithReferenceAhead(time.Hour)
	scoping.dereferenceGrace = 20 * time.Millisecond
	translator := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, time.Hour, true, scoping)
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	heldRoutes := servedSnapshot(t, pt).GetVersion(resourcev3.RouteType)
	// old is de-referenced and new is new to the client; the retarget is held.
	pt.syncXds(ctx, routedBuild(t, "v2", "new", "new"))
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt), "the retarget is held")

	time.Sleep(150 * time.Millisecond) // well past old's de-reference grace
	served := servedSnapshot(t, pt)
	assert.Equal(t, "v1", served.GetVersion(resourcev3.ListenerType), "the grace closing must not release the held listeners")
	assert.Equal(t, heldRoutes, served.GetVersion(resourcev3.RouteType), "nor the held routes")
	assert.ElementsMatch(t, []string{"old", "new"}, publishedClusterNames(t, pt),
		"the held routes' destination stays published with the new cluster")
	pt.gate.mu.Lock()
	held := pt.gate.pendingFlips[publishGateTestClient] != nil
	pt.gate.mu.Unlock()
	assert.True(t, held, "the hold is still pending its own release")
	assertSnapshotCoherent(t, served)
}
