package waypoint

import (
	"context"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	istioannot "istio.io/api/annotation"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/util/sets"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var VirtualWaypointGK = schema.GroupKind{
	Group: "waypoint",
	Kind:  "waypoint",
}

func NewPlugin(
	ctx context.Context,
	commonCols *collections.CommonCollections,
	waypointGatewayClassName string,
) sdk.Plugin {
	queries := query.NewData(
		commonCols,
	)
	waypointQueries := waypointquery.NewQueries(
		commonCols,
		queries,
	)
	plugin := sdk.Plugin{
		ContributesGwTranslator: func(gw *gwv1.Gateway) sdk.KGwTranslator {
			if string(gw.Spec.GatewayClassName) != waypointGatewayClassName {
				return nil
			}

			return NewTranslator(queries, waypointQueries, commonCols.Settings)
		},
		ExtraHasSynced: func() bool {
			return waypointQueries.HasSynced()
		},
	}

	// If ingress use waypoints is enabled, we need to process the backends per client. Depending
	// on the gateway class of the client, we will either add an EDS cluster or a static cluster.
	// The static cluster will be used to redirect the traffic to the waypoint service by using the
	// backend addresses (VIPs) as the endpoints. This will cause the traffic from the ingress to be
	// redirected to the waypoint by the ztunnel.
	pcp := &PerClientProcessor{
		waypointQueries:          waypointQueries,
		commonCols:               commonCols,
		waypointGatewayClassName: waypointGatewayClassName,
	}
	if commonCols.Settings.IngressUseWaypoints {
		plugin.ContributesPolicies = map[schema.GroupKind]sdk.PolicyPlugin{
			// TODO: Currently endpoints are still being added to an EDS CLA out of this plugin.
			// Contributing a PerClientProcessEndpoints function can return an empty CLA but
			// it is still redundant.
			VirtualWaypointGK: {
				PerClientProcessBackend: pcp.processBackend,
			},
		}
	}

	return plugin
}

type PerClientProcessor struct {
	waypointQueries          waypointquery.WaypointQueries
	commonCols               *collections.CommonCollections
	waypointGatewayClassName string
}

func (t *PerClientProcessor) processBackend(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniqlyConnectedClient, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
	// If the ucc has a waypoint gateway class we will let it have an EDS cluster
	// First try the annotation (for long gateway names > 63 chars), then fall back to the label
	gwName := ucc.Labels[wellknown.GatewayNameAnnotation]
	if gwName == "" {
		gwName = ucc.Labels[wellknown.GatewayNameLabel]
	}
	gwKey := ir.ObjectSource{
		Group:     wellknown.GatewayGVK.GroupKind().Group,
		Kind:      wellknown.GatewayGVK.GroupKind().Kind,
		Name:      gwName,
		Namespace: ucc.Namespace,
	}
	gwir := krt.FetchOne(kctx, t.commonCols.GatewayIndex.Gateways, krt.FilterKey(gwKey.ResourceName()))
	if gwir == nil || gwir.Obj == nil || string(gwir.Obj.Spec.GatewayClassName) == t.waypointGatewayClassName {
		// no op
		return
	}

	// If the ucc doesn't have the ambient.istio.io/redirection=enabled annotation, we don't need to do anything
	// For efficiency, the specific annotation (if exists) has been addeded to the augmented labels of the ucc.
	if val, ok := ucc.Labels[istioannot.AmbientRedirection.Name]; !ok || val != "enabled" {
		// no op
		return
	}

	// Only handle backends with the istio.io/ingress-use-waypoint label
	if !hasIngressUseWaypointLabel(kctx, t.commonCols, in) {
		// Neither the backend nor any relevant namespace/alias has the label, skip processing
		return
	}

	// Verify that the service is indeed attached to a waypoint by querying the reverse
	// service index.
	waypointForService := t.waypointQueries.GetServiceWaypoint(kctx, ctx, in.Obj)
	if waypointForService == nil {
		// no op
		return
	}

	// All preliminary checks passed, process the ingress use waypoint
	processIngressUseWaypoint(in, out, &t.commonCols.Settings)
}

// processIngressUseWaypoint configures the cluster of the connected gateway to have a static
// inlined addresses of the destination service. This will cause the traffic from the kgateway
// to be redirected to the waypoint by the ztunnel.
// Addresses are sorted based on DNS lookup family setting, with the primary address in Address
// and additional addresses in AdditionalAddresses.
func processIngressUseWaypoint(in ir.BackendObjectIR, out *envoyclusterv3.Cluster, settings *apisettings.Settings) {
	addresses := waypointquery.BackendAddresses(in)

	// Sort addresses based on DNS lookup family setting. Since this is a static cluster
	// with inlined addresses, we can't use DnsLookupFamily (which only applies to DNS-based
	// discovery). Instead, we sort the addresses based on the setting and use the primary
	// address in Address and additional addresses in AdditionalAddresses.
	sortedAddresses := sortAddressesByDnsLookupFamily(addresses, settings)

	// Set the output cluster to be of type STATIC and instead of the default EDS and add
	// the addresses of the backend embedded into the CLA of this cluster config.
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
		Type: envoyclusterv3.Cluster_STATIC,
	}
	out.EdsClusterConfig = nil
	out.LoadAssignment = &envoyendpointv3.ClusterLoadAssignment{
		ClusterName: out.GetName(),
		Endpoints:   make([]*envoyendpointv3.LocalityLbEndpoints, 0, 1),
	}

	if endpoint := claEndpoint(sortedAddresses, uint32(in.Port)); endpoint != nil { //nolint:gosec // G115: BackendObjectIR.Port is int32 representing a port number, always in valid range
		out.GetLoadAssignment().Endpoints = append(out.GetLoadAssignment().GetEndpoints(), endpoint)
	}
}

// claEndpoint creates a LocalityLbEndpoints with the primary address in Address
// and additional addresses in AdditionalAddresses.
func claEndpoint(addresses []string, port uint32) *envoyendpointv3.LocalityLbEndpoints {
	if len(addresses) == 0 {
		return nil
	}

	// Primary address goes in Address
	primaryAddr := addresses[0]
	endpoint := &envoyendpointv3.Endpoint{
		Address: &envoycorev3.Address{
			Address: &envoycorev3.Address_SocketAddress{
				SocketAddress: &envoycorev3.SocketAddress{
					Address: primaryAddr,
					PortSpecifier: &envoycorev3.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
	}

	// Additional addresses go in AdditionalAddresses
	if len(addresses) > 1 {
		additionalAddresses := make([]*envoyendpointv3.Endpoint_AdditionalAddress, 0, len(addresses)-1)
		for _, addr := range addresses[1:] {
			additionalAddresses = append(additionalAddresses, &envoyendpointv3.Endpoint_AdditionalAddress{
				Address: &envoycorev3.Address{
					Address: &envoycorev3.Address_SocketAddress{
						SocketAddress: &envoycorev3.SocketAddress{
							Address: addr,
							PortSpecifier: &envoycorev3.SocketAddress_PortValue{
								PortValue: port,
							},
						},
					},
				},
			})
		}
		endpoint.AdditionalAddresses = additionalAddresses
	}

	return &envoyendpointv3.LocalityLbEndpoints{
		LbEndpoints: []*envoyendpointv3.LbEndpoint{
			{
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
					Endpoint: endpoint,
				},
			},
		},
	}
}

// sortAddressesByDnsLookupFamily sorts addresses based on the DNS lookup family setting.
// Returns a sorted list of addresses where the first address will be used as primary
// (in Address) and the rest as additional (in AdditionalAddresses).
// Since static clusters can't use DnsLookupFamily (it only applies to DNS-based discovery),
// we sort the addresses based on the setting.
func sortAddressesByDnsLookupFamily(addresses []string, settings *apisettings.Settings) []string {
	// Default to V4_PREFERRED if settings are not available
	dnsLookupFamily := apisettings.DnsLookupFamilyV4Preferred
	if settings != nil {
		dnsLookupFamily = settings.DnsLookupFamily
	}

	// For ALL mode, we don't need to separate by family - just return all addresses
	if dnsLookupFamily == apisettings.DnsLookupFamilyAll {
		return addresses
	}

	// Separate IPv4 and IPv6 addresses for other modes
	var ipv4Addrs, ipv6Addrs []string
	for _, addr := range addresses {
		validIPv4, _, err := utils.IsIpv4Address(addr)
		if err != nil {
			// Skip invalid addresses
			continue
		}
		if validIPv4 {
			ipv4Addrs = append(ipv4Addrs, addr)
		} else {
			ipv6Addrs = append(ipv6Addrs, addr)
		}
	}

	// Sort based on DNS lookup family setting
	var sortedAddresses []string
	switch dnsLookupFamily {
	case apisettings.DnsLookupFamilyV4Only:
		// Only IPv4 addresses
		sortedAddresses = ipv4Addrs
	case apisettings.DnsLookupFamilyV6Only:
		// Only IPv6 addresses
		sortedAddresses = ipv6Addrs
	case apisettings.DnsLookupFamilyV4Preferred:
		// IPv4 first, then IPv6 as additional addresses
		sortedAddresses = append(ipv4Addrs, ipv6Addrs...)
	case apisettings.DnsLookupFamilyAuto:
		// IPv6 first, then IPv4 as additional addresses
		sortedAddresses = append(ipv6Addrs, ipv4Addrs...)
	default:
		// Default to V4_PREFERRED for unknown values
		sortedAddresses = append(ipv4Addrs, ipv6Addrs...)
	}

	return sortedAddresses
}

// hasIngressUseWaypointLabel checks if the backend or any relevant namespace/alias has the ingress-use-waypoint label.
func hasIngressUseWaypointLabel(kctx krt.HandlerContext, commonCols *collections.CommonCollections, in ir.BackendObjectIR) bool {
	// Check the backend's own label first
	if val, ok := in.Obj.GetLabels()[wellknown.IngressUseWaypointLabel]; ok && val == "true" {
		return true
	}

	// Then, check the namespace of the backend object itself
	backendNs := in.Obj.GetNamespace()
	if backendNs != "" {
		nsMeta := krt.FetchOne(kctx, commonCols.Namespaces, krt.FilterKey(backendNs))
		if nsMeta != nil {
			if val, ok := nsMeta.Labels[wellknown.IngressUseWaypointLabel]; ok && val == "true" {
				return true
			}
		}
	}

	// If not found in backend's own namespace, check aliases
	seenNs := sets.New[string]()
	for _, alias := range in.Aliases {
		ns := alias.GetNamespace()
		if ns == "" || seenNs.InsertContains(ns) {
			continue
		}
		nsMeta := krt.FetchOne(kctx, commonCols.Namespaces, krt.FilterKey(ns))
		if nsMeta != nil {
			if val, ok := nsMeta.Labels[wellknown.IngressUseWaypointLabel]; ok && val == "true" {
				return true
			}
		}
	}

	// If we get here, we didn't find any namespace with the ingress-use-waypoint label
	return false
}
