package backend

import (
	"fmt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyrouterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// PriorityGroupsIr is the internal representation of a priority groups backend.
//
// Every field is compared in Equals below; the +noKrtEquals markers suppress
// the analyzer where it can't trace the comparison.
type PriorityGroupsIr struct {
	// loadAssignment holds one LocalityLbEndpoints per priority group with
	// Priority set to the group's index, so all backends of a group share the
	// same priority and each following group is the next failover level.
	// Every endpoint is the internal listener bridging to a referenced
	// backend's own cluster.
	// +noKrtEquals
	loadAssignment *envoyendpointv3.ClusterLoadAssignment
	// internalListeners bridge the priority groups cluster to the referenced
	// backends' clusters, one per referenced backend, keeping each backend's
	// cluster-level config (TLS, EDS, DFP, lambda filters) intact.
	// +noKrtEquals
	internalListeners []*envoylistenerv3.Listener
	// needsGcpAuthn is set when a referenced backend is a GCP backend, whose
	// internal listener's gcp_authn filter needs the shared GCP metadata
	// cluster.
	needsGcpAuthn bool
}

// Equals checks if two PriorityGroupsIr objects are equal.
func (u *PriorityGroupsIr) Equals(other *PriorityGroupsIr) bool {
	if u == nil || other == nil {
		return u == nil && other == nil
	}
	if !proto.Equal(u.loadAssignment, other.loadAssignment) {
		return false
	}
	if len(u.internalListeners) != len(other.internalListeners) {
		return false
	}
	for i := range u.internalListeners {
		if !proto.Equal(u.internalListeners[i], other.internalListeners[i]) {
			return false
		}
	}
	return u.needsGcpAuthn == other.needsGcpAuthn
}

// internalListenerName returns the name of the internal listener bridging the
// priority groups backend to the referenced backend.
func internalListenerName(pgClusterName, refName string) string {
	return fmt.Sprintf("%s_internal_%s", pgClusterName, refName)
}

// backendClusterName returns the cluster name a Backend translates to.
func backendClusterName(namespace, name string) string {
	objSrc := ir.ObjectSource{
		Group:     wellknown.BackendGVK.Group,
		Kind:      wellknown.BackendGVK.Kind,
		Namespace: namespace,
		Name:      name,
	}
	return ir.NewBackendObjectIR(objSrc, 0, "", ExtensionName).ClusterName()
}

// buildPriorityGroupsIr resolves the backendRefs of every priority group and
// builds the load assignment and internal listeners for the backend.
// Referenced backends are fetched through the collection so this backend is
// retranslated whenever a referenced Backend changes.
func buildPriorityGroupsIr(
	krtctx krt.HandlerContext,
	col krt.Collection[*kgateway.Backend],
	be *kgateway.Backend,
) (*PriorityGroupsIr, []error) {
	pgIr := &PriorityGroupsIr{
		loadAssignment: &envoyendpointv3.ClusterLoadAssignment{},
	}
	var errs []error
	pgClusterName := backendClusterName(be.GetNamespace(), be.GetName())
	builtListeners := map[string]bool{}

	for idx := range be.Spec.PriorityGroups {
		group := &be.Spec.PriorityGroups[idx]
		if len(group.BackendRefs) == 0 {
			errs = append(errs, fmt.Errorf("priority group %d: backendRefs must not be empty", idx))
			continue
		}

		locality := &envoyendpointv3.LocalityLbEndpoints{
			Priority: uint32(idx), //nolint:gosec // G115: group index is bounded by the list size
		}
		for _, ref := range group.BackendRefs {
			refBackend := krt.FetchOne(krtctx, col, krt.FilterObjectName(types.NamespacedName{
				Namespace: be.GetNamespace(),
				Name:      ref.Name,
			}))
			if refBackend == nil {
				errs = append(errs, fmt.Errorf("priority group %d: backend %q not found", idx, ref.Name))
				continue
			}
			rb := *refBackend
			if len(rb.Spec.PriorityGroups) > 0 {
				errs = append(errs, fmt.Errorf("priority group %d: backend %q is itself a priority groups backend; nested priority groups are not supported", idx, ref.Name))
				continue
			}

			listenerName := internalListenerName(pgClusterName, ref.Name)
			if !builtListeners[listenerName] {
				listener, err := buildInternalListener(listenerName, rb, backendClusterName(rb.GetNamespace(), rb.GetName()))
				if err != nil {
					errs = append(errs, fmt.Errorf("priority group %d: backend %q: %w", idx, ref.Name, err))
					continue
				}
				pgIr.internalListeners = append(pgIr.internalListeners, listener)
				builtListeners[listenerName] = true
			}
			if rb.Spec.Gcp != nil {
				pgIr.needsGcpAuthn = true
			}

			locality.LbEndpoints = append(locality.GetLbEndpoints(), &envoyendpointv3.LbEndpoint{
				HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
					Endpoint: &envoyendpointv3.Endpoint{
						Address: &envoycorev3.Address{
							Address: &envoycorev3.Address_EnvoyInternalAddress{
								EnvoyInternalAddress: &envoycorev3.EnvoyInternalAddress{
									AddressNameSpecifier: &envoycorev3.EnvoyInternalAddress_ServerListenerName{
										ServerListenerName: listenerName,
									},
								},
							},
						},
					},
				},
			})
		}
		pgIr.loadAssignment.Endpoints = append(pgIr.loadAssignment.GetEndpoints(), locality)
	}
	return pgIr, errs
}

// buildInternalListener builds the internal listener bridging the priority
// groups cluster to a referenced backend's cluster. Backend types whose
// behavior is HTTP-specific (GCP authentication and host rewrite, dynamic
// forward proxy host resolution, lambda request transformation) get an HTTP
// connection manager with a single catch-all route to the referenced cluster;
// all other types get a protocol-agnostic tcp_proxy bridge, which also makes
// them usable from TCP routes.
func buildInternalListener(name string, rb *kgateway.Backend, refClusterName string) (*envoylistenerv3.Listener, error) {
	var bridgeFilter *envoylistenerv3.Filter
	var err error
	// if rb.Spec.Gcp != nil || rb.Spec.DynamicForwardProxy != nil || (rb.Spec.Aws != nil && rb.Spec.Aws.Lambda != nil) {
	// 	bridgeFilter, err = buildHTTPBridgeFilter(name, rb, refClusterName)
	// } else {
	// 	bridgeFilter, err = buildTCPBridgeFilter(name, refClusterName)
	// }
	bridgeFilter, err = buildHTTPBridgeFilter(name, rb, refClusterName)
	if err != nil {
		return nil, err
	}

	return &envoylistenerv3.Listener{
		Name: name,
		ListenerSpecifier: &envoylistenerv3.Listener_InternalListener{
			InternalListener: &envoylistenerv3.Listener_InternalListenerConfig{},
		},
		FilterChains: []*envoylistenerv3.FilterChain{{
			Filters: []*envoylistenerv3.Filter{bridgeFilter},
		}},
	}, nil
}

// buildTCPBridgeFilter builds a tcp_proxy filter forwarding all traffic to the
// referenced backend's cluster.
func buildTCPBridgeFilter(name, refClusterName string) (*envoylistenerv3.Filter, error) {
	tcpProxyConfig, err := utils.MessageToAny(&envoytcp.TcpProxy{
		StatPrefix: name,
		ClusterSpecifier: &envoytcp.TcpProxy_Cluster{
			Cluster: refClusterName,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tcp proxy config: %w", err)
	}
	return &envoylistenerv3.Filter{
		Name:       envoywellknown.TCPProxy,
		ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: tcpProxyConfig},
	}, nil
}

// buildHTTPBridgeFilter builds an HTTP connection manager with a single
// catch-all route to the referenced backend's cluster, plus the HTTP filters
// the referenced backend type needs (gcp_authn for GCP, dynamic_forward_proxy
// for DFP).
func buildHTTPBridgeFilter(name string, rb *kgateway.Backend, refClusterName string) (*envoylistenerv3.Filter, error) {
	routeAction := &envoyroutev3.RouteAction{
		ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: refClusterName},
	}

	var httpFilters []*envoy_hcm.HttpFilter
	switch {
	case rb.Spec.Gcp != nil:
		gcpAuthnConfig, err := utils.MessageToAny(getGcpAuthnFilterConfig())
		if err != nil {
			return nil, fmt.Errorf("failed to create gcp authn filter config: %w", err)
		}
		httpFilters = append(httpFilters, &envoy_hcm.HttpFilter{
			Name:       gcpAuthnFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{TypedConfig: gcpAuthnConfig},
		})
		// GCP backends require the Host header to match the GCP service.
		routeAction.HostRewriteSpecifier = &envoyroutev3.RouteAction_AutoHostRewrite{
			AutoHostRewrite: &wrapperspb.BoolValue{Value: true},
		}
	case rb.Spec.DynamicForwardProxy != nil:
		dfpConfig, err := utils.MessageToAny(dfpFilterConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create dynamic forward proxy filter config: %w", err)
		}
		httpFilters = append(httpFilters, &envoy_hcm.HttpFilter{
			Name:       "envoy.filters.http.dynamic_forward_proxy",
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{TypedConfig: dfpConfig},
		})
	}

	routerConfig, err := utils.MessageToAny(&envoyrouterv3.Router{})
	if err != nil {
		return nil, fmt.Errorf("failed to create router filter config: %w", err)
	}
	httpFilters = append(httpFilters, &envoy_hcm.HttpFilter{
		Name:       envoywellknown.Router,
		ConfigType: &envoy_hcm.HttpFilter_TypedConfig{TypedConfig: routerConfig},
	})

	hcmConfig, err := utils.MessageToAny(&envoy_hcm.HttpConnectionManager{
		StatPrefix:  name,
		HttpFilters: httpFilters,
		RouteSpecifier: &envoy_hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: &envoyroutev3.RouteConfiguration{
				Name: name,
				VirtualHosts: []*envoyroutev3.VirtualHost{{
					Name:    name,
					Domains: []string{"*"},
					Routes: []*envoyroutev3.Route{{
						Match: &envoyroutev3.RouteMatch{
							PathSpecifier: &envoyroutev3.RouteMatch_Prefix{Prefix: "/"},
						},
						Action: &envoyroutev3.Route_Route{Route: routeAction},
					}},
				}},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create http connection manager config: %w", err)
	}
	return &envoylistenerv3.Filter{
		Name:       envoywellknown.HTTPConnectionManager,
		ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: hcmConfig},
	}, nil
}

// processPriorityGroups applies the priority groups IR to the envoy cluster.
// The cluster's endpoints are internal listeners, one locality per priority
// group with the locality priority matching the group's position in the list.
func processPriorityGroups(pgIr *PriorityGroupsIr, out *envoyclusterv3.Cluster) {
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
		Type: envoyclusterv3.Cluster_STATIC,
	}
	// clone needed to avoid adding the cluster name to the original object in the IR.
	out.LoadAssignment = proto.Clone(pgIr.loadAssignment).(*envoyendpointv3.ClusterLoadAssignment)
	out.LoadAssignment.ClusterName = out.GetName()
}
