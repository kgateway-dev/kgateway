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

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const publishGateTestClient = "c1"

// newTestUcc builds a minimal UCC; the cluster/endpoint fixtures only consume
// the proto they wrap, not the client identity.
func newTestUcc(name string) ir.UniquelyConnectedClient {
	return ir.NewUniquelyConnectedClient(name, "", nil, ir.PodLocality{})
}

// stubPriorXDS models the client-state reader: has=false is a cold client,
// has=true a warm reconnect that reported a prior accepted version.
type stubPriorXDS struct{ has bool }

func (s stubPriorXDS) HasPriorXDSVersion(string) bool { return s.has }

func newPublishGateTestTranslator(t *testing.T, prior bool, budget time.Duration) *ProxyTranslator {
	pt := NewProxyTranslator(
		newTestSnapshotCache(t),
		stubPriorXDS{has: prior},
		budget,
		true,
	)
	return &pt
}

// deferredWrapperV builds a deferred wrapper whose listener version identifies
// it in assertions; the missing reference is what keeps it deferred.
func deferredWrapperV(version string) XdsSnapWrapper {
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Listener] = envoycache.NewResources(version, nil)
	return XdsSnapWrapper{
		snap:              snap,
		proxyKey:          publishGateTestClient,
		missingReferenced: []string{"cluster-missing"},
	}
}

// deferredMissingEndpointsWrapperV builds a deferred wrapper whose only gap is
// a referenced cluster with no derived CLA — the steady-state shape of an EDS
// cluster whose plugin has no endpoints source, such as an AWS EC2 Backend with
// discovery disabled (#14352): every referenced cluster is present in CDS, but
// one's ClusterLoadAssignment was never derived (a synthesized empty stands
// in). A Service without EndpointSlices, ExternalName included, derives an
// empty CLA instead and is not a gap at all.
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

// A cold, never-published client is withheld within the budget and published
// at expiry so the pod can start.
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

// The latest deferred snapshot is the one published at budget expiry.
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

// A client that reported a prior accepted xDS version is warm: while clusters
// are missing from CDS it must stay withheld at budget expiry (publishing
// routes to absent clusters would NC config it is already serving), but a
// coherent snapshot still publishes immediately.
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

// A warm client whose only gaps are clusters with no derived CLA publishes at
// budget expiry: that gap has no convergence guarantee (#14352), and
// withholding would freeze the client's config indefinitely after a
// controller restart.
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

// A warm client with BOTH missing and underived-CLA gaps stays withheld: the
// missing clusters dominate (publishing would NC their routes).
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

// A coherent snapshot supersedes a pending bounded publish, and the canceled
// timer must never overwrite it.
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

// KGW_PER_CLIENT_PUBLISH_BUDGET=0 is the conservative opt-out: never-published
// clients are withheld with no deadline.
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

// flipHoldFixture drives a published client into a held route flip: the
// published snapshot routes to cluster-old, then a deferred wrapper flips the
// routes onto cluster-new, which is newly referenced and has no derived CLA
// (synthesized empty), so resolution holds routes/listeners/secrets at the
// published versions.
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

	// The flip: routes now target cluster-new, whose CLA was never derived.
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
		// cluster-new's CLA was never derived: the empty CLA in the snapshot
		// mirrors the synthesized placeholder the transform would emit.
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

// A held route flip is released at budget expiry: the new routes publish and
// the still-unready cluster's routes fail until it becomes ready, instead of
// pinning every route/listener/secret update forever (#14352).
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

// Releasing a flip onto a cluster absent from CDS must also unblock later
// route/listener/secret updates while that cluster remains missing.
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

	// Fire the release directly to exercise expiry without timing-dependent assertions.
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

// A build that resolves the flip (endpoints derived, snapshot coherent)
// cancels the pending release; the expired timer must not overwrite it.
func TestFlipRelease_ResolvedFlipCancelsPending(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 100*time.Millisecond)
	_, flipWrap := flipHoldFixture(t, pt)

	pt.syncXds(context.Background(), flipWrap)

	// The gap resolves before expiry: the same flip arrives coherent.
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

// KGW_PER_CLIENT_PUBLISH_BUDGET=0 also disables the flip-release bound: the
// hold lasts until the flip resolves.
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

// A client that left and came back is not resolved against the snapshot the
// gate published before it left: the entry may predate what the client runs.
// It is withheld as a first publish instead, and that bound still fires even
// though the cache still holds the old entry.
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

// A first-publish timer whose callback waited on the gate lock while its
// episode ended and a new one began must not publish the new episode early.
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

// A flip-release timer whose callback waited on the gate lock while its
// episode ended and a new hold began must not release the new hold early.
func TestFlipRelease_StaleTimerDoesNotReleaseLaterEpisode(t *testing.T) {
	budget := 300 * time.Millisecond
	pt := newPublishGateTestTranslator(t, false, budget)
	_, flipWrap := flipHoldFixture(t, pt)
	ctx := context.Background()
	published := servedSnapshot(t, pt)
	pt.syncXds(ctx, flipWrap)

	pt.gate.mu.Lock()
	time.Sleep(budget + 100*time.Millisecond) // the first release timer fired and waits on the lock
	require.NoError(t, pt.gate.publishLocked(ctx, pt.xdsCache, publishGateTestClient, published, nil))
	current := pt.gate.connectionSnapshotLocked(pt.xdsCache, publishGateTestClient)
	require.NotNil(t, current)
	held, blocking := resolveDeferredPerCluster(flipWrap, current, true)
	require.NotEmpty(t, blocking)
	require.NoError(t, pt.gate.publishHeldLocked(ctx, pt.xdsCache, flipWrap, held, blocking))
	pt.gate.mu.Unlock()

	require.Never(t, func() bool {
		return snapshotReferencesCluster(servedSnapshot(t, pt), "cluster-new")
	}, budget/2, 10*time.Millisecond, "the earlier episode's timer must not release the later hold")
	require.Eventually(t, func() bool {
		return snapshotReferencesCluster(servedSnapshot(t, pt), "cluster-new")
	}, 2*time.Second, 5*time.Millisecond, "the later hold releases at its own budget")
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

	// cluster-known had endpoints published; cluster-left-out was published
	// without a CLA; cluster-new was never published; cluster-derived has a
	// derived (empty) CLA, which is truth and is never settled.
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
		settled := settleSynthesizedEndpoints(build, synthesized, published, false)
		assert.Equal(t, map[string]int{"cluster-known": 1, "cluster-new": 0, "cluster-derived": 0}, served(settled),
			"known endpoints are republished, a cluster published without a CLA is left alone, a new cluster gets its empty CLA, derived truth is untouched")
		assert.NotEqual(t, build.GetVersion(resourcev3.EndpointType), settled.GetVersion(resourcev3.EndpointType))
		assert.Len(t, build.Resources[envoycachetypes.Endpoint].Items, 4, "the built snapshot must not be mutated")
		assert.NoError(t, snapshotConsistencyError(publishGateTestClient, settled))
	})
	t.Run("first publish to a warm client", func(t *testing.T) {
		settled := settleSynthesizedEndpoints(build, synthesized, nil, true)
		assert.Equal(t, map[string]int{"cluster-derived": 0}, served(settled),
			"a warm client may hold endpoints from an earlier stream, so synthesized CLAs are left out")
	})
	t.Run("first publish to a cold client", func(t *testing.T) {
		assert.Same(t, build, settleSynthesizedEndpoints(build, synthesized, nil, false),
			"a cold client holds nothing, so synthesized CLAs publish as-is")
	})
	t.Run("nothing synthesized", func(t *testing.T) {
		assert.Same(t, build, settleSynthesizedEndpoints(build, nil, published, true))
	})
}
