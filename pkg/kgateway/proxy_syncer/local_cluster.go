package proxy_syncer

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const localClusterEndpointPort uint32 = 19000

type localClusterEndpoint struct {
	resourceName string
	address      string
	locality     ir.PodLocality
}

func NewPerClientLocalClusterEndpoints(
	krtopts krtutil.KrtOptions,
	uccs krt.Collection[ir.UniquelyConnectedClient],
	localityPods krt.Collection[krtcollections.LocalityPod],
) PerClientEnvoyEndpoints {
	endpoints := krt.NewCollection(uccs, func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) *UccWithEndpoints {
		localClusterName, gatewayName, gatewayNamespace := localClusterInfo(ucc)
		if localClusterName == "" || gatewayName == "" || gatewayNamespace == "" {
			return nil
		}

		cla := buildLocalClusterLoadAssignment(localClusterName, gatewayName, gatewayNamespace, krt.Fetch(kctx, localityPods))
		return &UccWithEndpoints{
			Client:        ucc,
			Endpoints:     cla,
			EndpointsHash: hashLocalClusterLoadAssignment(cla),
			endpointsName: localClusterName,
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

func localClusterInfo(ucc ir.UniquelyConnectedClient) (clusterName, gatewayName, gatewayNamespace string) {
	gatewayNamespace = ucc.Namespace
	gatewayName = gatewayNameFromLabels(ucc.Labels)

	roleParts := strings.Split(ucc.Role, ir.KeyDelimiter)
	if len(roleParts) == 3 {
		if gatewayNamespace == "" {
			gatewayNamespace = roleParts[1]
		}
		if gatewayName == "" {
			gatewayName = roleParts[2]
		}
	}

	if gatewayName == "" || gatewayNamespace == "" {
		return "", gatewayName, gatewayNamespace
	}
	return LocalClusterName(gatewayName, gatewayNamespace), gatewayName, gatewayNamespace
}

// LocalClusterName returns the name of the per-gateway "local cluster" EDS resource that
// kgateway programs for native zone-aware routing. It is the single source of truth for this
// name and must stay in sync with the bootstrap config produced by the Helm template
// (see kgateway.gateway.fullname in pkg/kgateway/helm/envoy/templates/_helpers.tpl).
func LocalClusterName(gatewayName, gatewayNamespace string) string {
	return fmt.Sprintf("%s.%s", kubeutils.SafeGatewayLabelValue(gatewayName), gatewayNamespace)
}

func buildLocalClusterLoadAssignment(
	clusterName string,
	gatewayName string,
	gatewayNamespace string,
	pods []krtcollections.LocalityPod,
) *envoyendpointv3.ClusterLoadAssignment {
	localEndpoints := make([]localClusterEndpoint, 0, len(pods))
	for _, pod := range pods {
		if pod.Namespace != gatewayNamespace || gatewayNameFromLabels(pod.AugmentedLabels) != gatewayName {
			continue
		}
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

	sort.Slice(localEndpoints, func(i, j int) bool {
		if localEndpoints[i].locality != localEndpoints[j].locality {
			return localEndpoints[i].locality.String() < localEndpoints[j].locality.String()
		}
		if localEndpoints[i].resourceName != localEndpoints[j].resourceName {
			return localEndpoints[i].resourceName < localEndpoints[j].resourceName
		}
		return localEndpoints[i].address < localEndpoints[j].address
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

func gatewayNameFromLabels(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	if gatewayName := labels[wellknown.GatewayNameAnnotation]; gatewayName != "" {
		return gatewayName
	}
	return labels[wellknown.GatewayNameLabel]
}

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
			utils.HashStringField(hasher, fmt.Sprintf("%d", socketAddress.GetPortValue()))
		}
	}
	return hasher.Sum64()
}
