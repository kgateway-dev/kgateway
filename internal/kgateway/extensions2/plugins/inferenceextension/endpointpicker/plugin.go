package endpointpicker

import (
	"context"
	"fmt"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	upstreamsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func NewPlugin(ctx context.Context, commonCol *common.CommonCollections) extplug.Plugin {
	poolGVR := schema.GroupVersionResource{
		Group:    infextv1a2.GroupVersion.Group,
		Version:  infextv1a2.GroupVersion.Version,
		Resource: "inferencepools",
	}

	poolCol := krtutil.SetupCollectionDynamic[infextv1a2.InferencePool](
		ctx,
		commonCol.Client,
		poolGVR,
		commonCol.KrtOpts.ToOptions("InferencePools")...,
	)

	return NewPluginFromCollections(ctx, commonCol, poolCol)
}

func NewPluginFromCollections(
	ctx context.Context,
	commonCol *common.CommonCollections,
	poolCol krt.Collection[*infextv1a2.InferencePool],
) extplug.Plugin {
	// The InferencePool group kind used by the BackendObjectIR and the ContributesBackendObjectIRs plugin.
	gk := schema.GroupKind{
		Group: infextv1a2.GroupVersion.Group,
		Kind:  wellknown.InferencePoolKind,
	}

	backendCol := krt.NewCollection(poolCol, func(kctx krt.HandlerContext, pool *infextv1a2.InferencePool) *ir.BackendObjectIR {
		// Create a BackendObjectIR IR representation from the given InferencePool.
		return &ir.BackendObjectIR{
			ObjectSource: ir.ObjectSource{
				Kind:      gk.Kind,
				Group:     gk.Group,
				Namespace: pool.Namespace,
				Name:      pool.Name,
			},
			Obj:               pool,
			Port:              pool.Spec.TargetPortNumber,
			GvPrefix:          "endpoint-picker",
			CanonicalHostname: "",
			ObjIr:             ir.NewInferencePool(pool),
		}
	}, commonCol.KrtOpts.ToOptions("InferencePoolIR")...)

	policyCol := krt.NewCollection(poolCol, func(krtctx krt.HandlerContext, pool *infextv1a2.InferencePool) *ir.PolicyWrapper {
		// Create a PolicyWrapper IR representation from the given InferencePool.
		return &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: pool.Namespace,
				Name:      pool.Name,
			},
			Policy:   pool,
			PolicyIR: ir.NewInferencePool(pool),
		}
	})

	// Return a plugin that contributes a policy and backend.
	return extplug.Plugin{
		ContributesBackends: map[schema.GroupKind]extplug.BackendPlugin{
			gk: {
				Backends: backendCol,
				BackendInit: ir.BackendInit{
					InitBackend: processBackendObjectIR,
				},
			},
		},
		ContributesPolicies: map[schema.GroupKind]extplug.PolicyPlugin{
			gk: {
				Name:                      "endpoint-picker",
				Policies:                  policyCol,
				NewGatewayTranslationPass: newEndpointPickerPass,
			},
		},
	}
}

// endpointPickerPass implements ir.ProxyTranslationPass. It collects any references
// to InferencePools, and returns the “ext_proc” cluster (STRICT_DNS), ext_proc filter,
// and overrides (per-route ext_proc cluster name, etc).
type endpointPickerPass struct {
	// Instead of a single pool, store multiple pools keyed by NamespacedName.
	usedPools map[types.NamespacedName]*ir.InferencePool
}

func newEndpointPickerPass(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &endpointPickerPass{
		usedPools: make(map[types.NamespacedName]*ir.InferencePool),
	}
}

func (p *endpointPickerPass) Name() string {
	return "endpoint-picker"
}

// No-op for these standard translation pass methods.
func (p *endpointPickerPass) ApplyListenerPlugin(ctx context.Context, lctx *ir.ListenerContext, out *listenerv3.Listener) {
}

func (p *endpointPickerPass) ApplyHCM(ctx context.Context, hctx *ir.HcmContext, out *hcmv3.HttpConnectionManager) error {
	return nil
}

func (p *endpointPickerPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

func (p *endpointPickerPass) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *endpointPickerPass) ApplyVhostPlugin(ctx context.Context, vctx *ir.VirtualHostContext, out *routev3.VirtualHost) {
}

func (p *endpointPickerPass) ApplyForRoute(ctx context.Context, rctx *ir.RouteContext, out *routev3.Route) error {
	return nil
}

func (p *endpointPickerPass) ApplyRouteConfigPlugin(
	ctx context.Context,
	pCtx *ir.RouteConfigContext,
	out *routev3.RouteConfiguration,
) {
}

func (p *endpointPickerPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	return nil
}

// ApplyForBackend updates the Envoy route for each InferencePool-backed HTTPRoute.
func (p *endpointPickerPass) ApplyForBackend(
	ctx context.Context,
	pCtx *ir.RouteBackendContext,
	in ir.HttpBackend,
	out *routev3.Route,
) error {
	if p == nil || pCtx == nil || pCtx.Backend == nil {
		return nil
	}

	// Ensure the backend object is an InferencePool.
	irPool, ok := pCtx.Backend.ObjIr.(*ir.InferencePool)
	if !ok || irPool == nil {
		return nil
	}

	// Store this pool in our map, keyed by NamespacedName.
	nn := types.NamespacedName{
		Namespace: irPool.ObjMeta.GetNamespace(),
		Name:      irPool.ObjMeta.GetName(),
	}
	p.usedPools[nn] = irPool

	// Ensure RouteAction is initialized.
	if out.GetRoute() == nil {
		out.Action = &routev3.Route_Route{
			Route: &routev3.RouteAction{},
		}
	}

	// Point the route to the ORIGINAL_DST cluster for this pool.
	out.GetRoute().ClusterSpecifier = &routev3.RouteAction_Cluster{
		Cluster: clusterNameOriginalDst(irPool.ObjMeta.GetName(), irPool.ObjMeta.GetNamespace()),
	}

	// Build the route-level ext_proc override that points to this pool's ext_proc cluster.
	override := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{
				GrpcService: &corev3.GrpcService{
					Timeout: durationpb.New(10 * time.Second),
					TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
							ClusterName: clusterNameExtProc(
								irPool.ObjMeta.GetName(),
								irPool.ObjMeta.GetNamespace(),
							),
							Authority: fmt.Sprintf("%s.%s.svc.cluster.local:%d",
								irPool.ConfigRef.Name,
								irPool.ObjMeta.GetNamespace(),
								irPool.ConfigRef.Ports[0].PortNum),
						},
					},
				},
			},
		},
	}

	// Attach per-route override to typed_per_filter_config.
	pCtx.AddTypedConfig(wellknown.InfPoolTransformationFilterName, override)

	return nil
}

// HttpFilters inserts one ext_proc filter per used InferencePool.
func (p *endpointPickerPass) HttpFilters(ctx context.Context, fc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	if p == nil || len(p.usedPools) == 0 {
		return nil, nil
	}

	var filters []plugins.StagedHttpFilter

	// For each used pool, create a distinct ext_proc filter referencing that pool’s cluster.
	for _, pool := range p.usedPools {
		if pool.ConfigRef == nil || len(pool.ConfigRef.Ports) == 0 {
			continue
		}

		clusterName := clusterNameExtProc(pool.ObjMeta.GetName(), pool.ObjMeta.GetNamespace())
		authority := fmt.Sprintf("%s.%s:%d",
			pool.ConfigRef.Name,
			pool.ObjMeta.GetNamespace(),
			pool.ConfigRef.Ports[0].PortNum,
		)

		// Use a unique filter name per pool to avoid collisions.
		filterName := fmt.Sprintf("%s_%s_%s",
			wellknown.InfPoolTransformationFilterName,
			pool.ObjMeta.GetNamespace(),
			pool.ObjMeta.GetName(),
		)

		extProcSettings := &extprocv3.ExternalProcessor{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
						ClusterName: clusterName,
						Authority:   authority,
					},
				},
			},
			ProcessingMode: &extprocv3.ProcessingMode{
				RequestHeaderMode:   extprocv3.ProcessingMode_SEND,
				RequestBodyMode:     extprocv3.ProcessingMode_BUFFERED,
				ResponseHeaderMode:  extprocv3.ProcessingMode_SKIP,
				RequestTrailerMode:  extprocv3.ProcessingMode_SKIP,
				ResponseTrailerMode: extprocv3.ProcessingMode_SKIP,
			},
			MessageTimeout:   durationpb.New(5 * time.Second),
			FailureModeAllow: false,
		}

		stagedFilter, err := plugins.NewStagedFilter(
			filterName, // must be unique
			extProcSettings,
			plugins.BeforeStage(plugins.RouteStage),
		)
		if err != nil {
			return nil, err
		}
		filters = append(filters, stagedFilter)
	}

	return filters, nil
}

// ResourcesToAdd returns the ext_proc clusters for all used InferencePools.
func (p *endpointPickerPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	if p == nil || len(p.usedPools) == 0 {
		return ir.Resources{}
	}

	var clusters []*clusterv3.Cluster
	for _, pool := range p.usedPools {
		c := buildExtProcCluster(pool)
		if c != nil {
			clusters = append(clusters, c)
		}
	}

	return ir.Resources{Clusters: clusters}
}

// processBackendObjectIR builds the ORIGINAL_DST cluster for each InferencePool.
func processBackendObjectIR(ctx context.Context, in ir.BackendObjectIR, out *clusterv3.Cluster) {
	out.ConnectTimeout = durationpb.New(1000 * time.Second)

	out.ClusterDiscoveryType = &clusterv3.Cluster_Type{
		Type: clusterv3.Cluster_ORIGINAL_DST,
	}

	out.LbPolicy = clusterv3.Cluster_CLUSTER_PROVIDED
	out.LbConfig = &clusterv3.Cluster_OriginalDstLbConfig_{
		OriginalDstLbConfig: &clusterv3.Cluster_OriginalDstLbConfig{
			UseHttpHeader:  true,
			HttpHeaderName: "x-gateway-destination-endpoint",
		},
	}

	out.CircuitBreakers = &clusterv3.CircuitBreakers{
		Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
			{
				MaxConnections:     wrapperspb.UInt32(40000),
				MaxPendingRequests: wrapperspb.UInt32(40000),
				MaxRequests:        wrapperspb.UInt32(40000),
			},
		},
	}

	out.Name = clusterNameOriginalDst(in.Name, in.Namespace)
}

// buildExtProcCluster builds and returns a “STRICT_DNS” cluster from the given pool.
func buildExtProcCluster(pool *ir.InferencePool) *clusterv3.Cluster {
	if pool == nil || pool.ConfigRef == nil || len(pool.ConfigRef.Ports) != 1 {
		return nil
	}

	name := clusterNameExtProc(pool.ObjMeta.GetName(), pool.ObjMeta.GetNamespace())
	c := &clusterv3.Cluster{
		Name:           name,
		ConnectTimeout: durationpb.New(10 * time.Second),
		ClusterDiscoveryType: &clusterv3.Cluster_Type{
			Type: clusterv3.Cluster_STRICT_DNS,
		},
		LbPolicy: clusterv3.Cluster_LEAST_REQUEST,
		LoadAssignment: &endpointv3.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpointv3.LocalityLbEndpoints{{
				LbEndpoints: []*endpointv3.LbEndpoint{{
					HealthStatus: corev3.HealthStatus_HEALTHY,
					HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
						Endpoint: &endpointv3.Endpoint{
							Address: &corev3.Address{
								Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:  fmt.Sprintf("%s.%s.svc.cluster.local", pool.ConfigRef.Name, pool.ObjMeta.Namespace),
										Protocol: corev3.SocketAddress_TCP,
										PortSpecifier: &corev3.SocketAddress_PortValue{
											PortValue: uint32(pool.ConfigRef.Ports[0].PortNum),
										},
									},
								},
							},
						},
					},
				}},
			}},
		},
		// Ensure Envoy accepts untrusted certificates.
		TransportSocket: &corev3.TransportSocket{
			Name: "envoy.transport_sockets.tls",
			ConfigType: &corev3.TransportSocket_TypedConfig{
				TypedConfig: func() *anypb.Any {
					tlsCtx := &tlsv3.UpstreamTlsContext{
						CommonTlsContext: &tlsv3.CommonTlsContext{
							ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{},
						},
					}
					anyTLS, _ := utils.MessageToAny(tlsCtx)
					return anyTLS
				}(),
			},
		},
	}

	http2Opts := &upstreamsv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &corev3.Http2ProtocolOptions{},
				},
			},
		},
	}

	anyHttp2, _ := utils.MessageToAny(http2Opts)
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": anyHttp2,
	}

	return c
}

func clusterNameExtProc(name, ns string) string {
	return fmt.Sprintf("endpointpicker_%s_%s_ext_proc", name, ns)
}

func clusterNameOriginalDst(name, ns string) string {
	return fmt.Sprintf("endpointpicker_%s_%s_original_dst", name, ns)
}
