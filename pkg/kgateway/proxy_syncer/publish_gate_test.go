package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const publishGateTestClient = "c1"

// newTestUcc builds a minimal UCC; fixtures ignore client identity.
func newTestUcc(name string) ir.UniquelyConnectedClient {
	return ir.NewUniquelyConnectedClient(name, "", nil, ir.PodLocality{})
}

// stubPriorXDS: has=false is a cold client, has=true a warm one (prior version).
type stubPriorXDS struct{ has bool }

func (s stubPriorXDS) HasPriorXDSVersion(string) bool { return s.has }

func newPublishGateTestTranslator(t *testing.T, prior bool, budget time.Duration) *ProxyTranslator {
	pt := NewProxyTranslator(
		newTestSnapshotCache(t),
		stubPriorXDS{has: prior},
		budget,
		true,
		clusterScoping{},
	)
	// Stop timers at cleanup; a warm hold would otherwise re-arm forever.
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	return &pt
}

// deferredWrapperV: the listener version names the build; a missing reference
// defers it.
func deferredWrapperV(version string) XdsSnapWrapper {
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Listener] = envoycache.NewResources(version, nil)
	return XdsSnapWrapper{
		snap:              snap,
		proxyKey:          publishGateTestClient,
		missingReferenced: []string{"cluster-missing"},
	}
}

// deferredMissingEndpointsWrapperV's only gap is a referenced cluster with no
// derived CLA (a synthesized empty stands in), as for an EDS cluster from a
// plugin that contributes no endpoint rows. A Service without EndpointSlices,
// ExternalName included, derives an empty CLA and is no gap.
func deferredMissingEndpointsWrapperV(version string) XdsSnapWrapper {
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Listener] = envoycache.NewResources(version, nil)
	return XdsSnapWrapper{
		snap:                       snap,
		proxyKey:                   publishGateTestClient,
		missingEndpointsReferenced: []string{"cluster-underived"},
	}
}

func coherentWrapperV(version string) XdsSnapWrapper {
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Listener] = envoycache.NewResources(version, nil)
	return XdsSnapWrapper{snap: snap, proxyKey: publishGateTestClient}
}

func publishedListenerVersion(t *testing.T, pt *ProxyTranslator) (string, bool) {
	t.Helper()
	snap, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	if err != nil {
		return "", false
	}
	return snap.GetVersion(resourcev3.ListenerType), true
}

// A cold client waits out the budget, then is published so the pod can start.
func TestFirstPublish_ColdClientPublishesAtBudget(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 50*time.Millisecond)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))

	_, ok := publishedListenerVersion(t, pt)
	assert.False(t, ok, "deferred snapshot must not publish before the budget expires")

	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 2*time.Second, 5*time.Millisecond, "deferred snapshot must publish after the budget expires")
	v, _ := publishedListenerVersion(t, pt)
	assert.Equal(t, "v1", v)
}

// At budget expiry the latest deferred build publishes.
func TestFirstPublish_LatestDeferredWins(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 80*time.Millisecond)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	pt.syncXds(context.Background(), deferredWrapperV("v2"))

	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 2*time.Second, 5*time.Millisecond)
	v, _ := publishedListenerVersion(t, pt)
	assert.Equal(t, "v2", v)
}

// A warm client with clusters missing from CDS stays withheld at expiry, since
// publishing would NC routes it serves; a coherent build publishes at once.
func TestFirstPublish_PriorXDSVersionClientStaysWithheld(t *testing.T) {
	pt := newPublishGateTestTranslator(t, true, 40*time.Millisecond)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))

	require.Never(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 250*time.Millisecond, 20*time.Millisecond,
		"a prior-xDS-version (warm reconnect) client must not receive a deferred snapshot with missing clusters at budget expiry")

	pt.syncXds(context.Background(), coherentWrapperV("coherent"))
	v, ok := publishedListenerVersion(t, pt)
	require.True(t, ok, "a coherent snapshot publishes immediately, warm or not")
	assert.Equal(t, "coherent", v)
}

// A warm client whose only gaps are underived CLAs publishes at budget expiry:
// they may never converge (#14352), and withholding would freeze its config.
func TestFirstPublish_WarmClientPublishesEndpointTruthAtBudget(t *testing.T) {
	pt := newPublishGateTestTranslator(t, true, 50*time.Millisecond)

	pt.syncXds(context.Background(), deferredMissingEndpointsWrapperV("v1"))

	_, ok := publishedListenerVersion(t, pt)
	assert.False(t, ok, "the deferred snapshot must not publish before the budget expires")

	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 2*time.Second, 5*time.Millisecond,
		"a warm client whose only gaps are underived-CLA clusters must be published at budget expiry")
	v, _ := publishedListenerVersion(t, pt)
	assert.Equal(t, "v1", v)
}

// Missing clusters keep a warm client withheld even alongside underived CLAs.
func TestFirstPublish_WarmClientMixedGapsStaysWithheld(t *testing.T) {
	pt := newPublishGateTestTranslator(t, true, 40*time.Millisecond)

	wrapper := deferredWrapperV("v1")
	wrapper.missingEndpointsReferenced = []string{"cluster-underived"}
	pt.syncXds(context.Background(), wrapper)

	require.Never(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 250*time.Millisecond, 20*time.Millisecond,
		"a warm client must stay withheld while any referenced cluster is missing from CDS, even if other gaps are underived CLAs")
}

// A coherent build cancels the pending first publish; its timer must not
// overwrite it.
func TestFirstPublish_CoherentSupersedesPending(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 100*time.Millisecond)

	pt.syncXds(context.Background(), deferredWrapperV("deferred"))
	pt.syncXds(context.Background(), coherentWrapperV("coherent"))

	v, ok := publishedListenerVersion(t, pt)
	require.True(t, ok)
	assert.Equal(t, "coherent", v)

	time.Sleep(250 * time.Millisecond) // well past the budget
	v, _ = publishedListenerVersion(t, pt)
	assert.Equal(t, "coherent", v, "a canceled first-publish timer must not overwrite a coherent snapshot")
}

// A departed client's pending bounded publish must not fire.
func TestFirstPublish_DepartureCancelsPending(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 50*time.Millisecond)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	pt.gate.clientDeparted(publishGateTestClient)

	time.Sleep(200 * time.Millisecond)
	_, ok := publishedListenerVersion(t, pt)
	assert.False(t, ok, "a departed client's pending first publish must not fire")
}

// KGW_PER_CLIENT_PUBLISH_BUDGET=0 disables the first-publication deadline.
func TestFirstPublish_BudgetZeroDisablesBound(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 0)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))

	require.Never(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 250*time.Millisecond, 20*time.Millisecond,
		"with the bound disabled, a deferred snapshot must never publish to a never-published client")
}

// --- flip-release bound ---

// flipHoldFixture publishes routes to cluster-old and returns a wrapper flipping
// them to underived cluster-new; syncing it holds the flip.
func flipHoldFixture(t *testing.T, pt *ProxyTranslator) (heldRouteVersion string, flipWrap XdsSnapWrapper) {
	t.Helper()

	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})

	oldCluster := edsClusterForClient(
		newTestUcc(publishGateTestClient), "cluster-old", 1,
	)
	oldCLA := endpointsForClient(newTestUcc(publishGateTestClient), "cluster-old", 2)

	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Listener] = listeners
	published.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-old")
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-old", []envoycachetypes.ResourceWithTTL{
		oldCluster.Cluster.ResourceWithTTL(),
	})
	published.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-old", []envoycachetypes.ResourceWithTTL{
		oldCLA.Endpoints.ResourceWithTTL(),
	})
	require.NoError(t, pt.gate.publish(context.Background(), pt.xdsCache, XdsSnapWrapper{snap: published, proxyKey: publishGateTestClient}))

	// Retarget to cluster-new before its CLA derives.
	newCluster := edsClusterForClient(newTestUcc(publishGateTestClient), "cluster-new", 3)
	flipSnap := &envoycache.Snapshot{}
	flipSnap.Resources[envoycachetypes.Listener] = listeners
	flipSnap.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-new")
	flipSnap.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-new", []envoycachetypes.ResourceWithTTL{
		oldCluster.Cluster.ResourceWithTTL(),
		newCluster.Cluster.ResourceWithTTL(),
	})
	flipSnap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-new", []envoycachetypes.ResourceWithTTL{
		oldCLA.Endpoints.ResourceWithTTL(),
		emptyEndpointsForClient(newTestUcc(publishGateTestClient), "cluster-new", 4).Endpoints.ResourceWithTTL(),
	})
	flipWrap = XdsSnapWrapper{
		snap:     flipSnap,
		proxyKey: publishGateTestClient,
		// Mirror the synthesized placeholder for cluster-new's underived CLA.
		missingEndpointsReferenced: []string{"cluster-new"},
	}
	return published.GetVersion(resourcev3.RouteType), flipWrap
}

func servedSnapshot(t *testing.T, pt *ProxyTranslator) *envoycache.Snapshot {
	t.Helper()
	resourceSnapshot, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	require.NoError(t, err)
	snap, ok := resourceSnapshot.(*envoycache.Snapshot)
	require.True(t, ok)
	return snap
}

// At expiry the held flip publishes; routes to the unready cluster fail rather
// than pinning every route, listener and secret update (#14352).
func TestFlipRelease_HeldFlipPublishesAtBudget(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 50*time.Millisecond)
	heldRouteVersion, flipWrap := flipHoldFixture(t, pt)

	pt.syncXds(context.Background(), flipWrap)

	held := servedSnapshot(t, pt)
	assert.Equal(t, heldRouteVersion, held.GetVersion(resourcev3.RouteType),
		"the flip must be held at the published route version before the budget expires")
	assert.True(t, hasResource(held.Resources[envoycachetypes.Cluster].Items, "cluster-new"),
		"the warming cluster's CDS still publishes during the hold")

	require.Eventually(t, func() bool {
		released := servedSnapshot(t, pt)
		return snapshotReferencesCluster(released, "cluster-new")
	}, 2*time.Second, 5*time.Millisecond,
		"the held flip must publish at budget expiry")
	released := servedSnapshot(t, pt)
	assertSnapshotCoherent(t, released)
}

// A released missing-CDS flip must not hold later route/listener/secret updates.
func TestFlipRelease_MissingClusterDoesNotHoldSubsequentUpdates(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	heldRouteVersion, flipWrap := flipHoldFixture(t, pt)
	delete(flipWrap.snap.Resources[envoycachetypes.Cluster].Items, "cluster-new")
	delete(flipWrap.snap.Resources[envoycachetypes.Endpoint].Items, "cluster-new")
	flipWrap.missingEndpointsReferenced = nil
	flipWrap.missingReferenced = []string{"cluster-new"}

	pt.syncXds(context.Background(), flipWrap)
	require.Equal(t, heldRouteVersion, servedSnapshot(t, pt).GetVersion(resourcev3.RouteType))

	// Invoke expiry directly to avoid timing-dependent assertions.
	pt.gate.mu.Lock()
	pending := pt.gate.pendingFlips[publishGateTestClient]
	if pending != nil {
		pending.timer.Stop()
	}
	pt.gate.mu.Unlock()
	require.NotNil(t, pending)
	pt.gate.fireFlipRelease(context.Background(), pt.xdsCache, publishGateTestClient, pending)
	released := servedSnapshot(t, pt)
	require.True(t, snapshotReferencesCluster(released, "cluster-new"))
	require.NotContains(t, released.GetResources(resourcev3.ClusterType), "cluster-new")

	for _, version := range []string{"update-1", "update-2"} {
		updated := *flipWrap.snap
		updated.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-new", "cluster-old")
		for _, rt := range []envoycachetypes.ResponseType{envoycachetypes.Route, envoycachetypes.Listener, envoycachetypes.Secret} {
			updated.Resources[rt].Version = version
		}
		flipWrap.snap = &updated
		pt.syncXds(context.Background(), flipWrap)

		published := servedSnapshot(t, pt)
		for _, typeURL := range []resourcev3.Type{resourcev3.RouteType, resourcev3.ListenerType, resourcev3.SecretType} {
			assert.Equal(t, version, published.GetVersion(typeURL), "subsequent %s update must publish immediately", typeURL)
		}
		assert.NotContains(t, published.GetResources(resourcev3.ClusterType), "cluster-new")
		pt.gate.mu.Lock()
		assert.Empty(t, pt.gate.pendingFlips, "a released missing cluster must not arm another hold")
		pt.gate.mu.Unlock()
	}
}

// A build that resolves the flip cancels the pending release; the timer must not
// overwrite it.
func TestFlipRelease_ResolvedFlipCancelsPending(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 100*time.Millisecond)
	_, flipWrap := flipHoldFixture(t, pt)

	pt.syncXds(context.Background(), flipWrap)

	// Resolve the same flip before expiry.
	resolved := flipWrap
	resolved.missingEndpointsReferenced = nil
	resolvedSnap := &envoycache.Snapshot{}
	*resolvedSnap = *flipWrap.snap
	resolvedSnap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-ready", []envoycachetypes.ResourceWithTTL{
		endpointsForClient(newTestUcc(publishGateTestClient), "cluster-old", 5).Endpoints.ResourceWithTTL(),
		endpointsForClient(newTestUcc(publishGateTestClient), "cluster-new", 6).Endpoints.ResourceWithTTL(),
	})
	resolved.snap = resolvedSnap
	pt.syncXds(context.Background(), resolved)

	require.Equal(t, "eds-ready", servedSnapshot(t, pt).GetVersion(resourcev3.EndpointType))
	time.Sleep(250 * time.Millisecond) // well past the budget
	assert.Equal(t, "eds-ready", servedSnapshot(t, pt).GetVersion(resourcev3.EndpointType),
		"a canceled flip-release timer must not overwrite the resolved snapshot")
}

// A departed client's pending flip release must not fire.
func TestFlipRelease_DepartureCancelsPending(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 50*time.Millisecond)
	heldRouteVersion, flipWrap := flipHoldFixture(t, pt)

	pt.syncXds(context.Background(), flipWrap)
	pt.gate.clientDeparted(publishGateTestClient)

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, heldRouteVersion, servedSnapshot(t, pt).GetVersion(resourcev3.RouteType),
		"a departed client's pending flip release must not fire")
}

// KGW_PER_CLIENT_PUBLISH_BUDGET=0 holds flips until resolution.
func TestFlipRelease_BudgetZeroDisablesBound(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 0)
	heldRouteVersion, flipWrap := flipHoldFixture(t, pt)

	pt.syncXds(context.Background(), flipWrap)

	require.Never(t, func() bool {
		return servedSnapshot(t, pt).GetVersion(resourcev3.RouteType) != heldRouteVersion
	}, 250*time.Millisecond, 20*time.Millisecond,
		"with the bound disabled, a held flip must never release on a timer")
}

// --- connection scope, stale timers, synthesized CLAs ---

// A returning client is not resolved against its pre-departure cache entry,
// which may be stale. It goes through first publish, whose bound still fires
// although that entry exists.
func TestFirstPublish_ReconnectIgnoresEntryFromPreviousConnection(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 100*time.Millisecond)
	ctx := context.Background()

	previous := coherentWrapperV("v0")
	previous.snap.Resources[envoycachetypes.Cluster] = envoycache.NewResources("cds-v0", nil)
	pt.syncXds(ctx, previous)
	pt.gate.clientDeparted(publishGateTestClient)

	next := deferredWrapperV("v1")
	next.snap.Resources[envoycachetypes.Cluster] = envoycache.NewResources("cds-v1", nil)
	pt.syncXds(ctx, next)

	served := servedSnapshot(t, pt)
	assert.Equal(t, "cds-v0", served.GetVersion(resourcev3.ClusterType),
		"a deferred snapshot must not be resolved against the previous connection's entry")
	require.Eventually(t, func() bool {
		return servedSnapshot(t, pt).GetVersion(resourcev3.ListenerType) == "v1"
	}, 2*time.Second, 5*time.Millisecond,
		"the bounded first publish must fire although the cache holds the previous connection's entry")
}

// A first-publish callback that waited on the lock across episodes must not
// publish the next episode early.
func TestFirstPublish_StaleTimerDoesNotPublishLaterEpisode(t *testing.T) {
	budget := 300 * time.Millisecond
	pt := newPublishGateTestTranslator(t, false, budget)
	ctx := context.Background()
	pt.syncXds(ctx, deferredWrapperV("v1"))

	pt.gate.mu.Lock()
	time.Sleep(budget + 100*time.Millisecond) // the first timer fired and waits on the lock
	delete(pt.gate.pending, publishGateTestClient)
	pt.gate.offerColdLocked(ctx, pt.xdsCache, deferredWrapperV("v2"))
	pt.gate.mu.Unlock()

	require.Never(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, budget/2, 10*time.Millisecond, "the earlier episode's timer must not publish the later episode")
	require.Eventually(t, func() bool {
		v, ok := publishedListenerVersion(t, pt)
		return ok && v == "v2"
	}, 2*time.Second, 5*time.Millisecond, "the later episode publishes at its own budget")
}

// An old flip-release callback blocked on the lock must not release a newer
// episode early.
func TestFlipRelease_StaleTimerDoesNotReleaseLaterEpisode(t *testing.T) {
	budget := 300 * time.Millisecond
	pt := newPublishGateTestTranslator(t, false, budget)
	_, flipWrap := flipHoldFixture(t, pt)
	ctx := context.Background()
	published := servedSnapshot(t, pt)
	pt.syncXds(ctx, flipWrap)

	pt.gate.mu.Lock()
	time.Sleep(budget + 100*time.Millisecond) // the first release timer fired and waits on the lock
	require.NoError(t, pt.gate.publishLocked(ctx, pt.xdsCache, publishGateTestClient, published, nil, nil))
	current := pt.gate.connectionSnapshotLocked(pt.xdsCache, publishGateTestClient)
	require.NotNil(t, current)
	held, blocking, hashes := resolveDeferredPerCluster(flipWrap, current, true, nil, pt.gate.lastPublishedEDSLocked(publishGateTestClient))
	require.NotEmpty(t, blocking)
	require.NoError(t, pt.gate.publishHeldLocked(ctx, pt.xdsCache, flipWrap, held, blocking, nil, 0, hashes, time.Now()))
	pt.gate.mu.Unlock()

	require.Never(t, func() bool {
		return snapshotReferencesCluster(servedSnapshot(t, pt), "cluster-new")
	}, budget/2, 10*time.Millisecond, "the earlier episode's timer must not release the later hold")
	require.Eventually(t, func() bool {
		return snapshotReferencesCluster(servedSnapshot(t, pt), "cluster-new")
	}, 2*time.Second, 5*time.Millisecond, "the later hold releases at its own budget")
}

// A cold sibling lifts the hold without replacing warm siblings' endpoints.
func TestFirstPublish_SharedColdClientPreservesWarmEndpoints(t *testing.T) {
	pt := newSharedKeyTestTranslator(t, true, 20*time.Millisecond, time.Minute)
	wrap := deferredWrapperV("v1")
	const cluster = "cluster-underived"
	ucc := newTestUcc(publishGateTestClient)
	wrap.snap.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds", []envoycachetypes.ResourceWithTTL{
		edsClusterForClient(ucc, cluster, 1).Cluster.ResourceWithTTL(),
	})
	wrap.snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: cluster}},
	})
	wrap.synthesizedEndpoints = []string{cluster}
	pt.syncXds(t.Context(), wrap)
	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, time.Second, 5*time.Millisecond, "the cold sibling must lift the missing-cluster hold")
	snap, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	require.NoError(t, err)
	require.NotContains(t, snap.GetResources(resourcev3.EndpointType), cluster,
		"omit the synthesized assignment to preserve the warm siblings' endpoints")
}

func TestSettleSynthesizedEndpoints(t *testing.T) {
	ucc := newTestUcc(publishGateTestClient)
	edsClusters := func(names ...string) envoycache.Resources {
		items := make([]envoycachetypes.ResourceWithTTL, 0, len(names))
		for _, name := range names {
			items = append(items, edsClusterForClient(ucc, name, 1).Cluster.ResourceWithTTL())
		}
		return envoycache.NewResourcesWithTTL("cds", items)
	}
	placeholder := func(name string) envoycachetypes.ResourceWithTTL {
		return envoycachetypes.ResourceWithTTL{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: name}}
	}
	live := func(name string) envoycachetypes.ResourceWithTTL {
		return endpointsForClient(ucc, name, 1).Endpoints.ResourceWithTTL()
	}

	// Published: cluster-known with endpoints, cluster-left-out without a CLA.
	// cluster-new is new; cluster-derived is derived, so never settled.
	build := &envoycache.Snapshot{}
	build.Resources[envoycachetypes.Cluster] = edsClusters("cluster-known", "cluster-left-out", "cluster-new", "cluster-derived")
	build.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds", []envoycachetypes.ResourceWithTTL{
		placeholder("cluster-known"), placeholder("cluster-left-out"), placeholder("cluster-new"), placeholder("cluster-derived"),
	})
	synthesized := []string{"cluster-known", "cluster-left-out", "cluster-new"}
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = edsClusters("cluster-known", "cluster-left-out", "cluster-derived")
	published.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-published", []envoycachetypes.ResourceWithTTL{
		live("cluster-known"), live("cluster-derived"),
	})

	// served maps each CLA name in the settled snapshot to its endpoint count.
	served := func(snap *envoycache.Snapshot) map[string]int {
		out := map[string]int{}
		for name := range snap.Resources[envoycachetypes.Endpoint].Items {
			out[name], _ = endpointCount(snap, name)
		}
		return out
	}

	t.Run("published on this connection", func(t *testing.T) {
		settled := settleSynthesizedEndpoints(build, synthesized, published)
		assert.Equal(t, map[string]int{"cluster-known": 1, "cluster-new": 0, "cluster-derived": 0}, served(settled),
			"known endpoints are republished, a cluster published without a CLA is left alone, a new cluster gets its empty CLA, derived truth is untouched")
		assert.NotEqual(t, build.GetVersion(resourcev3.EndpointType), settled.GetVersion(resourcev3.EndpointType))
		assert.Len(t, build.Resources[envoycachetypes.Endpoint].Items, 4, "the built snapshot must not be mutated")
		assert.NoError(t, snapshotConsistencyError(publishGateTestClient, settled))
	})
	t.Run("first publish on a connection", func(t *testing.T) {
		// Warm or cold, a first publish leaves these out: a reconnect keeps its
		// endpoints, and a new proxy waits rather than going Ready with zero hosts.
		settled := settleSynthesizedEndpoints(build, synthesized, nil)
		assert.Equal(t, map[string]int{"cluster-derived": 0}, served(settled),
			"synthesized CLAs whose rows are on their way are left out of a connection's first publish")
	})
	t.Run("nothing synthesized", func(t *testing.T) {
		assert.Same(t, build, settleSynthesizedEndpoints(build, nil, published))
	})
}

// Row withdrawal while connected preserves publication state. When the row
// returns, carry the missing cluster rather than use bounded first publication.
// Actual departure clears that state.
func TestSnapshotRowFlapKeepsConnectionState(t *testing.T) {
	ucc := newTestUcc(publishGateTestClient)
	full := func() *envoycache.Snapshot {
		snap := &envoycache.Snapshot{}
		snap.Resources[envoycachetypes.Listener] = sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
		snap.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-a", "cluster-b")
		snap.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-1", []envoycachetypes.ResourceWithTTL{
			edsClusterForClient(ucc, "cluster-a", 1).Cluster.ResourceWithTTL(),
			edsClusterForClient(ucc, "cluster-b", 2).Cluster.ResourceWithTTL(),
		})
		snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-1", []envoycachetypes.ResourceWithTTL{
			endpointsForClient(ucc, "cluster-a", 3).Endpoints.ResourceWithTTL(),
			endpointsForClient(ucc, "cluster-b", 4).Endpoints.ResourceWithTTL(),
		})
		return snap
	}
	lagging := func() XdsSnapWrapper {
		snap := full()
		snap.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-2", []envoycachetypes.ResourceWithTTL{
			edsClusterForClient(ucc, "cluster-a", 1).Cluster.ResourceWithTTL(),
		})
		snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-2", []envoycachetypes.ResourceWithTTL{
			endpointsForClient(ucc, "cluster-a", 3).Endpoints.ResourceWithTTL(),
		})
		return XdsSnapWrapper{snap: snap, proxyKey: publishGateTestClient, missingReferenced: []string{"cluster-b"}}
	}
	ctx := context.Background()

	t.Run("row deleted while connected", func(t *testing.T) {
		pt := newPublishGateTestTranslator(t, false, time.Hour)
		pt.syncXds(ctx, XdsSnapWrapper{snap: full(), proxyKey: publishGateTestClient})
		pt.clientSnapshotDeleted(publishGateTestClient, true)
		pt.syncXds(ctx, lagging())
		served := servedSnapshot(t, pt)
		assert.NotEqual(t, "cds-1", served.GetVersion(resourcev3.ClusterType), "the returning row is resolved and published at once")
		assert.Contains(t, served.Resources[envoycachetypes.Cluster].Items, "cluster-b", "the cluster the client serves is carried")
		n, ok := endpointCount(served, "cluster-b")
		assert.True(t, ok && n == 1, "and so are its endpoints")
	})
	t.Run("client departed", func(t *testing.T) {
		pt := newPublishGateTestTranslator(t, false, time.Hour)
		pt.syncXds(ctx, XdsSnapWrapper{snap: full(), proxyKey: publishGateTestClient})
		pt.clientSnapshotDeleted(publishGateTestClient, false)
		pt.syncXds(ctx, lagging())
		assert.Equal(t, "cds-1", servedSnapshot(t, pt).GetVersion(resourcev3.ClusterType),
			"a returning client goes through first publish; nothing is resolved against its previous connection")
	})
}

// A cold client that reconnects within the budget resumes its episode, so
// frequent resets still get a bounded first publish; a longer absence starts a
// new one.
func TestFirstPublish_BudgetResumesAcrossBriefDepartures(t *testing.T) {
	const budget = 400 * time.Millisecond
	ctx := context.Background()
	published := func(pt *ProxyTranslator) bool {
		_, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
		return err == nil
	}

	t.Run("brief departure resumes", func(t *testing.T) {
		pt := newPublishGateTestTranslator(t, false, budget)
		pt.syncXds(ctx, deferredWrapperV("v1"))
		time.Sleep(budget * 3 / 4)
		pt.gate.clientDeparted(publishGateTestClient)
		pt.syncXds(ctx, deferredWrapperV("v2"))
		require.Eventually(t, func() bool { return published(pt) }, budget*5/8, 5*time.Millisecond,
			"the episode's remaining budget, not a fresh one, bounds the first publish after a reconnect")
		assert.Equal(t, "v2", servedSnapshot(t, pt).GetVersion(resourcev3.ListenerType))
	})
	t.Run("long departure restarts", func(t *testing.T) {
		pt := newPublishGateTestTranslator(t, false, budget)
		pt.syncXds(ctx, deferredWrapperV("v1"))
		time.Sleep(budget / 4)
		pt.gate.clientDeparted(publishGateTestClient)
		time.Sleep(budget * 5 / 4)
		pt.syncXds(ctx, deferredWrapperV("v2"))
		time.Sleep(budget / 2)
		assert.False(t, published(pt), "a client gone for longer than the budget starts a fresh episode")
		require.Eventually(t, func() bool { return published(pt) }, budget, 5*time.Millisecond)
	})
}

// installThenFailCache installs each snapshot, then returns go-control-plane's
// open-watch error.
type installThenFailCache struct {
	envoycache.SnapshotCache
}

func (c installThenFailCache) SetSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot) error {
	if err := c.SnapshotCache.SetSnapshot(ctx, node, snapshot); err != nil {
		return err
	}
	return context.Canceled
}

// An installed snapshot is served even if answering a watch failed, so
// bookkeeping follows installation: the first publish counts and a held flip
// keeps its release bound.
func TestSetSnapshotErrorAfterInstallKeepsBookkeeping(t *testing.T) {
	ctx := context.Background()
	newTranslator := func(budget time.Duration) *ProxyTranslator {
		pt := NewProxyTranslator(installThenFailCache{newTestSnapshotCache(t)}, stubPriorXDS{}, budget, true, clusterScoping{})
		return &pt
	}
	t.Run("bounded first publish", func(t *testing.T) {
		pt := newTranslator(10 * time.Millisecond)
		pt.syncXds(ctx, deferredWrapperV("v1"))
		require.Eventually(t, func() bool {
			pt.gate.mu.Lock()
			defer pt.gate.mu.Unlock()
			_, published := pt.gate.published[publishGateTestClient]
			return published
		}, 2*time.Second, 5*time.Millisecond, "the installed bounded publish counts as published on this connection")
	})
	t.Run("held flip", func(t *testing.T) {
		pt := newTranslator(time.Hour)
		t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
		_, flipWrap := flipHoldFixture(t, pt)
		pt.syncXds(ctx, flipWrap)
		pt.gate.mu.Lock()
		defer pt.gate.mu.Unlock()
		assert.NotNil(t, pt.gate.pendingFlips[publishGateTestClient], "the installed hold still gets its release bound")
	})
}

// A missing referenced cluster is carried (make-before-break) only up to the
// warm withhold bound; then it is dropped and its routes fail.
func TestCarryOfAMissingClusterIsBounded(t *testing.T) {
	ResetMetrics()
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	pt.gate.warmWithholdBound = 100 * time.Millisecond
	ucc := newTestUcc(publishGateTestClient)
	ctx := context.Background()
	build := func(version string, clusters ...string) *envoycache.Snapshot {
		snap := &envoycache.Snapshot{}
		listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
		listeners.Version = version
		snap.Resources[envoycachetypes.Listener] = listeners
		snap.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-a", "cluster-b")
		var clusterItems, endpointItems []envoycachetypes.ResourceWithTTL
		var hash uint64
		for _, name := range clusters {
			hash += 2
			clusterItems = append(clusterItems, edsClusterForClient(ucc, name, hash-1).Cluster.ResourceWithTTL())
			endpointItems = append(endpointItems, endpointsForClient(ucc, name, hash).Endpoints.ResourceWithTTL())
		}
		snap.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-"+version, clusterItems)
		snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-"+version, endpointItems)
		return snap
	}
	lagging := func(version string) XdsSnapWrapper {
		return XdsSnapWrapper{snap: build(version, "cluster-a"), proxyKey: publishGateTestClient, missingReferenced: []string{"cluster-b"}}
	}

	pt.syncXds(ctx, XdsSnapWrapper{snap: build("v1", "cluster-a", "cluster-b"), proxyKey: publishGateTestClient})
	pt.syncXds(ctx, lagging("v2"))
	require.Contains(t, servedSnapshot(t, pt).Resources[envoycachetypes.Cluster].Items, "cluster-b", "a lagging cluster is carried")

	time.Sleep(150 * time.Millisecond)
	pt.syncXds(ctx, lagging("v3"))
	served := servedSnapshot(t, pt)
	assert.Equal(t, "v3", served.GetVersion(resourcev3.ListenerType))
	assert.NotContains(t, served.Resources[envoycachetypes.Cluster].Items, "cluster-b", "past the bound it is no longer carried")
	_, hasCLA := endpointCount(served, "cluster-b")
	assert.False(t, hasCLA, "nor are its endpoints")

	g := metricstest.MustGatherMetrics(t)
	assert.Equal(t, float64(1), g.MustGetMetricValueByLabels("kgateway_xds_snapshot_perclient_bounded_publishes_total",
		labelsWith("mode", boundedPublishCarryExpired)))
}

// carryTestBuild builds a snapshot routing to routeTargets with cluster/CLA
// pairs for clusters.
func carryTestBuild(t *testing.T, version string, routeTargets []string, clusters ...string) *envoycache.Snapshot {
	t.Helper()
	ucc := newTestUcc(publishGateTestClient)
	snap := &envoycache.Snapshot{}
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	listeners.Version = version
	snap.Resources[envoycachetypes.Listener] = listeners
	routes := routeResourcesForClusters(routeTargets...)
	routes.Version = "rds-" + version
	snap.Resources[envoycachetypes.Route] = routes
	var clusterItems, endpointItems []envoycachetypes.ResourceWithTTL
	var hash uint64
	for _, name := range clusters {
		hash += 2
		clusterItems = append(clusterItems, edsClusterForClient(ucc, name, hash-1).Cluster.ResourceWithTTL())
		endpointItems = append(endpointItems, endpointsForClient(ucc, name, hash).Endpoints.ResourceWithTTL())
	}
	snap.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-"+version, clusterItems)
	snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-"+version, endpointItems)
	return snap
}

// Enforce carry expiry even when a deleted backend produces no further builds.
func TestCarryExpiresWithoutAnotherBuild(t *testing.T) {
	ResetMetrics()
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	pt.gate.warmWithholdBound = 20 * time.Millisecond
	ctx := context.Background()
	routed := []string{"cluster-a", "cluster-b"}

	pt.syncXds(ctx, XdsSnapWrapper{snap: carryTestBuild(t, "v1", routed, "cluster-a", "cluster-b"), proxyKey: publishGateTestClient})
	pt.syncXds(ctx, XdsSnapWrapper{
		snap: carryTestBuild(t, "v2", routed, "cluster-a"), proxyKey: publishGateTestClient,
		missingReferenced: []string{"cluster-b"},
	})
	require.Contains(t, servedSnapshot(t, pt).Resources[envoycachetypes.Cluster].Items, "cluster-b", "a lagging cluster is carried")

	require.Eventually(t, func() bool {
		_, carried := servedSnapshot(t, pt).Resources[envoycachetypes.Cluster].Items["cluster-b"]
		return !carried
	}, 2*time.Second, 5*time.Millisecond, "the carry ends at the bound with no further input")
	served := servedSnapshot(t, pt)
	assert.Equal(t, "v2", served.GetVersion(resourcev3.ListenerType), "the latest build is what is re-published")
	_, hasCLA := endpointCount(served, "cluster-b")
	assert.False(t, hasCLA, "the cluster's endpoints go with it")
	g := metricstest.MustGatherMetrics(t)
	assert.Equal(t, float64(1), g.MustGetMetricValueByLabels("kgateway_xds_snapshot_perclient_bounded_publishes_total",
		labelsWith("mode", boundedPublishCarryExpired)))
}

// Coherent recovery ends the carry episode, even outside deferred resolution, so
// a later disappearance gets a fresh bound.
func TestCarryStartsFreshAfterTheClusterRecovers(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	pt.gate.warmWithholdBound = 80 * time.Millisecond
	ctx := context.Background()
	routed := []string{"cluster-a", "cluster-b"}
	lagging := func(version string) XdsSnapWrapper {
		return XdsSnapWrapper{
			snap: carryTestBuild(t, version, routed, "cluster-a"), proxyKey: publishGateTestClient,
			missingReferenced: []string{"cluster-b"},
		}
	}

	pt.syncXds(ctx, XdsSnapWrapper{snap: carryTestBuild(t, "v1", routed, "cluster-a", "cluster-b"), proxyKey: publishGateTestClient})
	pt.syncXds(ctx, lagging("v2"))
	time.Sleep(30 * time.Millisecond)
	// Recover in a coherent build.
	pt.syncXds(ctx, XdsSnapWrapper{snap: carryTestBuild(t, "v3", routed, "cluster-a", "cluster-b"), proxyKey: publishGateTestClient})
	time.Sleep(70 * time.Millisecond) // past the first episode's bound

	pt.syncXds(ctx, lagging("v4"))
	served := servedSnapshot(t, pt)
	assert.Equal(t, "v4", served.GetVersion(resourcev3.ListenerType))
	assert.Contains(t, served.Resources[envoycachetypes.Cluster].Items, "cluster-b",
		"a second disappearance is carried for a fresh bound")
	require.Eventually(t, func() bool {
		_, carried := servedSnapshot(t, pt).Resources[envoycachetypes.Cluster].Items["cluster-b"]
		return !carried
	}, 2*time.Second, 5*time.Millisecond, "and still ends at its own bound")
}

// Carry expiry must not restore a build superseded by a later publication.
func TestCarryExpiryNeverRepublishesAnOlderBuild(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	pt.gate.warmWithholdBound = 20 * time.Millisecond
	ctx := context.Background()

	pt.syncXds(ctx, XdsSnapWrapper{snap: carryTestBuild(t, "v1", []string{"cluster-a", "cluster-b"}, "cluster-a", "cluster-b"), proxyKey: publishGateTestClient})
	pt.syncXds(ctx, XdsSnapWrapper{
		snap: carryTestBuild(t, "v2", []string{"cluster-a", "cluster-b"}, "cluster-a"), proxyKey: publishGateTestClient,
		missingReferenced: []string{"cluster-b"},
	})
	// A coherent build removes the missing target.
	pt.syncXds(ctx, XdsSnapWrapper{snap: carryTestBuild(t, "v3", []string{"cluster-a"}, "cluster-a"), proxyKey: publishGateTestClient})

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, "v3", servedSnapshot(t, pt).GetVersion(resourcev3.ListenerType), "the superseded build is not re-published")
}

// A held closure must not re-carry expired clusters, or their bounds restart.
func TestHeldFlipDoesNotCarryAnExpiredCluster(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	pt.gate.warmWithholdBound = 50 * time.Millisecond
	ctx := context.Background()

	pt.syncXds(ctx, XdsSnapWrapper{snap: carryTestBuild(t, "v1", []string{"cluster-a", "cluster-b"}, "cluster-a", "cluster-b"), proxyKey: publishGateTestClient})
	pt.syncXds(ctx, XdsSnapWrapper{
		snap: carryTestBuild(t, "v2", []string{"cluster-a", "cluster-b"}, "cluster-a"), proxyKey: publishGateTestClient,
		missingReferenced: []string{"cluster-b"},
	})
	pt.gate.mu.Lock()
	pt.gate.cancelCarryExpiryLocked(publishGateTestClient) // let the held build below observe expiry
	pt.gate.mu.Unlock()
	time.Sleep(60 * time.Millisecond)

	// Past the bound, a build flips onto a never-seen cluster, so the flip and its
	// closure are held.
	pt.syncXds(ctx, XdsSnapWrapper{
		snap:              carryTestBuild(t, "v3", []string{"cluster-a", "cluster-b", "cluster-new"}, "cluster-a"),
		proxyKey:          publishGateTestClient,
		missingReferenced: []string{"cluster-b", "cluster-new"},
	})
	served := servedSnapshot(t, pt)
	assert.Equal(t, "v2", served.GetVersion(resourcev3.ListenerType), "the flip onto the new cluster is held")
	assert.Contains(t, served.Resources[envoycachetypes.Cluster].Items, "cluster-a", "the held closure is carried")
	assert.NotContains(t, served.Resources[envoycachetypes.Cluster].Items, "cluster-b", "but not a cluster carried for the bound")
	_, hasCLA := endpointCount(served, "cluster-b")
	assert.False(t, hasCLA)
}

// The routing-target walk is memoized by map identity: retained maps reuse it,
// and different content under a reused version label is walked again.
func TestPublishedReferencedClustersIsMemoizedByIdentity(t *testing.T) {
	memo := &referencedClustersMemo{}
	routesA := routeResourcesForClusters("cluster-a")
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	walks := 0
	refsOf := func(routes envoycache.Resources) map[string]struct{} {
		return memo.get(routes.Items, listeners.Items, func() map[string]struct{} {
			walks++
			return collectReferencedClusters(routes, listeners)
		})
	}

	first := refsOf(routesA)
	again := refsOf(routesA)
	assert.Equal(t, map[string]struct{}{"cluster-a": {}}, first)
	assert.Equal(t, first, again)
	assert.Equal(t, 1, walks, "the same routing config is walked once")

	routesB := routeResourcesForClusters("cluster-b")
	routesB.Version = routesA.Version // a reused label must not alias different content
	assert.Equal(t, map[string]struct{}{"cluster-b": {}}, refsOf(routesB))
	assert.Equal(t, 2, walks)
}

// Publication tests model connected clients; their snapshots cannot be reclaimed.
func (stubPriorXDS) RunIfDisconnected(string, func()) bool { return false }
