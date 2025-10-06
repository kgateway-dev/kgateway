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

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
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
	gwKey := ir.ObjectSource{
		Group:     wellknown.GatewayGVK.GroupKind().Group,
		Kind:      wellknown.GatewayGVK.GroupKind().Kind,
		Name:      ucc.Labels[wellknown.GatewayNameLabel],
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
	processIngressUseWaypoint(in, out)
}

// processIngressUseWaypoint configures the cluster of the connected gateway to have a static
// inlined addresses of the destination service. This will cause the traffic from the kgateway
// to be redirected to the waypoint by the ztunnel.
func processIngressUseWaypoint(in ir.BackendObjectIR, out *envoyclusterv3.Cluster) {
	addresses := waypointquery.BackendAddresses(in)

	// Set the output cluster to be of type STATIC and instead of the default EDS and add
	// the addresses of the backend embedded into the CLA of this cluster config.
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
		Type: envoyclusterv3.Cluster_STATIC,
	}
	out.EdsClusterConfig = nil
	out.LoadAssignment = &envoyendpointv3.ClusterLoadAssignment{
		ClusterName: out.GetName(),
		Endpoints:   make([]*envoyendpointv3.LocalityLbEndpoints, 0, len(addresses)),
	}

	for _, addr := range addresses {
		out.GetLoadAssignment().Endpoints = append(out.GetLoadAssignment().GetEndpoints(), claEndpoint(addr, uint32(in.Port))) //nolint:gosec // G115: BackendObjectIR.Port is int32 representing a port number, always in valid range
	}
}

func claEndpoint(address string, port uint32) *envoyendpointv3.LocalityLbEndpoints {
	return &envoyendpointv3.LocalityLbEndpoints{
		LbEndpoints: []*envoyendpointv3.LbEndpoint{
			{
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
					Endpoint: &envoyendpointv3.Endpoint{
						Address: &envoycorev3.Address{
							Address: &envoycorev3.Address_SocketAddress{
								SocketAddress: &envoycorev3.SocketAddress{
									Address: address,
									PortSpecifier: &envoycorev3.SocketAddress_PortValue{
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
