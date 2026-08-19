package proxy_syncer

import (
	"cmp"
	"hash/fnv"
	"slices"
	"strconv"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// localClusterEndpointPort is the port used in local-cluster CLA endpoints.
// The value is arbitrary, and the admin port is used since it is guaranteed
// to be listening on every Envoy pod.
const localClusterEndpointPort uint32 = 19000

// localClusterEndpoint is one gateway pod as it appears in the local-cluster CLA:
// just the address and locality, plus the pod's resource name for stable ordering.
type localClusterEndpoint struct {
	resourceName string
	address      string
	locality     ir.PodLocality
}

// gatewayPodIndexKey keys gateway pods by the Gateway they belong to.
func gatewayPodIndexKey(namespace, gatewayName string) string {
	return namespace + "/" + gatewayName
}

// NewPerClientLocalClusterEndpoints builds the CLA describing a gateway's own pods
// back to that gateway — the "local cluster" Envoy compares against when deciding
// how much traffic to keep in-zone. Each client gets a CLA covering its sibling
// pods, so the collection is per client but not per backend.
//
// A client that has never subscribed to the resource is skipped rather than served
// an empty CLA: an unrequested resource makes go-control-plane withhold the client's
// entire EDS response (see the KnowsLocalCluster branch below).
func NewPerClientLocalClusterEndpoints(
	krtopts krtutil.KrtOptions,
	uccs krt.Collection[ir.UniquelyConnectedClient],
	localityPods krt.Collection[krtcollections.LocalityPod],
) PerClientEnvoyEndpoints {
	podsByGateway := krtpkg.UnnamedIndex(localityPods, func(pod krtcollections.LocalityPod) []string {
		gwName := ir.GatewayNameFromLabels(pod.AugmentedLabels)
		if gwName == "" {
			return nil
		}
		return []string{gatewayPodIndexKey(pod.Namespace, gwName)}
	})

	endpoints := krt.NewCollection(uccs, func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) *UccWithEndpoints {
		if !ucc.KnowsLocalCluster {
			// Client's own EDS subscription has never named this resource (old Envoy with
			// no matching static bootstrap cluster). Never emit it for this client: an
			// unrequested resource in the snapshot makes go-control-plane's ADS "superset"
			// check withhold the client's *entire* EDS response. See issue #14471.
			return nil
		}
		localClusterName, gatewayName, gatewayNamespace := ucc.LocalClusterInfo()
		if localClusterName == "" || gatewayName == "" || gatewayNamespace == "" {
			return nil
		}

		logger.Debug("building local cluster CLA", "local_cluster_name", localClusterName, "gateway", gatewayName, "namespace", gatewayNamespace)
		gwPods := krt.Fetch(kctx, localityPods, krt.FilterIndex(podsByGateway, gatewayPodIndexKey(gatewayNamespace, gatewayName)))
		cla := buildLocalClusterLoadAssignment(localClusterName, gwPods)
		return &UccWithEndpoints{
			Client:        ucc,
			Endpoints:     sharedproto.Wrap(cla),
			EndpointsHash: hashLocalClusterLoadAssignment(cla),
			endpointsName: localClusterName,
			resourceName:  uccEndpointsResourceName(ucc, localClusterName),
		}
	}, krtopts.ToOptions("LocalClusterEndpoints")...)

	idx := krtpkg.UnnamedIndex(endpoints, func(ucc UccWithEndpoints) []string {
		return []string{ucc.Client.ResourceName()}
	})

	return PerClientEnvoyEndpoints{
		endpoints: endpoints,
		index:     idx,
	}
}

// buildLocalClusterLoadAssignment groups the gateway's pods by locality into a CLA,
// weighting each locality by its pod count so Envoy's zone-aware routing sees the
// real distribution of the fleet. Pods with no address yet are skipped.
//
// Output is deterministic: pods are sorted by locality, then resource name, then
// address before grouping, so an unchanged fleet always hashes to the same value
// regardless of the order KRT returned the pods in.
func buildLocalClusterLoadAssignment(
	clusterName string,
	pods []krtcollections.LocalityPod,
) *envoyendpointv3.ClusterLoadAssignment {
	localEndpoints := make([]localClusterEndpoint, 0, len(pods))
	for _, pod := range pods {
		address := pod.Address()
		if address == "" {
			continue
		}
		localEndpoints = append(localEndpoints, localClusterEndpoint{
			resourceName: pod.ResourceName(),
			address:      address,
			locality:     pod.Locality,
		})
	}

	slices.SortFunc(localEndpoints, func(a, b localClusterEndpoint) int {
		if a.locality != b.locality {
			return cmp.Compare(a.locality.String(), b.locality.String())
		}
		if a.resourceName != b.resourceName {
			return cmp.Compare(a.resourceName, b.resourceName)
		}
		return cmp.Compare(a.address, b.address)
	})

	endpointsByLocality := make(map[ir.PodLocality][]localClusterEndpoint)
	localities := make([]ir.PodLocality, 0)
	for _, endpoint := range localEndpoints {
		if _, exists := endpointsByLocality[endpoint.locality]; !exists {
			localities = append(localities, endpoint.locality)
		}
		endpointsByLocality[endpoint.locality] = append(endpointsByLocality[endpoint.locality], endpoint)
	}

	cla := &envoyendpointv3.ClusterLoadAssignment{ClusterName: clusterName}
	for _, locality := range localities {
		endpoints := endpointsByLocality[locality]
		localityEndpoints := &envoyendpointv3.LocalityLbEndpoints{
			Locality: &envoycorev3.Locality{
				Region:  locality.Region,
				Zone:    locality.Zone,
				SubZone: locality.Subzone,
			},
			LoadBalancingWeight: wrapperspb.UInt32(uint32(len(endpoints))), //nolint:gosec // bounded by pod list length
		}
		if locality == (ir.PodLocality{}) {
			localityEndpoints.Locality = nil
		}

		for _, endpoint := range endpoints {
			localityEndpoints.LbEndpoints = append(localityEndpoints.GetLbEndpoints(), &envoyendpointv3.LbEndpoint{
				LoadBalancingWeight: wrapperspb.UInt32(1),
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
					Endpoint: &envoyendpointv3.Endpoint{
						Address: &envoycorev3.Address{
							Address: &envoycorev3.Address_SocketAddress{
								SocketAddress: &envoycorev3.SocketAddress{
									Protocol: envoycorev3.SocketAddress_TCP,
									Address:  endpoint.address,
									PortSpecifier: &envoycorev3.SocketAddress_PortValue{
										PortValue: localClusterEndpointPort,
									},
								},
							},
						},
					},
				},
			})
		}
		cla.Endpoints = append(cla.GetEndpoints(), localityEndpoints)
	}

	return cla
}

// hashLocalClusterLoadAssignment versions the local-cluster CLA for KRT change
// detection, hashing the fields that actually reach Envoy — cluster name, and each
// locality's endpoint addresses and ports. Pod churn that changes none of those
// (a relabeled pod, a restarted pod keeping its IP) leaves the hash alone and so
// does not republish EDS.
func hashLocalClusterLoadAssignment(cla *envoyendpointv3.ClusterLoadAssignment) uint64 {
	hasher := fnv.New64a()
	utils.HashStringField(hasher, cla.GetClusterName())
	for _, localityEndpoints := range cla.GetEndpoints() {
		locality := localityEndpoints.GetLocality()
		utils.HashStringField(hasher, locality.GetRegion())
		utils.HashStringField(hasher, locality.GetZone())
		utils.HashStringField(hasher, locality.GetSubZone())
		for _, lbEndpoint := range localityEndpoints.GetLbEndpoints() {
			socketAddress := lbEndpoint.GetEndpoint().GetAddress().GetSocketAddress()
			utils.HashStringField(hasher, socketAddress.GetAddress())
			utils.HashStringField(hasher, strconv.FormatUint(uint64(socketAddress.GetPortValue()), 10))
		}
	}
	return hasher.Sum64()
}
