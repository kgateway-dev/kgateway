package proxy_syncer

import (
	"sync/atomic"
	"testing"
	"time"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// TestNewPerClientEnvoyEndpointsSharesClaAcrossEquivalentContexts verifies the
// CLA interning: UCCs that resolve to the same load-balancing context (e.g. the
// same locality for a zone-aware backend) share one ClusterLoadAssignment proto,
// while a UCC in a different locality gets its own.
func TestNewPerClientEnvoyEndpointsSharesClaAcrossEquivalentContexts(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "core",
		Kind:      "Service",
		Namespace: "default",
		Name:      "backend",
	}, 80, "", "")
	backend.TrafficDistribution = wellknown.TrafficDistributionPreferSameZone

	backendEndpoints := ir.NewEndpointsForBackend(backend)
	backendEndpoints.Add(ir.PodLocality{Region: "r1", Zone: "z1"}, ir.EndpointWithMd{
		LbEndpoint: lbEndpointPipe("same-zone"),
		EndpointMd: ir.EndpointMetadata{
			Labels: map[string]string{
				corev1.LabelZoneRegion:   "r1",
				corev1.LabelTopologyZone: "z1",
			},
		},
	})
	backendEndpoints.Add(ir.PodLocality{Region: "r1", Zone: "z2"}, ir.EndpointWithMd{
		LbEndpoint: lbEndpointPipe("other-zone"),
		EndpointMd: ir.EndpointMetadata{
			Labels: map[string]string{
				corev1.LabelZoneRegion:   "r1",
				corev1.LabelTopologyZone: "z2",
			},
		},
	})

	// PreferSameZone keys on the proxy's topology labels (not PodLocality), so the
	// zone/region must live in the UCC labels. A and B share a zone (differing only
	// in an irrelevant custom label); C is in a different zone.
	uccA := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{
		corev1.LabelZoneRegion:   "r1",
		corev1.LabelTopologyZone: "z1",
		"custom":                 "a",
	}, ir.PodLocality{Region: "r1", Zone: "z1"})
	uccB := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{
		corev1.LabelZoneRegion:   "r1",
		corev1.LabelTopologyZone: "z1",
		"custom":                 "b",
	}, ir.PodLocality{Region: "r1", Zone: "z1"})
	uccC := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{
		corev1.LabelZoneRegion:   "r1",
		corev1.LabelTopologyZone: "z2",
		"custom":                 "c",
	}, ir.PodLocality{Region: "r1", Zone: "z2"})

	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{uccA, uccB, uccC}, krtopts.ToOptions("UniqueClients")...)
	endpointsCol := krt.NewStaticCollection(nil, []ir.EndpointsForBackend{*backendEndpoints}, krtopts.ToOptions("Endpoints")...)

	// The build hook runs on KRT's transform goroutine; count atomically.
	var buildCalls atomic.Int32
	perClient := NewPerClientEnvoyEndpoints(
		krtopts,
		uccs,
		endpointsCol,
		func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient, ep ir.EndpointsForBackend) translator.ResolvedEndpoints {
			inputs := endpoints.EndpointsInputs{EndpointsForBackend: ep}
			return translator.ResolvedEndpoints{
				Inputs:            inputs,
				LoadBalancingHash: endpoints.LoadBalancingContextHash(ucc, inputs),
			}
		},
		func(ucc ir.UniquelyConnectedClient, resolved translator.ResolvedEndpoints) *envoyendpointv3.ClusterLoadAssignment {
			buildCalls.Add(1)
			return endpoints.PrioritizeEndpoints(nil, ucc, resolved.Inputs)
		},
	)

	var fetchedA, fetchedB, fetchedC []UccWithEndpoints
	require.Eventually(t, func() bool {
		fetchedA = perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, uccA)
		fetchedB = perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, uccB)
		fetchedC = perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, uccC)
		return len(fetchedA) == 1 && len(fetchedB) == 1 && len(fetchedC) == 1
	}, time.Second, 20*time.Millisecond)

	// A and B share the topology labels PreferSameZone keys on -> same CLA proto,
	// same hash.
	require.True(t, sharedproto.Same(fetchedA[0].Endpoints, fetchedB[0].Endpoints),
		"equivalent contexts must share one interned CLA proto")
	require.Equal(t, fetchedA[0].EndpointsHash, fetchedB[0].EndpointsHash)
	// C is in a different zone -> distinct hash, distinct proto, and a CLA whose
	// priorities genuinely differ (z2 endpoints are preferred for C, z1 for A).
	require.NotEqual(t, fetchedA[0].EndpointsHash, fetchedC[0].EndpointsHash)
	require.False(t, sharedproto.Same(fetchedA[0].Endpoints, fetchedC[0].Endpoints),
		"distinct contexts must not share a CLA proto")
	require.False(t, proto.Equal(fetchedA[0].Endpoints.Clone(), fetchedC[0].Endpoints.Clone()),
		"a client in another zone must be built a different CLA, not merely a different proto instance")
	// Every client is built so the interner can confirm content equality instead
	// of trusting the 64-bit input hash; equal results are then shared in memory.
	require.EqualValues(t, 3, buildCalls.Load())
}

// TestNewPerClientEnvoyEndpointsDoesNotAliasAcrossLocalities covers the other
// load-balancing-context axis: locality failover keys on PodLocality rather than
// labels. Two clients with identical labels in different localities must not
// share a CLA, while a third client co-located with the first must.
func TestNewPerClientEnvoyEndpointsDoesNotAliasAcrossLocalities(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "default", Name: "backend"}, 80, "", "")
	backendEndpoints := ir.NewEndpointsForBackend(backend)
	backendEndpoints.Add(ir.PodLocality{Region: "r1", Zone: "z1"}, ir.EndpointWithMd{LbEndpoint: lbEndpointPipe("z1")})
	backendEndpoints.Add(ir.PodLocality{Region: "r1", Zone: "z2"}, ir.EndpointWithMd{LbEndpoint: lbEndpointPipe("z2")})

	labels := map[string]string{"app": "same"}
	inZ1 := ir.NewUniquelyConnectedClient("z1", "ns", labels, ir.PodLocality{Region: "r1", Zone: "z1"})
	inZ2 := ir.NewUniquelyConnectedClient("z2", "ns", labels, ir.PodLocality{Region: "r1", Zone: "z2"})
	alsoZ1 := ir.NewUniquelyConnectedClient("z1-too", "ns", labels, ir.PodLocality{Region: "r1", Zone: "z1"})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{inZ1, inZ2, alsoZ1}, krtopts.ToOptions("UniqueClients")...)
	endpointsCol := krt.NewStaticCollection(nil, []ir.EndpointsForBackend{*backendEndpoints}, krtopts.ToOptions("Endpoints")...)

	perClient := NewPerClientEnvoyEndpoints(
		krtopts,
		uccs,
		endpointsCol,
		func(_ krt.HandlerContext, ucc ir.UniquelyConnectedClient, ep ir.EndpointsForBackend) translator.ResolvedEndpoints {
			// Mimic a destination rule's localityLbSetting: locality failover with
			// no failover-priority labels, so PodLocality is the discriminator.
			inputs := endpoints.EndpointsInputs{EndpointsForBackend: ep, PriorityInfo: &endpoints.PriorityInfo{}}
			return translator.ResolvedEndpoints{
				Inputs:            inputs,
				LoadBalancingHash: endpoints.LoadBalancingContextHash(ucc, inputs),
			}
		},
		func(ucc ir.UniquelyConnectedClient, resolved translator.ResolvedEndpoints) *envoyendpointv3.ClusterLoadAssignment {
			return endpoints.PrioritizeEndpoints(nil, ucc, resolved.Inputs)
		},
	)

	var fetchedZ1, fetchedZ2, fetchedAlsoZ1 []UccWithEndpoints
	require.Eventually(t, func() bool {
		fetchedZ1 = perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, inZ1)
		fetchedZ2 = perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, inZ2)
		fetchedAlsoZ1 = perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, alsoZ1)
		return len(fetchedZ1) == 1 && len(fetchedZ2) == 1 && len(fetchedAlsoZ1) == 1
	}, time.Second, 20*time.Millisecond)

	require.True(t, sharedproto.Same(fetchedZ1[0].Endpoints, fetchedAlsoZ1[0].Endpoints),
		"co-located clients with equal labels must share one interned CLA")
	require.NotEqual(t, fetchedZ1[0].EndpointsHash, fetchedZ2[0].EndpointsHash,
		"clients that differ only in PodLocality must not collide on the interning key")
	require.False(t, sharedproto.Same(fetchedZ1[0].Endpoints, fetchedZ2[0].Endpoints),
		"clients in different localities must not share a CLA proto")
	require.False(t, proto.Equal(fetchedZ1[0].Endpoints.Clone(), fetchedZ2[0].Endpoints.Clone()),
		"locality failover must assign different priorities for clients in different zones")
}

// TestNewPerClientEnvoyEndpointsDoesNotAliasHashCollisions forces two different
// resolved endpoint sets into the same hash bucket and verifies that the built
// CLAs remain distinct.
func TestNewPerClientEnvoyEndpointsDoesNotAliasHashCollisions(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "default", Name: "backend"}, 80, "", "")
	backendEndpoints := ir.NewEndpointsForBackend(backend)
	backendEndpoints.Add(ir.PodLocality{}, ir.EndpointWithMd{LbEndpoint: lbEndpointPipe("base")})

	uccA := ir.NewUniquelyConnectedClient("a", "ns", nil, ir.PodLocality{})
	uccB := ir.NewUniquelyConnectedClient("b", "ns", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{uccA, uccB}, krtopts.ToOptions("UniqueClients")...)
	endpointsCol := krt.NewStaticCollection(nil, []ir.EndpointsForBackend{*backendEndpoints}, krtopts.ToOptions("Endpoints")...)

	// The build hook runs on KRT's transform goroutine; count atomically.
	var buildCalls atomic.Int32
	perClient := NewPerClientEnvoyEndpoints(
		krtopts,
		uccs,
		endpointsCol,
		func(_ krt.HandlerContext, ucc ir.UniquelyConnectedClient, ep ir.EndpointsForBackend) translator.ResolvedEndpoints {
			resolved := ep.EmptyCopy()
			resolved.Add(ir.PodLocality{}, ir.EndpointWithMd{LbEndpoint: lbEndpointPipe(ucc.Role)})
			// Force a collision in the complete interning key. The old map keyed
			// only by this hash returned uccA's CLA for both clients.
			resolved.LbEpsEqualityHash = 42
			return translator.ResolvedEndpoints{Inputs: endpoints.EndpointsInputs{EndpointsForBackend: resolved}}
		},
		func(ucc ir.UniquelyConnectedClient, resolved translator.ResolvedEndpoints) *envoyendpointv3.ClusterLoadAssignment {
			buildCalls.Add(1)
			return endpoints.PrioritizeEndpoints(nil, ucc, resolved.Inputs)
		},
	)

	var fetchedA, fetchedB []UccWithEndpoints
	require.Eventually(t, func() bool {
		fetchedA = perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, uccA)
		fetchedB = perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, uccB)
		return len(fetchedA) == 1 && len(fetchedB) == 1
	}, time.Second, 20*time.Millisecond)

	// Establish the collision first: if the setup stopped colliding, the
	// non-aliasing assertions below would pass for the wrong reason.
	require.Equal(t, fetchedA[0].EndpointsHash, fetchedB[0].EndpointsHash,
		"precondition: the resolved inputs must collide on the interning hash")
	require.False(t, sharedproto.Same(fetchedA[0].Endpoints, fetchedB[0].Endpoints),
		"different CLAs in one hash bucket must not share an interned proto")
	require.NotEqual(t,
		fetchedA[0].Endpoints.Clone().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetPipe().GetPath(),
		fetchedB[0].Endpoints.Clone().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetPipe().GetPath(),
		"each client must retain its own resolved endpoint")
	require.EqualValues(t, 2, buildCalls.Load())
}
