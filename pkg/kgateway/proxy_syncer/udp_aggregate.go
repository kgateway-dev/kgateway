package proxy_syncer

import (
	"cmp"
	"hash/fnv"
	"math"
	"slices"
	"strconv"
	"strings"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// udpWeightScale scales backendRef weights into per-endpoint load-balancing weights. Envoy only
// needs proportionality. The scale is large so that splitting a small weight over many endpoints
// does not round each endpoint up to 1 and skew the shares between backends.
const udpWeightScale uint64 = 1_000_000

// Invalid weight goes to a blackhole endpoint on the loopback discard port.
// The Gateway API requires that weight to drop rather than redistribute
// to the healthy backends.
const (
	udpBlackholeAddr = "127.0.0.1"
	udpBlackholePort = 9
)

type udpAggregateMember struct {
	// backendResourceName matches ir.EndpointsForBackend.UpstreamResourceName.
	backendResourceName string
	weight              uint32
}

// udpAggregate describes the synthetic cluster a multi-backend UDPRoute routes to. The struct holds
// the weighted member backends whose endpoints the cluster unions, plus the combined weight of any
// invalid backends.
type udpAggregate struct {
	clusterName string
	members     []udpAggregateMember
	dropWeight  uint32
}

func (u udpAggregate) ResourceName() string { return u.clusterName }

func (u udpAggregate) Equals(o udpAggregate) bool {
	return u.clusterName == o.clusterName &&
		u.dropWeight == o.dropWeight &&
		slices.Equal(u.members, o.members)
}

// newUdpAggregateCollection derives one aggregate descriptor per multi-backend UDPRoute. Invalid
// backends contribute no endpoints, only their weight into dropWeight.
func newUdpAggregateCollection(
	krtopts krtutil.KrtOptions,
	udpRoutes krt.Collection[ir.UdpRouteIR],
) krt.Collection[udpAggregate] {
	return krt.NewCollection(udpRoutes, func(_ krt.HandlerContext, r ir.UdpRouteIR) *udpAggregate {
		// A multi-backend route uses a synthetic aggregate cluster. A single-backend route targets
		// its backend directly. The descriptor only supplies the members.
		// newAcceptedUdpAggregateCollection decides which clients receive the CLA, so a rejected
		// route's descriptor goes unused.
		if len(r.Backends) <= 1 {
			return nil
		}
		members := make([]udpAggregateMember, 0, len(r.Backends))
		var dropWeight uint32
		for _, b := range r.Backends {
			if b.Weight == 0 {
				continue
			}
			if b.BackendObject == nil {
				dropWeight += b.Weight
				continue
			}
			members = append(members, udpAggregateMember{
				backendResourceName: b.BackendObject.ResourceName(),
				weight:              b.Weight,
			})
		}
		return &udpAggregate{
			clusterName: ir.UdpAggregateClusterName(r.Namespace, r.Name),
			members:     members,
			dropWeight:  dropWeight,
		}
	}, krtopts.ToOptions("UdpAggregates")...)
}

// acceptedUdpAggregate names one aggregate cluster in a gateway's CDS, keyed by that gateway's
// snapshot role. kgateway emits the CLA only where the cluster is, so CDS and EDS stay in lockstep.
// A rejected route or a gateway that does not carry the route gets neither, which avoids the
// withhold in issue #14471.
type acceptedUdpAggregate struct {
	role        string
	clusterName string
}

func (a acceptedUdpAggregate) ResourceName() string { return a.role + "/" + a.clusterName }

func (a acceptedUdpAggregate) Equals(o acceptedUdpAggregate) bool {
	return a.role == o.role && a.clusterName == o.clusterName
}

// newAcceptedUdpAggregateCollection reads the aggregate clusters each gateway emitted in its CDS,
// so that only those gateways' clients receive the CLA.
func newAcceptedUdpAggregateCollection(
	krtopts krtutil.KrtOptions,
	mostXdsSnapshots krt.Collection[GatewayXdsResources],
) krt.Collection[acceptedUdpAggregate] {
	return krt.NewManyCollection(mostXdsSnapshots, func(_ krt.HandlerContext, snap GatewayXdsResources) []acceptedUdpAggregate {
		var out []acceptedUdpAggregate
		for _, c := range snap.Clusters {
			cluster, ok := c.Resource.(*envoyclusterv3.Cluster)
			if !ok || !strings.HasPrefix(cluster.GetName(), ir.UdpAggregateClusterPrefix) {
				continue
			}
			out = append(out, acceptedUdpAggregate{role: snap.ResourceName(), clusterName: cluster.GetName()})
		}
		return out
	}, krtopts.ToOptions("AcceptedUdpAggregates")...)
}

// NewPerClientUdpAggregateEndpoints builds the per-client CLA for each UDP aggregate cluster. A
// client receives one only when its gateway's CDS carries the matching cluster. Each member's
// endpoints come from that client's CLA for the member, built by the regular per-client endpoint
// pipeline, so endpoint plugins, policies and per-client endpoint selection apply as they do for a
// single backend.
func NewPerClientUdpAggregateEndpoints(
	krtopts krtutil.KrtOptions,
	uccs krt.Collection[ir.UniquelyConnectedClient],
	aggregates krt.Collection[udpAggregate],
	mostXdsSnapshots krt.Collection[GatewayXdsResources],
	memberEndpoints PerClientEnvoyEndpoints,
) PerClientEnvoyEndpoints {
	accepted := newAcceptedUdpAggregateCollection(krtopts, mostXdsSnapshots)
	acceptedByRole := krtpkg.UnnamedIndex(accepted, func(a acceptedUdpAggregate) []string {
		return []string{a.role}
	})

	endpoints := krt.NewManyCollection(uccs, func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) []UccWithEndpoints {
		clusters := krt.Fetch(kctx, accepted, krt.FilterIndex(acceptedByRole, ucc.Role))
		ret := make([]UccWithEndpoints, 0, len(clusters))
		for _, c := range clusters {
			agg := krt.FetchOne(kctx, aggregates, krt.FilterKey(c.clusterName))
			if agg == nil {
				continue
			}
			members := fetchUdpMemberEndpoints(kctx, ucc, *agg, memberEndpoints)
			cla := mergeUdpAggregateLoadAssignment(agg.clusterName, members, agg.dropWeight)
			ret = append(ret, UccWithEndpoints{
				Client:        ucc,
				Endpoints:     sharedproto.Wrap(cla),
				EndpointsHash: hashUdpAggregate(*agg, members),
				endpointsName: c.clusterName,
				resourceName:  uccEndpointsResourceName(ucc, c.clusterName),
			})
		}
		return ret
	}, krtopts.ToOptions("UdpAggregateEndpoints")...)

	idx := krtpkg.UnnamedIndex(endpoints, func(u UccWithEndpoints) []string {
		return []string{u.Client.ResourceName()}
	})

	return PerClientEnvoyEndpoints{
		endpoints: endpoints,
		index:     idx,
		// A route's descriptor derives its aggregate CLA. A client whose CDS has
		// the cluster before the row lands waits for the row instead of receiving
		// an empty placeholder over the endpoints it holds.
		hasSource: func(kctx krt.HandlerContext, claName string) bool {
			return krt.FetchOne(kctx, aggregates, krt.FilterKey(claName)) != nil
		},
	}
}

// udpMemberEndpoints pairs a member's backendRef weight with the client's CLA for that member.
// cla is nil while the member's row is not built yet. hash is the row's EndpointsHash.
type udpMemberEndpoints struct {
	weight uint32
	cla    *envoyendpointv3.ClusterLoadAssignment
	hash   uint64
}

func fetchUdpMemberEndpoints(
	kctx krt.HandlerContext,
	ucc ir.UniquelyConnectedClient,
	agg udpAggregate,
	memberEndpoints PerClientEnvoyEndpoints,
) []udpMemberEndpoints {
	members := make([]udpMemberEndpoints, 0, len(agg.members))
	for _, m := range agg.members {
		member := udpMemberEndpoints{weight: m.weight}
		row := krt.FetchOne(kctx, memberEndpoints.endpoints, krt.FilterKey(uccEndpointsResourceName(ucc, m.backendResourceName)))
		if row != nil && !row.Endpoints.IsNil() {
			member.cla = row.Endpoints.BorrowForRead()
			member.hash = row.EndpointsHash
		}
		members = append(members, member)
	}
	return members
}

// mergeUdpAggregateLoadAssignment merges the members' CLAs into one weighted CLA at a single
// priority. Cross-backend weights take precedence over the members' priorities, so locality
// failover and traffic distribution preferences do not survive the merge. The merge splits each
// member's backendRef weight over its endpoints by their own weights, so replica count does not
// change the member's share. A blackhole endpoint carries dropWeight.
func mergeUdpAggregateLoadAssignment(clusterName string, members []udpMemberEndpoints, dropWeight uint32) *envoyendpointv3.ClusterLoadAssignment {
	// weighted holds an endpoint's uint64 weight until the scale-down below narrows the whole set to
	// uint32.
	type weighted struct {
		locality ir.PodLocality
		ep       *envoyendpointv3.LbEndpoint
		weight   uint64
	}
	var collected []weighted
	var total uint64

	for _, m := range members {
		// A member without a row yet adds nothing until the row arrives. Route resolution rejects
		// backends that never have rows (InlineEndpoints), so dropWeight already counts their weight.
		if m.cla == nil {
			continue
		}
		var memberTotal uint64
		for _, lle := range m.cla.GetEndpoints() {
			for _, ep := range lle.GetLbEndpoints() {
				memberTotal += endpointLbWeight(ep)
			}
		}
		// A Service with no endpoints is invalid per the UDPRoute spec, so its share drops, not
		// redistributes.
		if memberTotal == 0 {
			dropWeight += m.weight
			continue
		}
		for _, lle := range m.cla.GetEndpoints() {
			loc := lle.GetLocality()
			locality := ir.PodLocality{Region: loc.GetRegion(), Zone: loc.GetZone(), Subzone: loc.GetSubZone()}
			for _, ep := range lle.GetLbEndpoints() {
				w := uint64(m.weight) * udpWeightScale * endpointLbWeight(ep) / memberTotal
				if w == 0 {
					w = 1
				}
				clone := proto.Clone(ep).(*envoyendpointv3.LbEndpoint)
				collected = append(collected, weighted{locality: locality, ep: clone, weight: w})
				total += w
			}
		}
	}

	if dropWeight > 0 {
		bhWeight := uint64(dropWeight) * udpWeightScale
		collected = append(collected, weighted{ep: blackholeLbEndpoint(), weight: bhWeight})
		total += bhWeight
	}

	// Envoy caps both the endpoint weights summed within a locality and the locality weights summed
	// across a priority at the uint32 max. Each locality weight is the sum of its endpoints, so
	// scaling every endpoint weight down by one divisor until the grand total fits keeps both sums
	// under the cap while preserving the backendRef proportions.
	if total > math.MaxUint32 {
		div := (total + math.MaxUint32 - 1) / math.MaxUint32
		for i := range collected {
			scaled := collected[i].weight / div
			if scaled == 0 {
				scaled = 1
			}
			collected[i].weight = scaled
		}
	}

	byLocality := map[ir.PodLocality][]*envoyendpointv3.LbEndpoint{}
	var localityOrder []ir.PodLocality
	for _, w := range collected {
		//nolint:gosec // G115: weight is scaled below the uint32 max above
		w.ep.LoadBalancingWeight = wrapperspb.UInt32(uint32(w.weight))
		if _, ok := byLocality[w.locality]; !ok {
			localityOrder = append(localityOrder, w.locality)
		}
		byLocality[w.locality] = append(byLocality[w.locality], w.ep)
	}

	slices.SortFunc(localityOrder, func(a, b ir.PodLocality) int {
		return cmp.Compare(a.String(), b.String())
	})

	cla := &envoyendpointv3.ClusterLoadAssignment{ClusterName: clusterName}
	for _, locality := range localityOrder {
		eps := byLocality[locality]
		slices.SortFunc(eps, func(a, b *envoyendpointv3.LbEndpoint) int {
			return cmp.Compare(lbEndpointSortKey(a), lbEndpointSortKey(b))
		})
		var localityWeight uint64
		for _, ep := range eps {
			localityWeight += uint64(ep.GetLoadBalancingWeight().GetValue())
		}
		lle := &envoyendpointv3.LocalityLbEndpoints{
			LbEndpoints: eps,
			//nolint:gosec // G115: localityWeight sums endpoint weights already scaled below the uint32 max
			LoadBalancingWeight: wrapperspb.UInt32(uint32(localityWeight)),
		}
		if locality != (ir.PodLocality{}) {
			lle.Locality = &envoycorev3.Locality{
				Region:  locality.Region,
				Zone:    locality.Zone,
				SubZone: locality.Subzone,
			}
		}
		cla.Endpoints = append(cla.GetEndpoints(), lle)
	}
	return cla
}

// endpointLbWeight is an endpoint's load-balancing weight, which Envoy defaults to 1 when unset.
func endpointLbWeight(ep *envoyendpointv3.LbEndpoint) uint64 {
	if w := ep.GetLoadBalancingWeight(); w != nil {
		return uint64(w.GetValue())
	}
	return 1
}

// blackholeLbEndpoint returns an endpoint targeting the loopback discard port. The caller sets its
// weight alongside the member endpoints so the whole set scales together.
func blackholeLbEndpoint() *envoyendpointv3.LbEndpoint {
	return &envoyendpointv3.LbEndpoint{
		HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
			Endpoint: &envoyendpointv3.Endpoint{
				Address: &envoycorev3.Address{
					Address: &envoycorev3.Address_SocketAddress{
						SocketAddress: &envoycorev3.SocketAddress{
							Protocol:      envoycorev3.SocketAddress_TCP,
							Address:       udpBlackholeAddr,
							PortSpecifier: &envoycorev3.SocketAddress_PortValue{PortValue: udpBlackholePort},
						},
					},
				},
			},
		},
	}
}

func lbEndpointSortKey(ep *envoyendpointv3.LbEndpoint) string {
	sa := ep.GetEndpoint().GetAddress().GetSocketAddress()
	return sa.GetAddress() + ":" + strconv.FormatUint(uint64(sa.GetPortValue()), 10)
}

// hashUdpAggregate derives the aggregate row's hash from the inputs of its CLA: the members'
// row hashes, which already carry endpoint content and policy versions, their weights, and the
// drop weight.
func hashUdpAggregate(agg udpAggregate, members []udpMemberEndpoints) uint64 {
	hasher := fnv.New64a()
	utils.HashStringField(hasher, agg.clusterName)
	utils.HashUint64(hasher, uint64(agg.dropWeight))
	for _, m := range members {
		utils.HashUint64(hasher, uint64(m.weight))
		if m.cla == nil {
			utils.HashStringField(hasher, "pending")
			continue
		}
		utils.HashUint64(hasher, m.hash)
	}
	return hasher.Sum64()
}
