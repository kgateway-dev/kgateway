package endpoints

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func testEndpoints(trafficDistribution wellknown.TrafficDistribution) ir.EndpointsForBackend {
	return ir.EndpointsForBackend{
		ClusterName:         "test-cluster",
		TrafficDistribution: trafficDistribution,
		LbEps: ir.LocalityLbMap{
			{Region: "r1", Zone: "z1"}: []ir.EndpointWithMd{
				{
					LbEndpoint: &envoyendpointv3.LbEndpoint{
						HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
							Endpoint: &envoyendpointv3.Endpoint{
								Address: &envoycorev3.Address{
									Address: &envoycorev3.Address_SocketAddress{
										SocketAddress: &envoycorev3.SocketAddress{Address: "1.2.3.4"},
									},
								},
							},
						},
					},
					EndpointMd: ir.EndpointMetadata{
						Labels: map[string]string{
							corev1.LabelZoneRegion:   "r1",
							corev1.LabelTopologyZone: "z1",
						},
					},
				},
			},
		},
	}
}

func client(zone string, labels map[string]string) ir.UniquelyConnectedClient {
	return ir.UniquelyConnectedClient{
		Labels:   labels,
		Locality: ir.PodLocality{Region: "r1", Zone: zone},
	}
}

func TestClientLbInfoKey_NoPriorityConfig(t *testing.T) {
	inputs := EndpointsInputs{EndpointsForBackend: testEndpoints(wellknown.TrafficDistributionAny)}

	a := ClientLbInfoKey(client("z1", map[string]string{"app": "gw1"}), inputs)
	b := ClientLbInfoKey(client("z2", map[string]string{"app": "gw2"}), inputs)

	if a != "" || b != "" {
		t.Fatalf("expected empty (client-agnostic) keys without priority config, got %q and %q", a, b)
	}
}

func TestClientLbInfoKey_FailoverPriority(t *testing.T) {
	inputs := EndpointsInputs{
		EndpointsForBackend: testEndpoints(wellknown.TrafficDistributionPreferSameZone),
	}
	// clientA and clientB are distinct gateways in the same zone; clientC is in
	// another zone. Only the priority label values should influence the key.
	clientA := client("z1", map[string]string{
		corev1.LabelZoneRegion:   "r1",
		corev1.LabelTopologyZone: "z1",
		"app":                    "gw1",
	})
	clientB := client("z1", map[string]string{
		corev1.LabelZoneRegion:   "r1",
		corev1.LabelTopologyZone: "z1",
		"app":                    "gw2",
	})
	clientC := client("z2", map[string]string{
		corev1.LabelZoneRegion:   "r1",
		corev1.LabelTopologyZone: "z2",
	})

	a := ClientLbInfoKey(clientA, inputs)
	b := ClientLbInfoKey(clientB, inputs)
	c := ClientLbInfoKey(clientC, inputs)

	if a == "" {
		t.Fatal("expected non-empty key with traffic distribution set")
	}
	if a != b {
		t.Fatalf("expected equal keys for clients with equal priority label values, got %q and %q", a, b)
	}
	if a == c {
		t.Fatalf("expected different keys for clients in different zones, got %q for both", a)
	}

	// clients with equal keys must produce identical CLAs
	if !proto.Equal(PrioritizeEndpoints(nil, clientA, inputs), PrioritizeEndpoints(nil, clientB, inputs)) {
		t.Fatal("expected identical CLAs for clients with equal lb info keys")
	}
}

func TestClientLbInfoKey_LocalityFailover(t *testing.T) {
	inputs := EndpointsInputs{
		EndpointsForBackend: testEndpoints(wellknown.TrafficDistributionAny),
		PriorityInfo:        &PriorityInfo{}, // locality failover mode: no FailoverPriority
	}

	a := ClientLbInfoKey(client("z1", map[string]string{"app": "gw1"}), inputs)
	b := ClientLbInfoKey(client("z1", map[string]string{"app": "gw2"}), inputs)
	c := ClientLbInfoKey(client("z2", nil), inputs)

	if a != b {
		t.Fatalf("expected equal keys for clients with equal locality, got %q and %q", a, b)
	}
	if a == c {
		t.Fatalf("expected different keys for clients with different localities, got %q for both", a)
	}
}
