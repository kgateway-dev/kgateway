package proxy_syncer

import (
	"cmp"
	"hash/fnv"
	"slices"
	"strconv"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// udpWeightScale scales backendRef weights into per-endpoint load-balancing weights. Envoy only
// needs proportionality, so a fixed scale is enough.
const udpWeightScale uint64 = 1000

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
// the weighted member backends whose endpoints are unioned, plus the combined weight of any invalid
// backends.
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
		// Must mirror translateUdpFilterChain's rule that more than one backendRef means a
		// synthetic cluster. The listener emits the CDS cluster for exactly the multi-backend
		// routes, and every CDS cluster needs a CLA or Envoy withholds the client's whole EDS
		// response (issue #14471). Members may be empty when all backends are invalid or
		// zero-weight, a valid empty CLA that drops.
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

// NewPerClientUdpAggregateEndpoints builds the per-client CLA for each UDP aggregate cluster. The
// endpoint set is client-independent, so kgateway builds the CLA once and emits it to every client,
// keeping the CLA present whenever the synthetic CDS cluster is and avoiding the withheld EDS in
// issue #14471.
func NewPerClientUdpAggregateEndpoints(
	krtopts krtutil.KrtOptions,
	uccs krt.Collection[ir.UniquelyConnectedClient],
	aggregates krt.Collection[udpAggregate],
	backendEndpoints krt.Collection[ir.EndpointsForBackend],
) PerClientEnvoyEndpoints {
	epByBackend := krtpkg.UnnamedIndex(backendEndpoints, func(e ir.EndpointsForBackend) []string {
		return []string{e.UpstreamResourceName}
	})

	endpoints := krt.NewManyCollection(aggregates, func(kctx krt.HandlerContext, agg udpAggregate) []UccWithEndpoints {
		cla := buildUdpAggregateLoadAssignment(kctx, agg, backendEndpoints, epByBackend)
		hash := hashUdpAggregateLoadAssignment(cla)
		clients := krt.Fetch(kctx, uccs)
		ret := make([]UccWithEndpoints, 0, len(clients))
		for _, ucc := range clients {
			ret = append(ret, UccWithEndpoints{
				Client:        ucc,
				Endpoints:     cla,
				EndpointsHash: hash,
				endpointsName: agg.clusterName,
				resourceName:  uccEndpointsResourceName(ucc, agg.clusterName),
			})
		}
		return ret
	}, krtopts.ToOptions("UdpAggregateEndpoints")...)

	idx := krtpkg.UnnamedIndex(endpoints, func(u UccWithEndpoints) []string {
		return []string{u.Client.ResourceName()}
	})

	return PerClientEnvoyEndpoints{endpoints: endpoints, index: idx}
}

// udpMemberEndpoints pairs a member's backendRef weight with its resolved endpoints, decoupling
// the krt Fetch from the (unit-tested) weight-merge below.
type udpMemberEndpoints struct {
	weight uint32
	efbs   []ir.EndpointsForBackend
}

func buildUdpAggregateLoadAssignment(
	kctx krt.HandlerContext,
	agg udpAggregate,
	backendEndpoints krt.Collection[ir.EndpointsForBackend],
	epByBackend krt.Index[string, ir.EndpointsForBackend],
) *envoyendpointv3.ClusterLoadAssignment {
	members := make([]udpMemberEndpoints, 0, len(agg.members))
	for _, m := range agg.members {
		members = append(members, udpMemberEndpoints{
			weight: m.weight,
			efbs:   krt.Fetch(kctx, backendEndpoints, krt.FilterIndex(epByBackend, m.backendResourceName)),
		})
	}
	return mergeUdpAggregateLoadAssignment(agg.clusterName, members, agg.dropWeight)
}

// mergeUdpAggregateLoadAssignment unions the members' endpoints into one weighted CLA. Each
// endpoint's weight is its member's backendRef weight scaled by udpWeightScale and divided by the
// member's live endpoint count, so a member's total weight stays proportional to its backendRef
// weight regardless of replica count. dropWeight is added as a single blackhole endpoint.
func mergeUdpAggregateLoadAssignment(clusterName string, members []udpMemberEndpoints, dropWeight uint32) *envoyendpointv3.ClusterLoadAssignment {
	byLocality := map[ir.PodLocality][]*envoyendpointv3.LbEndpoint{}
	var localityOrder []ir.PodLocality

	addToLocality := func(locality ir.PodLocality, ep *envoyendpointv3.LbEndpoint) {
		if _, ok := byLocality[locality]; !ok {
			localityOrder = append(localityOrder, locality)
		}
		byLocality[locality] = append(byLocality[locality], ep)
	}

	for _, m := range members {
		count := 0
		for _, efb := range m.efbs {
			for _, eps := range efb.LbEps {
				count += len(eps)
			}
		}
		if count == 0 {
			continue
		}
		//nolint:gosec // G115: count is a positive endpoint count (guarded > 0 above)
		perEndpointWeight := (uint64(m.weight) * udpWeightScale) / uint64(count)
		if perEndpointWeight == 0 {
			perEndpointWeight = 1
		}
		if perEndpointWeight > uint64(^uint32(0)) {
			perEndpointWeight = uint64(^uint32(0))
		}
		for _, efb := range m.efbs {
			for locality, eps := range efb.LbEps {
				for _, ep := range eps {
					if ep.LbEndpoint == nil {
						continue
					}
					clone := proto.Clone(ep.LbEndpoint).(*envoyendpointv3.LbEndpoint)
					//nolint:gosec // G115: perEndpointWeight is clamped to uint32 max above
					clone.LoadBalancingWeight = wrapperspb.UInt32(uint32(perEndpointWeight))
					addToLocality(locality, clone)
				}
			}
		}
	}

	if dropWeight > 0 {
		bhWeight := uint64(dropWeight) * udpWeightScale
		if bhWeight == 0 {
			bhWeight = 1
		}
		if bhWeight > uint64(^uint32(0)) {
			bhWeight = uint64(^uint32(0))
		}
		//nolint:gosec // G115: bhWeight is clamped to uint32 max above
		addToLocality(ir.PodLocality{}, blackholeLbEndpoint(uint32(bhWeight)))
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
		var localityWeight uint32
		for _, ep := range eps {
			localityWeight += ep.GetLoadBalancingWeight().GetValue()
		}
		lle := &envoyendpointv3.LocalityLbEndpoints{
			LbEndpoints:         eps,
			LoadBalancingWeight: wrapperspb.UInt32(localityWeight),
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

// blackholeLbEndpoint returns a weighted endpoint targeting the loopback discard port.
func blackholeLbEndpoint(weight uint32) *envoyendpointv3.LbEndpoint {
	return &envoyendpointv3.LbEndpoint{
		LoadBalancingWeight: wrapperspb.UInt32(weight),
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

func hashUdpAggregateLoadAssignment(cla *envoyendpointv3.ClusterLoadAssignment) uint64 {
	hasher := fnv.New64a()
	utils.HashStringField(hasher, cla.GetClusterName())
	for _, lle := range cla.GetEndpoints() {
		loc := lle.GetLocality()
		utils.HashStringField(hasher, loc.GetRegion())
		utils.HashStringField(hasher, loc.GetZone())
		utils.HashStringField(hasher, loc.GetSubZone())
		utils.HashStringField(hasher, strconv.FormatUint(uint64(lle.GetLoadBalancingWeight().GetValue()), 10))
		for _, ep := range lle.GetLbEndpoints() {
			sa := ep.GetEndpoint().GetAddress().GetSocketAddress()
			utils.HashStringField(hasher, sa.GetAddress())
			utils.HashStringField(hasher, strconv.FormatUint(uint64(sa.GetPortValue()), 10))
			utils.HashStringField(hasher, strconv.FormatUint(uint64(ep.GetLoadBalancingWeight().GetValue()), 10))
		}
	}
	return hasher.Sum64()
}
