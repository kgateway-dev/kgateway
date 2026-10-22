package proxy_syncer

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clusterWrapperV builds a coherent cluster/CLA set to test retained endpoints.
func clusterWrapperV(version string, clusters ...string) XdsSnapWrapper {
	clusterItems := make([]envoycachetypes.ResourceWithTTL, 0, len(clusters))
	endpointItems := make([]envoycachetypes.ResourceWithTTL, 0, len(clusters))
	for _, name := range clusters {
		clusterItems = append(clusterItems, envoycachetypes.ResourceWithTTL{Resource: edsClusterProto(name)})
		endpointItems = append(endpointItems, envoycachetypes.ResourceWithTTL{
			Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: name},
		})
	}
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Listener] = envoycache.NewResources(version, nil)
	snap.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL(version, clusterItems)
	snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL(version, endpointItems)
	return XdsSnapWrapper{snap: snap, proxyKey: publishGateTestClient}
}

func publishedClusterNames(t *testing.T, pt *ProxyTranslator) []string {
	t.Helper()
	snap, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	require.NoError(t, err)
	names := make([]string, 0)
	for name := range snap.GetResourcesAndTTL(resourcev3.ClusterType) {
		names = append(names, name)
	}
	return names
}

func publishedEndpointNames(t *testing.T, pt *ProxyTranslator) []string {
	t.Helper()
	snap, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	require.NoError(t, err)
	names := make([]string, 0)
	for name := range snap.GetResourcesAndTTL(resourcev3.EndpointType) {
		names = append(names, name)
	}
	return names
}

func newGraceTestTranslator(t *testing.T, grace time.Duration) *ProxyTranslator {
	pt := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, 50*time.Millisecond, true, scopedClustersWithGrace(grace))
	return &pt
}

func TestDereferenceGraceIgnoresPreviousConnection(t *testing.T) {
	pt := newGraceTestTranslator(t, time.Minute)
	pt.syncXds(t.Context(), clusterWrapperV("old", "deleted-backend"))
	pt.gate.clientDeparted(publishGateTestClient)
	_, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	require.NoError(t, err, "departure leaves the old cache entry until reclamation")

	pt.syncXds(t.Context(), clusterWrapperV("new", "current-backend"))
	require.ElementsMatch(t, []string{"current-backend"}, publishedClusterNames(t, pt),
		"the grace period must not resurrect clusters from an earlier connection")
	require.ElementsMatch(t, []string{"current-backend"}, publishedEndpointNames(t, pt))
	require.Empty(t, pt.gate.dereferenced, "a first publication has no previous references to retain")
}

// TestDereferenceGraceKeepsTheClusterUntilTheWindowCloses: the de-referencing
// publish keeps the cluster, and the timer removes it without another build.
// This checks the cache, not Envoy ordering.
func TestDereferenceGraceKeepsTheClusterUntilTheWindowCloses(t *testing.T) {
	pt := newGraceTestTranslator(t, 100*time.Millisecond)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going-away"))
	require.ElementsMatch(t, []string{"kept", "going-away"}, publishedClusterNames(t, pt))

	// The route stops naming going-away, so it leaves the emitted set.
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))
	assert.ElementsMatch(t, []string{"kept", "going-away"}, publishedClusterNames(t, pt),
		"the de-referenced cluster must outlive the route update that dropped it")
	assert.ElementsMatch(t, []string{"kept", "going-away"}, publishedEndpointNames(t, pt),
		"a carried cluster travels with its ClusterLoadAssignment")

	require.Eventually(t, func() bool {
		return len(publishedClusterNames(t, pt)) == 1
	}, 2*time.Second, 5*time.Millisecond,
		"nothing else recomputes this client, so the grace timer must publish the removal itself")
	assert.ElementsMatch(t, []string{"kept"}, publishedClusterNames(t, pt))
	assert.ElementsMatch(t, []string{"kept"}, publishedEndpointNames(t, pt),
		"the pruned cluster's CLA must go with it, or the EDS set outlives its CDS entry")
}

// TestDereferenceGraceIsDisabledByZero: zero removes at once and keeps no state.
func TestDereferenceGraceIsDisabledByZero(t *testing.T) {
	pt := newGraceTestTranslator(t, 0)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going-away"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))

	assert.ElementsMatch(t, []string{"kept"}, publishedClusterNames(t, pt))
	assert.Empty(t, pt.gate.dereferenced, "a disabled grace must not allocate per-client state")
}

// TestDereferenceGraceDoesNotOscillateOnAFlappingRoute: re-referencing must
// clear the record, or the old window prunes a referenced cluster.
func TestDereferenceGraceDoesNotOscillateOnAFlappingRoute(t *testing.T) {
	pt := newGraceTestTranslator(t, 60*time.Millisecond)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "flapping"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))
	pt.syncXds(ctx, clusterWrapperV("v3", "kept", "flapping"))

	assert.Empty(t, pt.gate.dereferenced[publishGateTestClient],
		"re-referencing must clear the record, not leave a window running")

	// Well past the original window: the cluster is referenced, so it stays.
	time.Sleep(150 * time.Millisecond)
	assert.ElementsMatch(t, []string{"kept", "flapping"}, publishedClusterNames(t, pt),
		"an expired window from an earlier de-reference must not prune a currently-referenced cluster")
}

// TestDereferenceGraceForgetsExpiredClusters: a pruned cluster's record is
// dropped, so per-client state cannot grow without bound.
func TestDereferenceGraceForgetsExpiredClusters(t *testing.T) {
	pt := newGraceTestTranslator(t, 20*time.Millisecond)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going-away"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))

	require.Eventually(t, func() bool {
		return len(publishedClusterNames(t, pt)) == 1
	}, 2*time.Second, 5*time.Millisecond)

	pt.gate.mu.Lock()
	defer pt.gate.mu.Unlock()
	assert.Empty(t, pt.gate.dereferenced[publishGateTestClient],
		"a pruned cluster's record must be dropped, not retained forever")
}

// TestClientDepartedCancelsTheGraceTimer: no timer may publish for a departed
// client.
func TestClientDepartedCancelsTheGraceTimer(t *testing.T) {
	pt := newGraceTestTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going-away"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))
	require.NotEmpty(t, pt.gate.dereferenced[publishGateTestClient])

	pt.gate.clientDeparted(publishGateTestClient)

	assert.Empty(t, pt.gate.dereferenced, "a departed client leaves no armed timer behind")
}

// TestDereferenceGraceNeverResurrectsAnErroredCluster: carries fail closed, or a
// backend that failed validation keeps its pre-error config for the window.
func TestDereferenceGraceNeverResurrectsAnErroredCluster(t *testing.T) {
	pt := newGraceTestTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "now-errored"))

	deferredWrap := clusterWrapperV("v2", "kept")
	deferredWrap.erroredClusters = []string{"now-errored"}
	pt.syncXds(ctx, deferredWrap)

	assert.ElementsMatch(t, []string{"kept"}, publishedClusterNames(t, pt),
		"an errored cluster must not be carried forward by the grace window")
}

// TestGraceDereferencedClustersReportsSoonestExpiry: a later de-reference must
// not postpone an earlier window's removal.
func TestGraceDereferencedClustersReportsSoonestExpiry(t *testing.T) {
	g := newPublishGate(0, false, nil, scopedClustersWithGrace(time.Minute))
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: edsClusterProto("first-out")},
		{Resource: edsClusterProto("second-out")},
	})
	building := envoycache.Resources{Items: map[string]envoycachetypes.ResourceWithTTL{}}

	start := time.Now()
	graced, soonest := g.graceDereferencedClustersLocked(publishGateTestClient, published, building, nil, false, start)
	require.Len(t, graced, 2)
	require.InDelta(t, time.Minute, soonest, float64(time.Second))

	// 30s later the first window has 30s left; a third de-reference must not extend it.
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v2", []envoycachetypes.ResourceWithTTL{
		{Resource: edsClusterProto("first-out")},
		{Resource: edsClusterProto("second-out")},
		{Resource: edsClusterProto("third-out")},
	})
	_, soonest = g.graceDereferencedClustersLocked(publishGateTestClient, published, building, nil, false, start.Add(30*time.Second))
	assert.InDelta(t, 30*time.Second, soonest, float64(time.Second),
		"the timer must fire for the oldest window, not the newest")
}

// TestGraceDereferencedClustersDropsExpiredEntries: an expired cluster is no
// longer graced, so the re-publish removes it.
func TestGraceDereferencedClustersDropsExpiredEntries(t *testing.T) {
	g := newPublishGate(0, false, nil, scopedClustersWithGrace(time.Second))
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: edsClusterProto("going-away")},
	})
	building := envoycache.Resources{Items: map[string]envoycachetypes.ResourceWithTTL{}}

	start := time.Now()
	graced, _ := g.graceDereferencedClustersLocked(publishGateTestClient, published, building, nil, false, start)
	require.Len(t, graced, 1)

	graced, soonest := g.graceDereferencedClustersLocked(publishGateTestClient, published, building, nil, false, start.Add(2*time.Second))
	assert.Empty(t, graced, "past the window the cluster stops being published")
	assert.Zero(t, soonest, "with nothing graced there is nothing left to wake up for")
}

// TestGraceDereferencedClustersSkipsClustersNoLongerTranslated: only a cluster
// scoping stopped emitting is graced. One whose backend is gone leaves at once,
// as with scoping off.
func TestGraceDereferencedClustersSkipsClustersNoLongerTranslated(t *testing.T) {
	g := newPublishGate(0, false, nil, scopedClustersWithGrace(time.Minute))
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: edsClusterProto("scoped-out")},
		{Resource: edsClusterProto("deleted")},
	})
	building := envoycache.Resources{Items: map[string]envoycachetypes.ResourceWithTTL{}}
	translated := map[string]envoycachetypes.ResourceWithTTL{"scoped-out": {Resource: edsClusterProto("scoped-out")}}

	graced, _ := g.graceDereferencedClustersLocked(publishGateTestClient, published, building, translated, false, time.Now())
	assert.Equal(t, map[string]struct{}{"scoped-out": {}}, graced)
	assert.NotContains(t, g.dereferenced[publishGateTestClient].since, "deleted", "a gone cluster opens no window")
}

// TestDereferenceGraceRemovesADeletedBackendAtOnce: through the gate, a
// retargeted-away cluster is kept for the grace while a deleted backend's
// cluster leaves with the build that no longer translates it. A request-time
// selector routes by cluster name, so keeping it would keep the deleted
// destination routable for the whole grace.
func TestDereferenceGraceRemovesADeletedBackendAtOnce(t *testing.T) {
	pt := newGraceTestTranslator(t, time.Hour)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "retargeted", "deleted"))

	b2 := clusterWrapperV("v2", "kept")
	b2.translatedClusters = map[string]envoycachetypes.ResourceWithTTL{
		"kept":       {Resource: edsClusterProto("kept")},
		"retargeted": {Resource: edsClusterProto("retargeted")},
	}
	pt.syncXds(ctx, b2)

	assert.ElementsMatch(t, []string{"kept", "retargeted"}, publishedClusterNames(t, pt))
	assert.ElementsMatch(t, []string{"kept", "retargeted"}, publishedEndpointNames(t, pt),
		"the deleted cluster's assignment leaves with it")
}

// A graced cluster that errors leaves the client (fail closed), and so must its
// record, or its timer later re-publishes an older build over a newer one.
func TestDereferenceGraceForgetsAClusterTheCarryDropped(t *testing.T) {
	pt := newGraceTestTranslator(t, 100*time.Millisecond)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "now-errored"))
	b2 := clusterWrapperV("v2", "kept")
	b2.erroredClusters = []string{"now-errored"}
	pt.syncXds(ctx, b2)
	b3 := clusterWrapperV("v3", "kept")
	b3.erroredClusters = []string{"now-errored"}
	pt.syncXds(ctx, b3)
	require.Equal(t, "v3", servedSnapshot(t, pt).GetVersion(resourcev3.ListenerType))

	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, "v3", servedSnapshot(t, pt).GetVersion(resourcev3.ListenerType),
		"a grace timer must never re-publish an older build over a newer one")
	pt.gate.mu.Lock()
	defer pt.gate.mu.Unlock()
	assert.Nil(t, pt.gate.dereferenced[publishGateTestClient], "no record may outlive the cluster it was kept for")
}

// A stale record, already older than the grace, would also prune the cluster's
// next genuine de-reference at once.
func TestDereferenceGraceGivesARecoveredClusterAFreshWindow(t *testing.T) {
	pt := newGraceTestTranslator(t, 100*time.Millisecond)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "flaky"))
	b2 := clusterWrapperV("v2", "kept")
	b2.erroredClusters = []string{"flaky"}
	pt.syncXds(ctx, b2)
	time.Sleep(250 * time.Millisecond)

	pt.syncXds(ctx, clusterWrapperV("v3", "kept", "flaky")) // recovered, referenced again
	require.Contains(t, publishedClusterNames(t, pt), "flaky")
	pt.syncXds(ctx, clusterWrapperV("v4", "kept")) // de-referenced for real
	assert.Contains(t, publishedClusterNames(t, pt), "flaky", "a de-referenced cluster keeps its full window")
}

// A coherent publish carrying graced clusters must cancel a held flip's release,
// or it later re-publishes the older held build with an empty placeholder CLA.
func TestGracedCoherentPublishCancelsAPendingFlipRelease(t *testing.T) {
	translator := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, 100*time.Millisecond, true,
		scopedClustersWithGrace(time.Hour))
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()
	ucc := newTestUcc(publishGateTestClient)
	listeners := func(version string) envoycache.Resources {
		r := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
		r.Version = version
		return r
	}
	a := edsClusterForClient(ucc, "cluster-a", 1)
	b := edsClusterForClient(ucc, "cluster-b", 3)

	v1 := &envoycache.Snapshot{}
	v1.Resources[envoycachetypes.Listener] = listeners("v1")
	v1.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-a")
	v1.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-1", []envoycachetypes.ResourceWithTTL{a.Cluster.ResourceWithTTL()})
	v1.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-1", []envoycachetypes.ResourceWithTTL{endpointsForClient(ucc, "cluster-a", 2).Endpoints.ResourceWithTTL()})
	pt.syncXds(ctx, XdsSnapWrapper{snap: v1, proxyKey: publishGateTestClient})

	// The route flips to cluster-b before its endpoints are derived: held.
	v2 := &envoycache.Snapshot{}
	v2.Resources[envoycachetypes.Listener] = listeners("v2")
	v2.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-b")
	v2.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-2", []envoycachetypes.ResourceWithTTL{b.Cluster.ResourceWithTTL()})
	v2.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-2", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "cluster-b"}},
	})
	pt.syncXds(ctx, XdsSnapWrapper{
		snap: v2, proxyKey: publishGateTestClient,
		missingEndpointsReferenced: []string{"cluster-b"}, synthesizedEndpoints: []string{"cluster-b"},
	})
	require.Equal(t, "v1", servedSnapshot(t, pt).GetVersion(resourcev3.ListenerType), "flip held")

	// cluster-b derives its endpoints: coherent, with cluster-a graced.
	v3 := &envoycache.Snapshot{}
	v3.Resources[envoycachetypes.Listener] = listeners("v3")
	v3.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-b")
	v3.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-2", []envoycachetypes.ResourceWithTTL{b.Cluster.ResourceWithTTL()})
	v3.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-3", []envoycachetypes.ResourceWithTTL{
		endpointsForClient(ucc, "cluster-b", 4).Endpoints.ResourceWithTTL(),
	})
	pt.syncXds(ctx, XdsSnapWrapper{snap: v3, proxyKey: publishGateTestClient, endpointHashes: map[string]uint64{"cluster-b": 4}})
	require.Equal(t, "v3", servedSnapshot(t, pt).GetVersion(resourcev3.ListenerType))
	require.Contains(t, publishedClusterNames(t, pt), "cluster-a", "cluster-a is still graced")

	time.Sleep(400 * time.Millisecond) // past the held flip's 100ms release bound
	served := servedSnapshot(t, pt)
	assert.Equal(t, "v3", served.GetVersion(resourcev3.ListenerType), "an expired flip release must not re-publish the older held build")
	n, ok := endpointCount(served, "cluster-b")
	assert.True(t, ok && n == 1, "cluster-b keeps its derived endpoints")
}

// A grace closing while a flip onto an unready cluster is held must not
// release the flip, whatever the publish budget: with it 0 or negative there is
// no release record, and the prune still makes a new build's hold decision. The
// window opens with a publish that ships (cluster-gone dropped), so its prune
// runs during the hold; the held build itself opens none.
func TestGracePruneKeepsAHeldFlipHeld(t *testing.T) {
	for _, budget := range []time.Duration{time.Hour, 0, -time.Second} {
		t.Run(budget.String(), func(t *testing.T) {
			translator := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, budget, true,
				scopedClustersWithGrace(80*time.Millisecond))
			pt := &translator
			t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
			ctx := context.Background()
			ucc := newTestUcc(publishGateTestClient)
			listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
			oldCluster := edsClusterForClient(ucc, "cluster-old", 1)
			oldEndpoints := endpointsForClient(ucc, "cluster-old", 2)

			published := &envoycache.Snapshot{}
			published.Resources[envoycachetypes.Listener] = listeners
			published.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-old", "cluster-gone")
			published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-old", []envoycachetypes.ResourceWithTTL{
				oldCluster.Cluster.ResourceWithTTL(),
				edsClusterForClient(ucc, "cluster-gone", 5).Cluster.ResourceWithTTL(),
			})
			published.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-old", []envoycachetypes.ResourceWithTTL{
				oldEndpoints.Endpoints.ResourceWithTTL(),
				endpointsForClient(ucc, "cluster-gone", 6).Endpoints.ResourceWithTTL(),
			})
			pt.syncXds(ctx, XdsSnapWrapper{snap: published, proxyKey: publishGateTestClient})

			// The routes stop naming cluster-gone: its 80ms grace opens.
			dropped := &envoycache.Snapshot{}
			dropped.Resources[envoycachetypes.Listener] = listeners
			dropped.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-old")
			dropped.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-dropped", []envoycachetypes.ResourceWithTTL{
				oldCluster.Cluster.ResourceWithTTL(),
			})
			dropped.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-dropped", []envoycachetypes.ResourceWithTTL{
				oldEndpoints.Endpoints.ResourceWithTTL(),
			})
			pt.syncXds(ctx, XdsSnapWrapper{snap: dropped, proxyKey: publishGateTestClient})

			// Routes now name unready cluster-new: the flip is held.
			flip := &envoycache.Snapshot{}
			flip.Resources[envoycachetypes.Listener] = listeners
			flip.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-new")
			flip.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-new", []envoycachetypes.ResourceWithTTL{
				edsClusterForClient(ucc, "cluster-new", 3).Cluster.ResourceWithTTL(),
			})
			flip.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-new", []envoycachetypes.ResourceWithTTL{
				{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "cluster-new"}},
			})
			pt.syncXds(ctx, XdsSnapWrapper{
				snap: flip, proxyKey: publishGateTestClient,
				missingEndpointsReferenced: []string{"cluster-new"}, synthesizedEndpoints: []string{"cluster-new"},
			})
			require.True(t, snapshotReferencesCluster(servedSnapshot(t, pt), "cluster-old"), "the flip is held")

			require.Eventually(t, func() bool {
				pt.gate.mu.Lock()
				defer pt.gate.mu.Unlock()
				return pt.gate.dereferenced[publishGateTestClient] == nil
			}, 2*time.Second, 5*time.Millisecond, "cluster-gone's grace prune runs")
			served := servedSnapshot(t, pt)
			assert.True(t, snapshotReferencesCluster(served, "cluster-old"),
				"a flip onto an unready cluster stays held until it is ready or its release bound, not the 80ms grace")
			assert.False(t, snapshotReferencesCluster(served, "cluster-new"))
		})
	}
}

// A flip release that graces clusters must arm the prune, with its build, or
// they linger until a later build.
func TestFlipReleaseArmsTheGracePrune(t *testing.T) {
	pt := newGraceTestTranslator(t, 100*time.Millisecond)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	build := func(version, routeTo string, clusters ...string) XdsSnapWrapper {
		w := clusterWrapperV(version, clusters...)
		w.snap.Resources[envoycachetypes.Listener] = listeners
		w.snap.Resources[envoycachetypes.Route] = routeResourcesForClusters(routeTo)
		return w
	}
	pt.syncXds(ctx, build("v0", "cluster-old", "cluster-old"))

	pf := &pendingFlipRelease{wrap: build("v1", "cluster-new", "cluster-new")}
	pt.gate.mu.Lock()
	pt.gate.cancelDereferenceTimerLocked(publishGateTestClient)
	pt.gate.pendingFlips[publishGateTestClient] = pf
	pt.gate.mu.Unlock()
	pt.gate.fireFlipRelease(ctx, pt.xdsCache, publishGateTestClient, pf)

	pt.gate.mu.Lock()
	st := pt.gate.dereferenced[publishGateTestClient]
	require.NotNil(t, st, "the release graced cluster-old")
	assert.NotNil(t, st.timer, "the prune is armed")
	assert.NotNil(t, st.wrap.snap, "with the build it re-publishes")
	pt.gate.mu.Unlock()
	require.Contains(t, publishedClusterNames(t, pt), "cluster-old")

	require.Eventually(t, func() bool {
		return !slices.Contains(publishedClusterNames(t, pt), "cluster-old")
	}, 2*time.Second, 10*time.Millisecond, "cluster-old is removed when its grace closes")
}

// A prune callback already waiting on the lock when its timer was re-armed or
// cancelled must do nothing: its state or epoch is stale.
func TestStaleGracePruneCallbackDoesNothing(t *testing.T) {
	pt := newGraceTestTranslator(t, time.Hour)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))
	pt.gate.mu.Lock()
	state := pt.gate.dereferenced[publishGateTestClient]
	require.NotNil(t, state)
	staleEpoch := state.epoch
	pt.gate.mu.Unlock()

	pt.syncXds(ctx, clusterWrapperV("v3", "kept")) // re-arms: a new epoch
	assert.NotPanics(t, func() { pt.gate.fireDereferencePrune(ctx, pt.xdsCache, publishGateTestClient, state, staleEpoch) })
	assert.Equal(t, "v3", servedSnapshot(t, pt).GetVersion(resourcev3.ListenerType))
	assert.Contains(t, publishedClusterNames(t, pt), "going", "a stale callback must not prune")

	pt.gate.mu.Lock()
	pt.gate.cancelDereferenceTimerLocked(publishGateTestClient)
	pt.gate.dereferenced[publishGateTestClient] = &dereferenceState{since: map[string]time.Time{"going": time.Now()}}
	pt.gate.mu.Unlock()
	assert.NotPanics(t, func() { pt.gate.fireDereferencePrune(ctx, pt.xdsCache, publishGateTestClient, state, state.epoch) },
		"a callback whose state was replaced must not touch the new one")
}

// The grace path, like publish and resolveDeferred, skips a build whose row
// moved on while it was queued.
func TestDereferenceGracePathSkipsASupersededBuild(t *testing.T) {
	pt := newGraceTestTranslator(t, time.Minute)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	var current atomic.Pointer[XdsSnapWrapper]
	pt.gate.currentBuild = func(string) *XdsSnapWrapper { return current.Load() }
	ctx := t.Context()

	v1 := clusterWrapperV("v1", "backend")
	current.Store(&v1)
	pt.syncXds(ctx, v1)
	v2 := clusterWrapperV("v2", "backend")
	current.Store(&v2)
	pt.syncXds(ctx, v2)
	require.Equal(t, "v2", servedSnapshot(t, pt).GetVersion(resourcev3.ListenerType))

	pt.syncXds(ctx, v1) // queued before v2, delivered late
	assert.Equal(t, "v2", servedSnapshot(t, pt).GetVersion(resourcev3.ListenerType),
		"a superseded build must not replace the current one")
}
