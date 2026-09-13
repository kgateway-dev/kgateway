package proxy_syncer

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func udpTestEndpoint(addr string) ir.EndpointWithMd {
	return ir.EndpointWithMd{
		LbEndpoint: &envoyendpointv3.LbEndpoint{
			HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
				Endpoint: &envoyendpointv3.Endpoint{
					Address: &envoycorev3.Address{
						Address: &envoycorev3.Address_SocketAddress{
							SocketAddress: &envoycorev3.SocketAddress{
								Address:       addr,
								PortSpecifier: &envoycorev3.SocketAddress_PortValue{PortValue: 8080},
							},
						},
					},
				},
			},
		},
	}
}

func efbInDefaultLocality(eps ...ir.EndpointWithMd) ir.EndpointsForBackend {
	return ir.EndpointsForBackend{LbEps: ir.LocalityLbMap{ir.PodLocality{}: eps}}
}

// endpointWeight looks up the LB weight of the endpoint with the given address in a CLA.
func endpointWeight(cla *envoyendpointv3.ClusterLoadAssignment, addr string) (uint32, bool) {
	for _, lle := range cla.GetEndpoints() {
		for _, ep := range lle.GetLbEndpoints() {
			if ep.GetEndpoint().GetAddress().GetSocketAddress().GetAddress() == addr {
				return ep.GetLoadBalancingWeight().GetValue(), true
			}
		}
	}
	return 0, false
}

func TestMergeUdpAggregateLoadAssignment_WeightsRespectReplicaCount(t *testing.T) {
	// Member A has weight 90 across 2 endpoints, member B weight 10 across 1 endpoint.
	// Each member's TOTAL weight must stay proportional to its backendRef weight (90:10),
	// independent of the member's endpoint count.
	members := []udpMemberEndpoints{
		{weight: 90, efbs: []ir.EndpointsForBackend{efbInDefaultLocality(
			udpTestEndpoint("10.0.0.1"), udpTestEndpoint("10.0.0.2"),
		)}},
		{weight: 10, efbs: []ir.EndpointsForBackend{efbInDefaultLocality(
			udpTestEndpoint("10.0.1.1"),
		)}},
	}

	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	require.Equal(t, "udpagg_test", cla.GetClusterName())
	require.Len(t, cla.GetEndpoints(), 1, "all endpoints share the default locality")

	wA1, ok := endpointWeight(cla, "10.0.0.1")
	require.True(t, ok)
	wA2, _ := endpointWeight(cla, "10.0.0.2")
	wB, _ := endpointWeight(cla, "10.0.1.1")

	// Per-endpoint weights.
	// A = 90*1000/2 = 45000
	// B = 10*1000/1 = 10000
	assert.Equal(t, uint32(45000), wA1)
	assert.Equal(t, uint32(45000), wA2)
	assert.Equal(t, uint32(10000), wB)

	// Totals per member: A = 90000, B = 10000 -> 9:1, matching the 90:10 backendRef weights.
	assert.Equal(t, uint32(90000), wA1+wA2)
	assert.Equal(t, uint32(10000), wB)

	// Locality weight is the sum of its endpoints' weights.
	assert.Equal(t, uint32(100000), cla.GetEndpoints()[0].GetLoadBalancingWeight().GetValue())
}

func TestMergeUdpAggregateLoadAssignment_SkipsEmptyAndZeroWeight(t *testing.T) {
	members := []udpMemberEndpoints{
		{weight: 100, efbs: []ir.EndpointsForBackend{efbInDefaultLocality(udpTestEndpoint("10.0.0.1"))}},
		{weight: 50, efbs: nil}, // invalid/no endpoints: contributes nothing
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	require.Len(t, cla.GetEndpoints(), 1)
	require.Len(t, cla.GetEndpoints()[0].GetLbEndpoints(), 1)
	w, ok := endpointWeight(cla, "10.0.0.1")
	require.True(t, ok)
	assert.Equal(t, uint32(100000), w) // 100*1000/1
}

func TestMergeUdpAggregateLoadAssignment_MultiLocality(t *testing.T) {
	// One member, one endpoint per locality, so each locality's weight is that endpoint's weight.
	members := []udpMemberEndpoints{
		{weight: 10, efbs: []ir.EndpointsForBackend{{
			LbEps: ir.LocalityLbMap{
				ir.PodLocality{Region: "r1", Zone: "z1"}: {udpTestEndpoint("10.0.0.1")},
				ir.PodLocality{Region: "r2", Zone: "z2"}: {udpTestEndpoint("10.0.0.2")},
			},
		}}},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	require.Len(t, cla.GetEndpoints(), 2)
	// 10*1000/2 = 5000 per endpoint. Each locality has one endpoint, so locality weight = 5000.
	for _, lle := range cla.GetEndpoints() {
		require.Len(t, lle.GetLbEndpoints(), 1)
		assert.Equal(t, uint32(5000), lle.GetLbEndpoints()[0].GetLoadBalancingWeight().GetValue())
		assert.Equal(t, uint32(5000), lle.GetLoadBalancingWeight().GetValue())
	}
}

func TestMergeUdpAggregateLoadAssignment_DropWeightBlackhole(t *testing.T) {
	// Valid backend weight 20 (1 endpoint) and an invalid backend weight 80 (dropWeight). The invalid
	// share must go to a blackhole endpoint, not be redistributed.
	members := []udpMemberEndpoints{
		{weight: 20, efbs: []ir.EndpointsForBackend{efbInDefaultLocality(udpTestEndpoint("10.0.0.1"))}},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 80)

	wValid, ok := endpointWeight(cla, "10.0.0.1")
	require.True(t, ok, "valid endpoint present")
	wDrop, ok := endpointWeight(cla, udpBlackholeAddr)
	require.True(t, ok, "blackhole endpoint present for the invalid backend's weight")

	// valid = 20*1000 = 20000
	// blackhole = 80*1000 = 80000
	assert.Equal(t, uint32(20000), wValid)
	assert.Equal(t, uint32(80000), wDrop)

	// The blackhole targets the loopback discard port.
	var found bool
	for _, lle := range cla.GetEndpoints() {
		for _, ep := range lle.GetLbEndpoints() {
			sa := ep.GetEndpoint().GetAddress().GetSocketAddress()
			if sa.GetAddress() == udpBlackholeAddr {
				assert.Equal(t, uint32(udpBlackholePort), sa.GetPortValue())
				found = true
			}
		}
	}
	assert.True(t, found)
}
