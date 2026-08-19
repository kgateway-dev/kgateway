package proxy_syncer

import (
	"testing"
	"time"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/require"
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
// same locality for a zone-aware backend) share one ClusterLoadAssignment proto
// and trigger a single build, while a UCC in a different locality gets its own.
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

	buildCalls := 0
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
			buildCalls++
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

	// A and B share a locality -> same CLA proto, same hash, built once.
	require.True(t, sharedproto.Same(fetchedA[0].Endpoints, fetchedB[0].Endpoints),
		"equivalent contexts must share one interned CLA proto")
	require.Equal(t, fetchedA[0].EndpointsHash, fetchedB[0].EndpointsHash)
	// C is in a different zone -> distinct CLA and hash.
	require.False(t, sharedproto.Same(fetchedA[0].Endpoints, fetchedC[0].Endpoints),
		"distinct contexts must not share a CLA proto")
	require.NotEqual(t, fetchedA[0].EndpointsHash, fetchedC[0].EndpointsHash)
	// Two distinct load-balancing contexts -> exactly two builds across three UCCs.
	require.Equal(t, 2, buildCalls)
}

// TestNewPerClientEnvoyEndpointsKeysInterningOnResolvedEndpointHash verifies
// that a plugin-built replacement endpoint set participates in the interning
// key even when the plugin needs no separate additional hash.
func TestNewPerClientEnvoyEndpointsKeysInterningOnResolvedEndpointHash(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "default", Name: "backend"}, 80, "", "")
	backendEndpoints := ir.NewEndpointsForBackend(backend)
	backendEndpoints.Add(ir.PodLocality{}, ir.EndpointWithMd{LbEndpoint: lbEndpointPipe("base")})

	uccA := ir.NewUniquelyConnectedClient("a", "ns", nil, ir.PodLocality{})
	uccB := ir.NewUniquelyConnectedClient("b", "ns", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{uccA, uccB}, krtopts.ToOptions("UniqueClients")...)
	endpointsCol := krt.NewStaticCollection(nil, []ir.EndpointsForBackend{*backendEndpoints}, krtopts.ToOptions("Endpoints")...)

	buildCalls := 0
	perClient := NewPerClientEnvoyEndpoints(
		krtopts,
		uccs,
		endpointsCol,
		func(_ krt.HandlerContext, ucc ir.UniquelyConnectedClient, ep ir.EndpointsForBackend) translator.ResolvedEndpoints {
			resolved := ep.EmptyCopy()
			resolved.Add(ir.PodLocality{}, ir.EndpointWithMd{LbEndpoint: lbEndpointPipe(ucc.Role)})
			return translator.ResolvedEndpoints{Inputs: endpoints.EndpointsInputs{EndpointsForBackend: resolved}}
		},
		func(ucc ir.UniquelyConnectedClient, resolved translator.ResolvedEndpoints) *envoyendpointv3.ClusterLoadAssignment {
			buildCalls++
			return endpoints.PrioritizeEndpoints(nil, ucc, resolved.Inputs)
		},
	)

	var fetchedA, fetchedB []UccWithEndpoints
	require.Eventually(t, func() bool {
		fetchedA = perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, uccA)
		fetchedB = perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, uccB)
		return len(fetchedA) == 1 && len(fetchedB) == 1
	}, time.Second, 20*time.Millisecond)

	require.False(t, sharedproto.Same(fetchedA[0].Endpoints, fetchedB[0].Endpoints),
		"different resolved endpoint sets must not share an interned CLA")
	require.NotEqual(t, fetchedA[0].EndpointsHash, fetchedB[0].EndpointsHash)
	require.Equal(t, 2, buildCalls)
}

