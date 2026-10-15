package proxy_syncer

import (
	"fmt"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestClusterLoadAssignmentsEqualAgreesWithProtoEqual is the soundness check
// for the interner's identity-aware equality: over CLAs built the way
// production builds them (PrioritizeEndpoints, sharing LbEndpoint protos with
// the endpoint IR) and over deliberate variants of each one, the fast path must
// return exactly what proto.Equal returns. A disagreement in the "true"
// direction would alias two clients' distinct assignments; in the "false"
// direction it would only cost a missed share, but the contract is agreement.
func TestClusterLoadAssignmentsEqualAgreesWithProtoEqual(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")
	eps := ir.NewEndpointsForBackend(backend)
	for i, loc := range []ir.PodLocality{{Region: "r1", Zone: "z1"}, {Region: "r1", Zone: "z2"}, {Region: "r2", Zone: "z3"}} {
		eps.Add(loc, ir.EndpointWithMd{
			LbEndpoint: lbEndpointPipe(fmt.Sprintf("ep-%d", i)),
			EndpointMd: ir.EndpointMetadata{Labels: map[string]string{corev1.LabelZoneRegion: loc.Region, corev1.LabelTopologyZone: loc.Zone}},
		})
	}
	zonal := *eps
	zonal.TrafficDistribution = wellknown.TrafficDistributionPreferSameZone

	clients := []ir.UniquelyConnectedClient{
		ir.NewUniquelyConnectedClient("a", "ns", map[string]string{corev1.LabelZoneRegion: "r1", corev1.LabelTopologyZone: "z1"}, ir.PodLocality{Region: "r1", Zone: "z1"}),
		ir.NewUniquelyConnectedClient("b", "ns", map[string]string{corev1.LabelZoneRegion: "r1", corev1.LabelTopologyZone: "z1", "x": "y"}, ir.PodLocality{Region: "r1", Zone: "z1"}),
		ir.NewUniquelyConnectedClient("c", "ns", map[string]string{corev1.LabelZoneRegion: "r1", corev1.LabelTopologyZone: "z2"}, ir.PodLocality{Region: "r1", Zone: "z2"}),
	}

	// Production-shaped CLAs: built per client, sharing LbEndpoint pointers.
	var assignments []*envoyendpointv3.ClusterLoadAssignment
	for _, inputs := range []endpoints.EndpointsInputs{
		{EndpointsForBackend: *eps},
		{EndpointsForBackend: zonal},
		{EndpointsForBackend: *eps, PriorityInfo: &endpoints.PriorityInfo{}},
	} {
		for _, ucc := range clients {
			assignments = append(assignments, endpoints.PrioritizeEndpoints(nil, ucc, inputs))
		}
	}

	// Variants of the first CLA that differ in exactly one place each, including
	// places the fast path does not model directly and must fall back on.
	base := assignments[0]
	variant := func(mutate func(c *envoyendpointv3.ClusterLoadAssignment)) *envoyendpointv3.ClusterLoadAssignment {
		c := proto.Clone(base).(*envoyendpointv3.ClusterLoadAssignment)
		mutate(c)
		return c
	}
	assignments = append(assignments,
		proto.Clone(base).(*envoyendpointv3.ClusterLoadAssignment), // equal content, no shared pointers
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) { c.ClusterName = "other" }),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) { c.Endpoints[0].Priority = 7 }),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) {
			c.Endpoints[0].LoadBalancingWeight = wrapperspb.UInt32(99)
		}),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) { c.Endpoints[0].LoadBalancingWeight = nil }),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) {
			c.Endpoints[0].Locality = &envoycorev3.Locality{Region: "elsewhere"}
		}),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) { c.Endpoints[0].Locality = nil }),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) { c.Endpoints = c.Endpoints[:len(c.Endpoints)-1] }),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) { c.Endpoints[1].LbEndpoints = nil }),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) {
			c.Endpoints[0].LbEndpoints[0].GetEndpoint().GetAddress().GetPipe().Path = "moved"
		}),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) {
			c.Endpoints[0].LbEndpoints[0].LoadBalancingWeight = wrapperspb.UInt32(5)
		}),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) { c.Endpoints[0].Metadata = &envoycorev3.Metadata{} }),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) { c.Endpoints[0].Proximity = wrapperspb.UInt32(1) }),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) {
			c.Endpoints[0].LbConfig = &envoyendpointv3.LocalityLbEndpoints_LoadBalancerEndpoints{
				LoadBalancerEndpoints: &envoyendpointv3.LocalityLbEndpoints_LbEndpointList{},
			}
		}),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) {
			c.Policy = &envoyendpointv3.ClusterLoadAssignment_Policy{EndpointStaleAfter: durationpb.New(1)}
		}),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) {
			c.NamedEndpoints = map[string]*envoyendpointv3.Endpoint{"n": {}}
		}),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) {
			// An unknown field on the root: a future proto field this code
			// does not know about must still be compared.
			c.ProtoReflect().SetUnknown([]byte{0xf8, 0x7f, 0x01})
		}),
		variant(func(c *envoyendpointv3.ClusterLoadAssignment) {
			c.Endpoints[0].ProtoReflect().SetUnknown([]byte{0xf8, 0x7f, 0x01})
		}),
		nil,
	)

	agreed := 0
	for i, x := range assignments {
		for j, y := range assignments {
			want := proto.Equal(x, y)
			got := clusterLoadAssignmentsEqual(x, y)
			require.Equal(t, want, got, "pair (%d, %d): fast path %v, proto.Equal %v", i, j, got, want)
			if want {
				agreed++
			}
		}
	}
	// Vacuity guard: the equal pairs must include more than the diagonal, so
	// the "true" direction was exercised across distinct instances too.
	require.Greater(t, agreed, len(assignments), "expected equal pairs beyond each CLA with itself")
}
