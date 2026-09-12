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
		deferred:          true,
		missingReferenced: []string{"cluster-missing"},
	}
}

// deferredMissingEndpointsWrapperV builds a deferred wrapper whose only gap is
// a referenced cluster with no derived CLA — the steady-state shape of an
// ExternalName backend (#14352): every referenced cluster is present in CDS,
// but one's ClusterLoadAssignment was never derived (a synthesized empty
// stands in).
func deferredMissingEndpointsWrapperV(version string) XdsSnapWrapper {
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Listener] = envoycache.NewResources(version, nil)
	return XdsSnapWrapper{
		snap:                       snap,
		proxyKey:                   publishGateTestClient,
		deferred:                   true,
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
// budget expiry: that gap is the backends' steady state (ExternalName —
// #14352), and withholding would freeze the client's config indefinitely
// after a controller restart.
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
	require.NoError(t, pt.gate.publish(context.Background(), pt.xdsCache, publishGateTestClient, published))

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
		deferred: true,
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

// A build that resolves the flip (endpoints derived, snapshot coherent)
// cancels the pending release; the expired timer must not overwrite it.
func TestFlipRelease_ResolvedFlipCancelsPending(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 100*time.Millisecond)
	_, flipWrap := flipHoldFixture(t, pt)

	pt.syncXds(context.Background(), flipWrap)

	// The gap resolves before expiry: the same flip arrives coherent.
	resolved := flipWrap
	resolved.deferred = false
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
