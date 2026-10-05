package endpoints

import (
	"strconv"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestPrioritizeEndpointsAlwaysSetsLocality pins that a group whose endpoints have
// no locality is still emitted with a (empty) Locality. Envoy only records a
// group's load_balancing_weight when the group has a locality, so under
// locality-weighted LB a group without one gets weight 0 and receives no traffic
// while any other locality has endpoints.
func TestPrioritizeEndpointsAlwaysSetsLocality(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{
		Group: "networking.istio.io", Kind: "ServiceEntry", Namespace: "ns", Name: "se",
	}, 80, "", "")
	backendEndpoints := ir.NewEndpointsForBackend(backend)
	localities := []ir.PodLocality{
		{},
		{Region: "r1", Zone: "z1"},
	}
	for i, locality := range localities {
		backendEndpoints.Add(locality, prioritizeTestEndpoint(locality, i))
	}

	client := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{
		corev1.LabelTopologyRegion: "r1",
		corev1.LabelTopologyZone:   "z1",
	}, ir.PodLocality{Region: "r1", Zone: "z1"})

	priorityModes := map[string]*PriorityInfo{
		"noPriorityInfo": nil,
		"failoverPriority": {
			FailoverPriority: NewPriorities([]string{corev1.LabelTopologyRegion, corev1.LabelTopologyZone}),
		},
		"localityFailover": {},
	}

	for name, priorityInfo := range priorityModes {
		t.Run(name, func(t *testing.T) {
			inputs := EndpointsInputs{EndpointsForBackend: *backendEndpoints, PriorityInfo: priorityInfo}
			cla := PrioritizeEndpoints(nil, client, inputs)

			// Round-trip through the wire format: Envoy sees field presence, not
			// the Go pointer, so an empty Locality must survive serialization.
			raw, err := proto.Marshal(cla)
			require.NoError(t, err)
			decoded := &envoyendpointv3.ClusterLoadAssignment{}
			require.NoError(t, proto.Unmarshal(raw, decoded))

			require.Len(t, decoded.GetEndpoints(), len(localities))
			for _, group := range decoded.GetEndpoints() {
				assert.NotNil(t, group.GetLocality(),
					"every locality group must carry a Locality so Envoy keeps its weight; got %v", group)
				assert.NotZero(t, group.GetLoadBalancingWeight().GetValue(),
					"every locality group must carry a load balancing weight; got %v", group)
			}
		})
	}
}

func prioritizeTestEndpoint(locality ir.PodLocality, index int) ir.EndpointWithMd {
	return ir.EndpointWithMd{
		LbEndpoint: &envoyendpointv3.LbEndpoint{
			HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{Endpoint: &envoyendpointv3.Endpoint{
				Address: &envoycorev3.Address{Address: &envoycorev3.Address_SocketAddress{SocketAddress: &envoycorev3.SocketAddress{
					Address: "10.0.0." + strconv.Itoa(index+1),
				}}},
			}},
			LoadBalancingWeight: wrapperspb.UInt32(1),
		},
		EndpointMd: ir.EndpointMetadata{Labels: map[string]string{
			corev1.LabelTopologyRegion: locality.Region,
			corev1.LabelTopologyZone:   locality.Zone,
		}},
	}
}
