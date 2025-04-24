package waypoint

import (
	"context"

	istioannot "istio.io/api/annotation"
	networkingv1beta1 "istio.io/api/networking/v1beta1"
	istionetworking "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

var VirtualWaypointGK = schema.GroupKind{
	Group: "waypoint",
	Kind:  "waypoint",
}

func NewPlugin(
	ctx context.Context,
	commonCols *common.CommonCollections,
) extensionsplug.Plugin {
	queries := query.NewData(
		commonCols,
	)
	waypointQueries := waypointquery.NewQueries(
		commonCols,
		queries,
	)
	plugin := extensionsplug.Plugin{
		ContributesGwTranslator: func(gw *gwv1.Gateway) extensionsplug.KGwTranslator {
			if gw.Spec.GatewayClassName != wellknown.WaypointClassName {
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
		waypointQueries: waypointQueries,
		commonCols:      commonCols,
	}
	if commonCols.Settings.IngressUseWaypoints {
		plugin.ContributesPolicies = map[schema.GroupKind]extensionsplug.PolicyPlugin{
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
	waypointQueries waypointquery.WaypointQueries
	commonCols      *common.CommonCollections
}

func (t *PerClientProcessor) processBackend(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniqlyConnectedClient, in ir.BackendObjectIR, out *envoy_config_cluster_v3.Cluster) {
	// If the ucc has a waypoint gateway class we will let it have an EDS cluster
	gwKey := ir.ObjectSource{
		Group:     wellknown.GatewayGVK.GroupKind().Group,
		Kind:      wellknown.GatewayGVK.GroupKind().Kind,
		Name:      ucc.Labels[wellknown.GatewayNameLabel],
		Namespace: ucc.Namespace,
	}
	gwir := krt.FetchOne(kctx, t.commonCols.GatewayIndex.Gateways, krt.FilterKey(gwKey.ResourceName()))
	if gwir == nil || gwir.Obj == nil || gwir.Obj.Spec.GatewayClassName == wellknown.WaypointClassName {
		// no op
		return
	}

	// If the ucc doesn't have the ambient.istio.io/redirection=enabled annotation, we don't need to do anything
	if val, ok := ucc.Annotations[istioannot.AmbientRedirection.Name]; !ok || val != "enabled" {
		// no op
		return
	}

	// Only handle backends with the istio.io/ingress-use-waypoint label
	if val, ok := in.Obj.GetLabels()[wellknown.IngressUseWaypointLabel]; !ok || val != "true" {
		// Also check the service'snamespace for the label
		nsMeta := krt.FetchOne(kctx, t.commonCols.Namespaces, krt.FilterKey(in.Obj.GetNamespace()))
		if nsMeta == nil {
			return
		}
		if val, ok := nsMeta.Labels[wellknown.IngressUseWaypointLabel]; !ok || val != "true" {
			// Both the service and the namespace do not have the label, no op
			return
		}
	}

	// Verify that the service is indeed attached to a waypoint by querying the reverse
	// service index.
	waypointForService := t.waypointQueries.GetServiceWaypoint(kctx, ctx, in.Obj)
	if waypointForService == nil {
		// no op
		return
	}

	// All preliminary checks passed, process the ingress use waypoint
	processIngressUseWaypoint(in, out)
}

// processIngressUseWaypoint configures the cluster of the connected gateway to have a static
// inlined addresses of the destination service. This will cause the traffic from the kgateway
// to be redirected to the waypoint by the ztunnel.
func processIngressUseWaypoint(in ir.BackendObjectIR, out *envoy_config_cluster_v3.Cluster) {
	var addresses []string
	switch in.Obj.(type) {
	case *corev1.Service:
		addresses = serviceAddresses(in.Obj.(*corev1.Service))
	case *istionetworking.ServiceEntry:
		addresses = serviceEntryAddresses(in.Obj.(*istionetworking.ServiceEntry))
	}

	// Set the output cluster to be of type STATIC and instead of the default EDS and add
	// the addresses of the backend embedded into the CLA of this cluster config.
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_STATIC,
	}
	out.EdsClusterConfig = nil
	out.LoadAssignment = &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: out.GetName(),
		Endpoints:   make([]*envoy_config_endpoint_v3.LocalityLbEndpoints, 0, len(addresses)),
	}

	for _, addr := range addresses {
		out.GetLoadAssignment().Endpoints = append(out.GetLoadAssignment().GetEndpoints(), claEndpoint(addr, uint32(in.Port)))
	}
}

// serviceAddresses returns the addresses of the service. ClusterIPs are optional in a Service
// and if exists will include the address of ClusterIP.
// Value can also be "None" (headless service) in both ClusterIPs and ClusterIP.
func serviceAddresses(svc *corev1.Service) []string {
	var addrs []string
	if len(svc.Spec.ClusterIPs) > 0 {
		for _, ip := range svc.Spec.ClusterIPs {
			if ip != "" && ip != "None" {
				addrs = append(addrs, ip)
			}
		}
	}
	if len(addrs) == 0 && len(svc.Spec.ClusterIP) > 0 && svc.Spec.ClusterIP != "None" {
		addrs = []string{svc.Spec.ClusterIP}
	}
	return addrs
}

func serviceEntryAddresses(se *istionetworking.ServiceEntry) []string {
	addrs := append(se.Spec.GetAddresses(), slices.Map(se.Status.GetAddresses(), func(a *networkingv1beta1.ServiceEntryAddress) string {
		return a.Value
	})...)
	return addrs
}

func claEndpoint(address string, port uint32) *envoy_config_endpoint_v3.LocalityLbEndpoints {
	return &envoy_config_endpoint_v3.LocalityLbEndpoints{
		LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
			{
				HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
					Endpoint: &envoy_config_endpoint_v3.Endpoint{
						Address: &envoy_config_core_v3.Address{
							Address: &envoy_config_core_v3.Address_SocketAddress{
								SocketAddress: &envoy_config_core_v3.SocketAddress{
									Address: address,
									PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
										PortValue: port,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
