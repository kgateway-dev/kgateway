package proxy_syncer

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
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

// udpTestLocality is one LocalityLbEndpoints of a member CLA, as the per-client pipeline builds it.
type udpTestLocality struct {
	locality *envoycorev3.Locality
	priority uint32
	eps      []*envoyendpointv3.LbEndpoint
}

func udpMemberCLA(localities ...udpTestLocality) *envoyendpointv3.ClusterLoadAssignment {
	cla := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "member"}
	for _, l := range localities {
		cla.Endpoints = append(cla.Endpoints, &envoyendpointv3.LocalityLbEndpoints{
			Locality:    l.locality,
			Priority:    l.priority,
			LbEndpoints: l.eps,
		})
	}
	return cla
}

// udpEps builds endpoints in the default locality at priority 0.
func udpEps(addrs ...string) udpTestLocality {
	l := udpTestLocality{}
	for _, addr := range addrs {
		l.eps = append(l.eps, udpTestEndpoint(addr).LbEndpoint)
	}
	return l
}

func weightedUdpEndpoint(addr string, weight uint32) *envoyendpointv3.LbEndpoint {
	ep := udpTestEndpoint(addr).LbEndpoint
	ep.LoadBalancingWeight = wrapperspb.UInt32(weight)
	return ep
}

// udpMemberRows serves per-client member CLAs the way the regular per-client pipeline does.
func udpMemberRows(rows krt.Collection[UccWithEndpoints]) PerClientEnvoyEndpoints {
	return PerClientEnvoyEndpoints{
		endpoints: rows,
		index:     krtpkg.UnnamedIndex(rows, func(u UccWithEndpoints) []string { return []string{u.Client.ResourceName()} }),
	}
}

func udpMemberRow(ucc ir.UniquelyConnectedClient, backend string, hash uint64, cla *envoyendpointv3.ClusterLoadAssignment) UccWithEndpoints {
	return UccWithEndpoints{
		Client:        ucc,
		Endpoints:     sharedproto.Wrap(cla),
		EndpointsHash: hash,
		endpointsName: backend,
		resourceName:  uccEndpointsResourceName(ucc, backend),
	}
}

// endpointWeight looks up the LB weight of the endpoint with the given address in a CLA.
// udpW returns a backendRef weight scaled to endpoint weight units.
func udpW(w uint64) uint32 {
	//nolint:gosec // G115: test weights stay far below the uint32 max
	return uint32(w * udpWeightScale)
}

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
		{weight: 90, cla: udpMemberCLA(udpEps("10.0.0.1", "10.0.0.2"))},
		{weight: 10, cla: udpMemberCLA(udpEps("10.0.1.1"))},
	}

	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	require.Equal(t, "udpagg_test", cla.GetClusterName())
	require.Len(t, cla.GetEndpoints(), 1, "all endpoints share the default locality")

	wA1, ok := endpointWeight(cla, "10.0.0.1")
	require.True(t, ok)
	wA2, _ := endpointWeight(cla, "10.0.0.2")
	wB, _ := endpointWeight(cla, "10.0.1.1")

	// Per-endpoint weights.
	// A = 90/2 = 45 per endpoint, B = 10/1 = 10, in units of udpWeightScale
	assert.Equal(t, udpW(45), wA1)
	assert.Equal(t, udpW(45), wA2)
	assert.Equal(t, udpW(10), wB)

	// Locality weight is the sum of its endpoints' weights.
	assert.Equal(t, udpW(100), cla.GetEndpoints()[0].GetLoadBalancingWeight().GetValue())
}

// A small weight split over many endpoints must not round each endpoint up and skew the shares.
func TestMergeUdpAggregateLoadAssignment_ManyEndpointsKeepShares(t *testing.T) {
	many := make([]string, 1500)
	for i := range many {
		many[i] = fmt.Sprintf("10.1.%d.%d", i/250, i%250)
	}
	members := []udpMemberEndpoints{
		{weight: 1, cla: udpMemberCLA(udpEps(many...))},
		{weight: 1, cla: udpMemberCLA(udpEps("10.0.0.1", "10.0.0.2", "10.0.0.3"))},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)

	var a, b uint64
	for _, ep := range cla.GetEndpoints()[0].GetLbEndpoints() {
		if strings.HasPrefix(ep.GetEndpoint().GetAddress().GetSocketAddress().GetAddress(), "10.1.") {
			a += uint64(ep.GetLoadBalancingWeight().GetValue())
		} else {
			b += uint64(ep.GetLoadBalancingWeight().GetValue())
		}
	}
	assert.InDelta(t, 0.5, float64(a)/float64(a+b), 0.001, "equal backendRef weights keep equal shares")
}

// A member whose row is not built yet adds nothing until it is, rather than dropping its share.
func TestMergeUdpAggregateLoadAssignment_PendingMemberAddsNothing(t *testing.T) {
	members := []udpMemberEndpoints{
		{weight: 80, cla: udpMemberCLA(udpEps("10.0.0.1"))},
		{weight: 20},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)

	require.Len(t, cla.GetEndpoints(), 1)
	w, ok := endpointWeight(cla, "10.0.0.1")
	require.True(t, ok)
	assert.Equal(t, udpW(80), w)

	_, hasBlackhole := endpointWeight(cla, udpBlackholeAddr)
	assert.False(t, hasBlackhole, "a member without a row yet must not be dropped")
}

func TestMergeUdpAggregateLoadAssignment_MultiLocality(t *testing.T) {
	// One member, one endpoint per locality, so each locality's weight is that endpoint's weight.
	l1 := udpEps("10.0.0.1")
	l1.locality = &envoycorev3.Locality{Region: "r1", Zone: "z1"}
	l2 := udpEps("10.0.0.2")
	l2.locality = &envoycorev3.Locality{Region: "r2", Zone: "z2"}
	members := []udpMemberEndpoints{{weight: 10, cla: udpMemberCLA(l1, l2)}}

	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	require.Len(t, cla.GetEndpoints(), 2)
	// 10/2 = 5 per endpoint. Each locality has one endpoint, so the locality weight is 5 too.
	for _, lle := range cla.GetEndpoints() {
		require.Len(t, lle.GetLbEndpoints(), 1)
		assert.Equal(t, udpW(5), lle.GetLbEndpoints()[0].GetLoadBalancingWeight().GetValue())
		assert.Equal(t, udpW(5), lle.GetLoadBalancingWeight().GetValue())
	}
}

// Envoy ignores the weight of a group without a Locality under locality-weighted LB, so the
// blackhole and endpoints without topology labels need one next to zoned endpoints.
func TestMergeUdpAggregateLoadAssignment_EmptyLocalityKeepsWeight(t *testing.T) {
	zoned := udpEps("10.0.0.1")
	zoned.locality = &envoycorev3.Locality{Region: "r1", Zone: "z1"}
	members := []udpMemberEndpoints{
		{weight: 20, cla: udpMemberCLA(zoned)},
		{weight: 30, cla: udpMemberCLA(udpEps("10.0.1.1"))},
	}

	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 50)
	require.Len(t, cla.GetEndpoints(), 2)
	for _, lle := range cla.GetEndpoints() {
		assert.NotNil(t, lle.GetLocality(), "every group needs a Locality to keep its weight")
	}
	unzoned := cla.GetEndpoints()[0]
	require.Empty(t, unzoned.GetLocality().GetRegion(), "the empty locality sorts first")
	assert.Equal(t, udpW(80), unzoned.GetLoadBalancingWeight().GetValue(), "the blackhole shares the empty locality")
}

// Cross-backend weights take precedence over a member's priorities: every endpoint lands at
// priority 0, so a member whose endpoints are all at a lower priority for this client still gets its
// share instead of losing it to the member at priority 0.
func TestMergeUdpAggregateLoadAssignment_FlattensPriorities(t *testing.T) {
	local := udpEps("10.0.0.1")
	remote := udpEps("10.0.0.2")
	remote.priority = 1
	onlyRemote := udpEps("10.0.1.1")
	onlyRemote.priority = 2
	members := []udpMemberEndpoints{
		{weight: 80, cla: udpMemberCLA(local, remote)},
		{weight: 20, cla: udpMemberCLA(onlyRemote)},
	}

	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	for _, lle := range cla.GetEndpoints() {
		assert.Zero(t, lle.GetPriority(), "the aggregate has a single priority")
	}
	wLocal, _ := endpointWeight(cla, "10.0.0.1")
	wRemote, _ := endpointWeight(cla, "10.0.0.2")
	wOnlyRemote, _ := endpointWeight(cla, "10.0.1.1")
	assert.Equal(t, udpW(40), wLocal)
	assert.Equal(t, udpW(40), wRemote)
	assert.Equal(t, udpW(20), wOnlyRemote, "the 80/20 split survives the member's priorities")
}

// A member's share is split over its endpoints in proportion to the weights the per-client
// pipeline gave them.
func TestMergeUdpAggregateLoadAssignment_RespectsEndpointWeights(t *testing.T) {
	members := []udpMemberEndpoints{
		{weight: 100, cla: udpMemberCLA(udpTestLocality{eps: []*envoyendpointv3.LbEndpoint{
			weightedUdpEndpoint("10.0.0.1", 3),
			weightedUdpEndpoint("10.0.0.2", 1),
		}})},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	w1, _ := endpointWeight(cla, "10.0.0.1")
	w2, _ := endpointWeight(cla, "10.0.0.2")
	assert.Equal(t, udpW(75), w1)
	assert.Equal(t, udpW(25), w2)
}

// The merge keeps what the per-client pipeline set on each endpoint, such as its health status.
func TestMergeUdpAggregateLoadAssignment_KeepsEndpointHealth(t *testing.T) {
	unhealthy := udpTestEndpoint("10.0.0.2").LbEndpoint
	unhealthy.HealthStatus = envoycorev3.HealthStatus_UNHEALTHY
	members := []udpMemberEndpoints{
		{weight: 100, cla: udpMemberCLA(udpTestLocality{eps: []*envoyendpointv3.LbEndpoint{
			udpTestEndpoint("10.0.0.1").LbEndpoint, unhealthy,
		}})},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	for _, ep := range cla.GetEndpoints()[0].GetLbEndpoints() {
		if ep.GetEndpoint().GetAddress().GetSocketAddress().GetAddress() == "10.0.0.2" {
			assert.Equal(t, envoycorev3.HealthStatus_UNHEALTHY, ep.GetHealthStatus())
		}
	}
	assert.Equal(t, envoycorev3.HealthStatus_UNHEALTHY, unhealthy.GetHealthStatus(), "the member's CLA is not modified")
	assert.Nil(t, unhealthy.GetLoadBalancingWeight(), "the member's CLA is not modified")
}

func TestMergeUdpAggregateLoadAssignment_DropWeightBlackhole(t *testing.T) {
	// Valid backend weight 20 (1 endpoint) and an invalid backend weight 80 (dropWeight). The invalid
	// share must go to a blackhole endpoint, not to the valid backend.
	members := []udpMemberEndpoints{
		{weight: 20, cla: udpMemberCLA(udpEps("10.0.0.1"))},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 80)

	wValid, ok := endpointWeight(cla, "10.0.0.1")
	require.True(t, ok, "valid endpoint present")
	wDrop, ok := endpointWeight(cla, udpBlackholeAddr)
	require.True(t, ok, "blackhole endpoint present for the invalid backend's weight")

	// valid = 20, blackhole = 80, in units of udpWeightScale
	assert.Equal(t, udpW(20), wValid)
	assert.Equal(t, udpW(80), wDrop)

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

// A Service with no endpoints (a member row with no endpoints) has its weighted share dropped to
// the blackhole per the UDPRoute spec, not redistributed to the healthy member.
func TestMergeUdpAggregateLoadAssignment_EmptyServiceDrops(t *testing.T) {
	members := []udpMemberEndpoints{
		{weight: 20, cla: udpMemberCLA(udpEps("10.0.0.1"))},
		{weight: 80, cla: udpMemberCLA()},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)

	wValid, ok := endpointWeight(cla, "10.0.0.1")
	require.True(t, ok)
	wDrop, ok := endpointWeight(cla, udpBlackholeAddr)
	require.True(t, ok, "the empty Service's weighted share must go to the blackhole")

	// valid = 20, blackhole = 80, in units of udpWeightScale. 20% valid, 80% dropped.
	assert.Equal(t, udpW(20), wValid)
	assert.Equal(t, udpW(80), wDrop)
}

// When the summed endpoint weights exceed the uint32 max, every weight scales down by one divisor
// so both the per-locality endpoint sum and the locality weight stay under the cap and the split
// stays equal.
func TestMergeUdpAggregateLoadAssignment_ScalesDownToUint32(t *testing.T) {
	// Five members, one endpoint each, all weight 1e6. Unscaled per-endpoint weight is
	// 1e6*1e6/1 = 1e12, so the five total 5e12, above the uint32 max (~4.29e9).
	members := make([]udpMemberEndpoints, 5)
	addrs := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5"}
	for i, addr := range addrs {
		members[i] = udpMemberEndpoints{weight: 1_000_000, cla: udpMemberCLA(udpEps(addr))}
	}

	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	require.Len(t, cla.GetEndpoints(), 1, "all endpoints share the default locality")

	// div = ceil(5e12 / 4294967295) = 1165, so each 1e12 becomes 858369098 and the five still
	// split evenly.
	var localityWeight uint64
	for _, addr := range addrs {
		w, ok := endpointWeight(cla, addr)
		require.True(t, ok, "endpoint %s present", addr)
		assert.Equal(t, uint32(858_369_098), w)
		localityWeight += uint64(w)
	}

	lle := cla.GetEndpoints()[0]
	assert.Equal(t, uint32(4_291_845_490), lle.GetLoadBalancingWeight().GetValue())
	require.LessOrEqual(t, localityWeight, uint64(math.MaxUint32), "the locality endpoint sum must fit uint32")
	require.LessOrEqual(t, uint64(lle.GetLoadBalancingWeight().GetValue()), uint64(math.MaxUint32), "the locality weight must fit uint32")
}

func udpTestClient(gw string) ir.UniquelyConnectedClient {
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", gw)
	return ir.NewUniquelyConnectedClient(role, "ns", map[string]string{wellknown.GatewayNameLabel: gw}, ir.PodLocality{})
}

func udpGatewayWithCluster(gw, cluster string) GatewayXdsResources {
	return GatewayXdsResources{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: gw},
		Clusters:       []envoycachetypes.ResourceWithTTL{{Resource: &envoyclusterv3.Cluster{Name: cluster}}},
	}
}

// TestNewPerClientUdpAggregateEndpointsScopesToGatewayWithCluster guards both #14471 directions.
// Only clients of a gateway whose CDS carries the matching cluster receive the aggregate CLA. A
// client of a gateway without the cluster, from an unaccepted route or another gateway, receives
// none, even though the aggregate descriptor exists.
func TestNewPerClientUdpAggregateEndpointsScopesToGatewayWithCluster(t *testing.T) {
	g := gomega.NewWithT(t)

	clusterName := ir.UdpAggregateClusterName("ns", "route")
	clientA, clientB := udpTestClient("gw-a"), udpTestClient("gw-b")
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{clientA, clientB})

	// gw-a's CDS carries the aggregate cluster and gw-b's does not.
	snapshots := krt.NewStaticCollection(nil, []GatewayXdsResources{
		udpGatewayWithCluster("gw-a", clusterName),
		udpGatewayWithCluster("gw-b", "kube_ns_backend_80"),
	})
	aggregates := krt.NewStaticCollection(nil, []udpAggregate{
		{clusterName: clusterName, members: []udpAggregateMember{{backendResourceName: "be1", weight: 100}}},
	})
	rows := krt.NewStaticCollection(nil, []UccWithEndpoints{
		udpMemberRow(clientA, "be1", 1, udpMemberCLA(udpEps("10.0.0.1"))),
		udpMemberRow(clientB, "be1", 1, udpMemberCLA(udpEps("10.0.0.1"))),
	})

	eps := NewPerClientUdpAggregateEndpoints(krtutil.KrtOptions{}, uccs, aggregates, snapshots, udpMemberRows(rows))

	var got []UccWithEndpoints
	g.Eventually(func() []UccWithEndpoints {
		got = eps.endpoints.List()
		return got
	}).Should(gomega.HaveLen(1), "only the gateway whose CDS carries the aggregate cluster gets the CLA")

	g.Expect(got[0].Client.ResourceName()).To(gomega.Equal(clientA.ResourceName()))
	g.Expect(got[0].Endpoints.Clone().GetClusterName()).To(gomega.Equal(clusterName))
}

// Each client's aggregate uses that client's CLA for each member, so what the per-client pipeline
// decides for one client does not leak into another client's aggregate.
func TestNewPerClientUdpAggregateEndpointsUsesEachClientsMemberCLA(t *testing.T) {
	g := gomega.NewWithT(t)

	clusterName := ir.UdpAggregateClusterName("ns", "route")
	clientA, clientB := udpTestClient("gw-a"), udpTestClient("gw-b")
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{clientA, clientB})
	snapshots := krt.NewStaticCollection(nil, []GatewayXdsResources{
		udpGatewayWithCluster("gw-a", clusterName),
		udpGatewayWithCluster("gw-b", clusterName),
	})
	aggregates := krt.NewStaticCollection(nil, []udpAggregate{
		{clusterName: clusterName, members: []udpAggregateMember{{backendResourceName: "be1", weight: 100}}},
	})
	rows := krt.NewStaticCollection(nil, []UccWithEndpoints{
		udpMemberRow(clientA, "be1", 1, udpMemberCLA(udpEps("10.0.0.1"))),
		udpMemberRow(clientB, "be1", 2, udpMemberCLA(udpEps("10.0.0.2"))),
	})

	eps := NewPerClientUdpAggregateEndpoints(krtutil.KrtOptions{}, uccs, aggregates, snapshots, udpMemberRows(rows))
	g.Eventually(func() int { return len(eps.endpoints.List()) }).Should(gomega.Equal(2))

	byClient := map[string]*envoyendpointv3.ClusterLoadAssignment{}
	for _, row := range eps.endpoints.List() {
		byClient[row.Client.ResourceName()] = row.Endpoints.Clone()
	}
	_, ok := endpointWeight(byClient[clientA.ResourceName()], "10.0.0.1")
	g.Expect(ok).To(gomega.BeTrue(), "client A's aggregate uses client A's member CLA")
	_, ok = endpointWeight(byClient[clientB.ResourceName()], "10.0.0.2")
	g.Expect(ok).To(gomega.BeTrue(), "client B's aggregate uses client B's member CLA")
	_, ok = endpointWeight(byClient[clientB.ResourceName()], "10.0.0.1")
	g.Expect(ok).To(gomega.BeFalse(), "client A's endpoints do not leak into client B's aggregate")
}

// Removing and restoring a member's endpoints under a weighted UDPRoute keeps the weights and the
// invalid backend's drop share correct at every step, and returns to the original CLA.
func TestNewPerClientUdpAggregateEndpointsEndpointRemovalAndRestore(t *testing.T) {
	g := gomega.NewWithT(t)

	clusterName := ir.UdpAggregateClusterName("ns", "route")
	client := udpTestClient("gw")
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{client})
	snapshots := krt.NewStaticCollection(nil, []GatewayXdsResources{udpGatewayWithCluster("gw", clusterName)})
	// v1 weight 60 and v2 weight 30 are valid. A third backendRef of weight 10 is invalid.
	aggregates := krt.NewStaticCollection(nil, []udpAggregate{{
		clusterName: clusterName,
		members: []udpAggregateMember{
			{backendResourceName: "v1", weight: 60},
			{backendResourceName: "v2", weight: 30},
		},
		dropWeight: 10,
	}})
	full := []UccWithEndpoints{
		udpMemberRow(client, "v1", 1, udpMemberCLA(udpEps("10.0.0.1", "10.0.0.2"))),
		udpMemberRow(client, "v2", 1, udpMemberCLA(udpEps("10.0.1.1"))),
	}
	rows := krt.NewMutableCollection(nil, full)
	eps := NewPerClientUdpAggregateEndpoints(krtutil.KrtOptions{}, uccs, aggregates, snapshots, udpMemberRows(rows.AsCollection()))

	type step struct {
		name    string
		weights map[string]uint32
	}
	current := func() (map[string]uint32, uint64) {
		list := eps.endpoints.List()
		if len(list) != 1 {
			return nil, 0
		}
		got := map[string]uint32{}
		for _, lle := range list[0].Endpoints.Clone().GetEndpoints() {
			for _, ep := range lle.GetLbEndpoints() {
				got[ep.GetEndpoint().GetAddress().GetSocketAddress().GetAddress()] = ep.GetLoadBalancingWeight().GetValue()
			}
		}
		return got, list[0].EndpointsHash
	}
	hashes := map[uint64]string{}
	expect := func(s step) {
		g.Eventually(func() map[string]uint32 {
			got, _ := current()
			return got
		}).Should(gomega.Equal(s.weights), s.name)
		_, hash := current()
		if prev, seen := hashes[hash]; seen && prev != "full" {
			t.Fatalf("%s reuses the hash of %s", s.name, prev)
		}
		hashes[hash] = s.name
	}

	expect(step{name: "full", weights: map[string]uint32{
		"10.0.0.1": udpW(30), "10.0.0.2": udpW(30), "10.0.1.1": udpW(30), udpBlackholeAddr: udpW(10),
	}})

	// v1 loses an endpoint: its whole 60 moves to the endpoint it still has.
	rows.UpdateObject(udpMemberRow(client, "v1", 2, udpMemberCLA(udpEps("10.0.0.1"))))
	expect(step{name: "v1 scaled down", weights: map[string]uint32{
		"10.0.0.1": udpW(60), "10.0.1.1": udpW(30), udpBlackholeAddr: udpW(10),
	}})

	// v2 loses every endpoint: an empty Service, so its 30 joins the drop share.
	rows.UpdateObject(udpMemberRow(client, "v2", 2, udpMemberCLA()))
	expect(step{name: "v2 empty", weights: map[string]uint32{
		"10.0.0.1": udpW(60), udpBlackholeAddr: udpW(40),
	}})

	// Both return to the original weights and hash.
	for _, row := range full {
		rows.UpdateObject(row)
	}
	expect(step{name: "full", weights: map[string]uint32{
		"10.0.0.1": udpW(30), "10.0.0.2": udpW(30), "10.0.1.1": udpW(30), udpBlackholeAddr: udpW(10),
	}})
}

func TestUdpAggregateForRoute(t *testing.T) {
	be := func(name string) *ir.BackendObjectIR {
		obj := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: name}, 53, "", "kube")
		return &obj
	}
	route := func(backends ...ir.BackendRefIR) ir.UdpRouteIR {
		return ir.UdpRouteIR{ObjectSource: ir.ObjectSource{Namespace: "ns", Name: "route"}, Backends: backends}
	}
	clusterName := ir.UdpAggregateClusterName("ns", "route")

	assert.Nil(t, udpAggregateForRoute(route(ir.BackendRefIR{Weight: 1, BackendObject: be("a")})),
		"a single weighted backend targets its own cluster")

	assert.Equal(t, &udpAggregate{clusterName: clusterName, members: []udpAggregateMember{}},
		udpAggregateForRoute(route(ir.BackendRefIR{Weight: 0, BackendObject: be("a")})),
		"a sole zero-weight backend gets an aggregate with nothing to forward to")

	got := udpAggregateForRoute(route(
		ir.BackendRefIR{Weight: 0, BackendObject: be("zero")},
		ir.BackendRefIR{Weight: 70, BackendObject: be("a")},
		ir.BackendRefIR{Weight: 20},
		ir.BackendRefIR{Weight: 0},
		ir.BackendRefIR{Weight: 5},
		ir.BackendRefIR{Weight: 10, BackendObject: be("b")},
	))
	assert.Equal(t, &udpAggregate{
		clusterName: clusterName,
		members: []udpAggregateMember{
			{backendResourceName: be("a").ResourceName(), weight: 70},
			{backendResourceName: be("b").ResourceName(), weight: 10},
		},
		dropWeight: 25,
	}, got, "zero weights are skipped and invalid backends add their weight to the drop")
}

func TestUdpAggregateEquals(t *testing.T) {
	base := func() udpAggregate {
		return udpAggregate{
			clusterName: "udpagg_ns_route",
			members:     []udpAggregateMember{{backendResourceName: "a", weight: 1}},
			dropWeight:  2,
		}
	}
	assert.True(t, base().Equals(base()))
	for name, mutate := range map[string]func(*udpAggregate){
		"cluster name":  func(u *udpAggregate) { u.clusterName = "udpagg_ns_other" },
		"drop weight":   func(u *udpAggregate) { u.dropWeight = 3 },
		"member weight": func(u *udpAggregate) { u.members[0].weight = 2 },
		"member name":   func(u *udpAggregate) { u.members[0].backendResourceName = "b" },
		"member count":  func(u *udpAggregate) { u.members = append(u.members, udpAggregateMember{backendResourceName: "b"}) },
	} {
		other := base()
		mutate(&other)
		assert.False(t, base().Equals(other), name)
		assert.False(t, other.Equals(base()), name)
	}
}

func TestAcceptedUdpAggregates(t *testing.T) {
	aggName := ir.UdpAggregateClusterName("ns", "route")
	snap := GatewayXdsResources{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Clusters: []envoycachetypes.ResourceWithTTL{
			{Resource: &envoyclusterv3.Cluster{Name: "kube_ns_backend_53"}},
			{Resource: &envoyclusterv3.Cluster{Name: aggName}},
			{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: ir.UdpAggregateClusterName("ns", "cla")}},
		},
	}
	got := acceptedUdpAggregates(snap)
	require.Equal(t, []acceptedUdpAggregate{{role: snap.ResourceName(), clusterName: aggName}}, got,
		"only aggregate clusters count, other clusters and non-cluster resources do not")

	a := got[0]
	assert.Equal(t, snap.ResourceName()+"/"+aggName, a.ResourceName())
	assert.True(t, a.Equals(acceptedUdpAggregate{role: a.role, clusterName: a.clusterName}))
	for _, other := range []acceptedUdpAggregate{
		{role: "other", clusterName: a.clusterName},
		{role: a.role, clusterName: "udpagg_ns_other"},
	} {
		assert.False(t, a.Equals(other))
		assert.False(t, other.Equals(a))
	}
}

// The blackhole is the loopback discard port, so dropped datagrams go nowhere.
func TestBlackholeLbEndpoint(t *testing.T) {
	sa := blackholeLbEndpoint().GetEndpoint().GetAddress().GetSocketAddress()
	assert.Equal(t, "127.0.0.1", sa.GetAddress())
	assert.Equal(t, uint32(9), sa.GetPortValue())
}

// Members that add nothing, pending or empty, must not stop the merge of the members after them.
func TestMergeUdpAggregateLoadAssignment_SkipsMembersThatAddNothing(t *testing.T) {
	members := []udpMemberEndpoints{
		{weight: 10},
		{weight: 30, cla: udpMemberCLA(udpTestLocality{})},
		{weight: 60, cla: udpMemberCLA(udpEps("10.0.0.1"))},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 1)

	w, ok := endpointWeight(cla, "10.0.0.1")
	require.True(t, ok, "the member after a pending and an empty one is merged")
	assert.Equal(t, udpW(60), w)
	bh, ok := endpointWeight(cla, udpBlackholeAddr)
	require.True(t, ok, "a drop weight of 1 still adds the blackhole")
	assert.Equal(t, udpW(31), bh, "the empty Service's weight joins the drop weight")
}

// An endpoint whose share rounds down to 0 keeps weight 1, since Envoy rejects weight 0. Unset
// weights count as 1 when splitting the member's share.
func TestMergeUdpAggregateLoadAssignment_TinyShareKeepsWeightOne(t *testing.T) {
	members := []udpMemberEndpoints{{weight: 1, cla: udpMemberCLA(udpTestLocality{eps: []*envoyendpointv3.LbEndpoint{
		weightedUdpEndpoint("10.0.0.1", math.MaxUint32),
		udpTestEndpoint("10.0.0.2").LbEndpoint,
	}})}}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)

	heavy, _ := endpointWeight(cla, "10.0.0.1")
	light, ok := endpointWeight(cla, "10.0.0.2")
	require.True(t, ok)
	assert.Equal(t, uint32(1), light, "1e6*1/(2^32) rounds to 0 and keeps weight 1")
	assert.Equal(t, uint32(999_999), heavy, "the unset weight counts as 1 in the member total")
}

// Scaling down keeps every endpoint at weight 1 or more, even one whose scaled share is 0.
func TestMergeUdpAggregateLoadAssignment_ScaleDownKeepsWeightOne(t *testing.T) {
	members := []udpMemberEndpoints{
		{weight: 1_000_000, cla: udpMemberCLA(udpEps("10.0.0.1"))},
		{weight: 1_000_000, cla: udpMemberCLA(udpEps("10.0.0.2"))},
		{weight: 1_000_000, cla: udpMemberCLA(udpEps("10.0.0.3"))},
		{weight: 1, cla: udpMemberCLA(udpTestLocality{eps: []*envoyendpointv3.LbEndpoint{
			weightedUdpEndpoint("10.0.1.1", math.MaxUint32),
			udpTestEndpoint("10.0.1.2").LbEndpoint,
		}})},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)

	// The total of 3e12 + 1e6 scales by ceil(total / (2^32 - 1 - 5)) = 699.
	big, _ := endpointWeight(cla, "10.0.0.1")
	assert.Equal(t, uint32(1_000_000_000_000/699), big)
	mid, _ := endpointWeight(cla, "10.0.1.1")
	assert.Equal(t, uint32(999_999/699), mid)
	tiny, ok := endpointWeight(cla, "10.0.1.2")
	require.True(t, ok)
	assert.Equal(t, uint32(1), tiny, "a share that scales to 0 keeps weight 1")
}

// The aggregate row hash changes with every input of its CLA, so KRT republishes the CLA.
func TestHashUdpAggregate(t *testing.T) {
	agg := udpAggregate{clusterName: "udpagg_ns_route", dropWeight: 1}
	members := func() []udpMemberEndpoints {
		return []udpMemberEndpoints{
			{weight: 60, cla: udpMemberCLA(udpEps("10.0.0.1")), hash: 7},
			{weight: 40},
		}
	}
	base := hashUdpAggregate(agg, members())
	assert.Equal(t, base, hashUdpAggregate(agg, members()), "the hash is deterministic")

	for name, change := range map[string]func(*udpAggregate, []udpMemberEndpoints){
		"cluster name":   func(a *udpAggregate, _ []udpMemberEndpoints) { a.clusterName = "udpagg_ns_other" },
		"drop weight":    func(a *udpAggregate, _ []udpMemberEndpoints) { a.dropWeight = 2 },
		"member weight":  func(_ *udpAggregate, m []udpMemberEndpoints) { m[0].weight = 61 },
		"member hash":    func(_ *udpAggregate, m []udpMemberEndpoints) { m[0].hash = 8 },
		"member pending": func(_ *udpAggregate, m []udpMemberEndpoints) { m[0].cla = nil },
		"member arrives": func(_ *udpAggregate, m []udpMemberEndpoints) {
			m[1].cla = udpMemberCLA(udpEps("10.0.1.1"))
		},
	} {
		a, m := agg, members()
		change(&a, m)
		assert.NotEqual(t, base, hashUdpAggregate(a, m), name)
	}

	// A pending member does not hide the members after it.
	pendingFirst := []udpMemberEndpoints{{weight: 40}, {weight: 60, cla: udpMemberCLA(udpEps("10.0.0.1")), hash: 7}}
	changed := slices.Clone(pendingFirst)
	changed[1].hash = 8
	assert.NotEqual(t, hashUdpAggregate(agg, pendingFirst), hashUdpAggregate(agg, changed))
}

func TestMergeUdpAggregateLoadAssignment_DropWeightOne(t *testing.T) {
	members := []udpMemberEndpoints{{weight: 99, cla: udpMemberCLA(udpEps("10.0.0.1"))}}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 1)
	bh, ok := endpointWeight(cla, udpBlackholeAddr)
	require.True(t, ok, "a drop weight of 1 adds the blackhole")
	assert.Equal(t, udpW(1), bh)
}

// An endpoint without a weight counts as 1, as Envoy treats it.
func TestMergeUdpAggregateLoadAssignment_UnsetEndpointWeightCountsAsOne(t *testing.T) {
	members := []udpMemberEndpoints{{weight: 3, cla: udpMemberCLA(udpTestLocality{eps: []*envoyendpointv3.LbEndpoint{
		weightedUdpEndpoint("10.0.0.1", 2),
		udpTestEndpoint("10.0.0.2").LbEndpoint,
	}})}}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	weighted, _ := endpointWeight(cla, "10.0.0.1")
	unset, _ := endpointWeight(cla, "10.0.0.2")
	assert.Equal(t, udpW(2), weighted)
	assert.Equal(t, udpW(1), unset)
}

// Endpoints come out in address order whatever order the members list them in, so an unchanged
// set of endpoints yields the same CLA.
func TestMergeUdpAggregateLoadAssignment_SortsEndpoints(t *testing.T) {
	members := []udpMemberEndpoints{
		{weight: 1, cla: udpMemberCLA(udpEps("10.0.0.3", "10.0.0.1"))},
		{weight: 1, cla: udpMemberCLA(udpEps("10.0.0.2"))},
	}
	cla := mergeUdpAggregateLoadAssignment("udpagg_test", members, 0)
	require.Len(t, cla.GetEndpoints(), 1)
	var got []string
	for _, ep := range cla.GetEndpoints()[0].GetLbEndpoints() {
		got = append(got, ep.GetEndpoint().GetAddress().GetSocketAddress().GetAddress())
	}
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}, got)
}

// A client gets no row for an aggregate cluster whose descriptor is gone, and a member whose row
// has not landed adds nothing until it does.
func TestNewPerClientUdpAggregateEndpointsMissingDescriptorAndMemberRow(t *testing.T) {
	g := gomega.NewWithT(t)

	withDescriptor := ir.UdpAggregateClusterName("ns", "route")
	client := udpTestClient("gw")
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{client})
	snapshots := krt.NewStaticCollection(nil, []GatewayXdsResources{{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Clusters: []envoycachetypes.ResourceWithTTL{
			{Resource: &envoyclusterv3.Cluster{Name: ir.UdpAggregateClusterName("ns", "gone")}},
			{Resource: &envoyclusterv3.Cluster{Name: withDescriptor}},
		},
	}})
	aggregates := krt.NewStaticCollection(nil, []udpAggregate{{
		clusterName: withDescriptor,
		members: []udpAggregateMember{
			{backendResourceName: "no-row", weight: 50},
			{backendResourceName: "be1", weight: 50},
		},
	}})
	rows := krt.NewStaticCollection(nil, []UccWithEndpoints{
		udpMemberRow(client, "be1", 1, udpMemberCLA(udpEps("10.0.0.1"))),
	})

	eps := NewPerClientUdpAggregateEndpoints(krtutil.KrtOptions{}, uccs, aggregates, snapshots, udpMemberRows(rows))
	g.Eventually(func() int { return len(eps.endpoints.List()) }).Should(gomega.Equal(1),
		"only the cluster with a descriptor gets a row")

	cla := eps.endpoints.List()[0].Endpoints.Clone()
	g.Expect(cla.GetClusterName()).To(gomega.Equal(withDescriptor))
	w, ok := endpointWeight(cla, "10.0.0.1")
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(w).To(gomega.Equal(udpW(50)))
	_, hasBlackhole := endpointWeight(cla, udpBlackholeAddr)
	g.Expect(hasBlackhole).To(gomega.BeFalse(), "a member without a row is pending, not dropped")
}
