package endpointpicker

import (
	"context"
	"fmt"
	"strings"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ext_procv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	infextv1a1 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	poolClient := kclient.New[*infextv1a1.InferencePool](commoncol.Client)
	pools := krt.WrapClient(poolClient, commoncol.KrtOpts.ToOptions("InferencePools")...)
	return NewPluginFromCollections(ctx, commoncol.KrtOpts, pools, commoncol.Pods, commoncol.Settings)
}

func NewPluginFromCollections(
	ctx context.Context,
	krtOpts krtutil.KrtOptions,
	pools krt.Collection[*infextv1a1.InferencePool],
	pods krt.Collection[krtcollections.LocalityPod],
	stngs settings.Settings,
) extensionsplug.Plugin {
	gk := schema.GroupKind{
		Group: infextv1a1.GroupVersion.Group,
		Kind:  wellknown.InferencePoolKind,
	}

	// TODO [danehans]: Filter InferencePools based one's that are referenced by an HTTPRoute
	// with a status.parents[].controllerName that matches our Gateway controllerName.
	infPoolUpstream := krt.NewCollection(pools, func(kctx krt.HandlerContext, pool *infextv1a1.InferencePool) *ir.Upstream {
		return &ir.Upstream{
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
		}
	}, krtOpts.ToOptions("EndpointPickerUpstreams")...)

	// Create the endpoints collection
	inputs := krtcollections.NewInfPoolEndpointsInputs(krtOpts, infPoolUpstream, pods)
	infPoolEndpoints := krtcollections.NewInfPoolEndpoints(ctx, inputs)

	return extensionsplug.Plugin{
		ContributesUpstreams: map[schema.GroupKind]extensionsplug.UpstreamPlugin{
			gk: {
				UpstreamInit: ir.UpstreamInit{
					InitUpstream: processUpstream,
				},
				Endpoints: infPoolEndpoints,
				Upstreams: infPoolUpstream,
			},
		},
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			gk: {
				Name: "endpointpicker-extproc",
				NewGatewayTranslationPass: func(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
					return newExtProcProxyPass(infPoolUpstream, infPoolEndpoints)
				},
			},
		},
	}
}

func processUpstream(ctx context.Context, in ir.Upstream, out *clusterv3.Cluster) {
	// Set cluster type to ORIGINAL_DST
	out.ClusterDiscoveryType = &clusterv3.Cluster_Type{
		Type: clusterv3.Cluster_ORIGINAL_DST,
	}

	// Set connect timeout to 1000 seconds.
	// TODO [danehans]: Figure out an API that can be used to set this value.
	out.ConnectTimeout = durationpb.New(1000 * time.Second)

	// Use CLUSTER_PROVIDED load balancing.
	out.LbPolicy = clusterv3.Cluster_CLUSTER_PROVIDED

	// Configure circuit breakers with a single threshold.
	// TODO [danehans]: Figure out an API that can be used to set these values.
	out.CircuitBreakers = &clusterv3.CircuitBreakers{
		Thresholds: []*clusterv3.CircuitBreakers_Thresholds{
			{
				MaxConnections:     wrapperspb.UInt32(40000),
				MaxPendingRequests: wrapperspb.UInt32(40000),
				MaxRequests:        wrapperspb.UInt32(40000),
			},
		},
	}

	// If OriginalDstLbConfig is not available on Cluster,
	// encode the configuration as a typed extension.
	// Note: The type URL will be "type.googleapis.com/envoy.config.cluster.v3.Cluster_OriginalDstLbConfig".
	lbConfig := &clusterv3.Cluster_OriginalDstLbConfig{
		UseHttpHeader:  true,
		HttpHeaderName: "x-gateway-destination-endpoint",
	}
	anyLbConfig, err := anypb.New(lbConfig)
	if err != nil {
		// handle error appropriately
		return
	}
	out.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		"envoy.lb": anyLbConfig,
	}
}

// extProcProxyPass implements ir.ProxyTranslationPass without modifying the existing Plugin struct.
type extProcProxyPass struct {
	// extProcClusters maps an InferencePool, keyed by namespace/name, to cluster name to create.
	// TODO [danehans]: Use typesNamespacedName for key.
	extProcClusters  map[string]string
	infPoolUpstream  krt.Collection[ir.Upstream]
	infPoolEndpoints krt.Collection[ir.EndpointsForUpstream]
}

// newExtProcProxyPass initializes a new instance.
func newExtProcProxyPass(
	infPoolUpstream krt.Collection[ir.Upstream],
	infPoolEndpoints krt.Collection[ir.EndpointsForUpstream],
) ir.ProxyTranslationPass {
	return &extProcProxyPass{
		extProcClusters:  make(map[string]string),
		infPoolUpstream:  infPoolUpstream,
		infPoolEndpoints: infPoolEndpoints,
	}
}

// Name identifies this pass.
func (e *extProcProxyPass) Name() string {
	return "endpointpicker-extproc"
}

// ApplyListenerPlugin is invoked once for each Envoy listener. No-op for ext_proc.
func (e *extProcProxyPass) ApplyListenerPlugin(
	ctx context.Context,
	pCtx *ir.ListenerContext,
	out *listenerv3.Listener,
) {
	// no-op
}

// ApplyHCM is invoked once for each HttpConnectionManager config. No-op for ext_proc.
func (e *extProcProxyPass) ApplyHCM(
	ctx context.Context,
	pCtx *ir.HcmContext,
	out *hcmv3.HttpConnectionManager,
) error {
	return nil
}

// ApplyVhostPlugin is invoked for each virtual host. No-op for ext_proc.
func (e *extProcProxyPass) ApplyVhostPlugin(
	ctx context.Context,
	pCtx *ir.VirtualHostContext,
	out *routev3.VirtualHost,
) {
}

// ApplyForRoute is invoked once for each route. No-op for ext_proc here.
func (e *extProcProxyPass) ApplyForRoute(
	ctx context.Context,
	pCtx *ir.RouteContext,
	outputRoute *routev3.Route,
) error {
	return nil
}

// ApplyForRouteBackend is invoked for each backend on each route by detecting
// if the backend references an InferencePool and store ext_proc cluster info.
func (e *extProcProxyPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	// Check if the backend is InferencePool
	if pCtx.Upstream.Kind != wellknown.InferencePoolKind &&
		pCtx.Upstream.Group != infextv1a1.GroupVersion.Group {
		return fmt.Errorf("unsupported group or kind: must be group %s and kind %s",
			infextv1a1.GroupVersion.Group, wellknown.InferencePoolKind)
	}

	// Cast the object to an InferencePool
	pool, ok := pCtx.Upstream.Obj.(*infextv1a1.InferencePool)
	if !ok || pool == nil {
		return fmt.Errorf("inference pool %s/%s not found", pCtx.Upstream.Namespace, pCtx.Upstream.Name)
	}

	// Validate the InferencePool extension reference
	ref := pool.Spec.ExtensionRef
	if ref == nil {
		return fmt.Errorf("inference pool %s/%s missing extensionRef", pool.Namespace, pool.Name)
	}
	if (ref.Kind != nil && *ref.Kind != wellknown.ServiceKind) || (ref.Group != nil && *ref.Group != "") {
		return fmt.Errorf(
			"invalid extensionRef for inference pool %s/%s", pool.Namespace, pool.Name)
	}

	// Build a unique name for the ext_proc cluster, e.g. ext-proc-<namespace>-<poolName>.
	clusterName := fmt.Sprintf("ext-proc-%s-%s", pool.Namespace, pool.Name)
	e.extProcClusters[pool.Namespace+"/"+pool.Name] = clusterName

	// Optionally, set typed_per_filter_config on this route or return that config
	// so the ext_proc filter references the cluster.
	override := &ext_procv3.ExtProcPerRoute{
		Override: &ext_procv3.ExtProcPerRoute_Overrides{
			Overrides: &ext_procv3.ExtProcOverrides{
				GrpcService: &corev3.GrpcService{
					TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
							ClusterName: clusterName,
						},
					},
				},
			},
		},
	}
	anyOverride, err := anypb.New(override)
	if err != nil {
		return fmt.Errorf("failed to marshal ext_proc per-route override: %w", err)
	}

	// Attach the typed_per_filter_config to the route backend context.
	pCtx.AddTypedConfig("envoy.filters.http.ext_proc", anyOverride)

	return nil
}

// HttpFilters is called once per filter chain. If extProcNeeded, we add the ext_proc filter.
func (e *extProcProxyPass) HttpFilters(
	ctx context.Context,
	fc ir.FilterChainCommon,
) ([]plugins.StagedHttpFilter, error) {
	// Build the ExternalProcessor config (without a default gRPC service, since it's set per route).
	extProc := &ext_procv3.ExternalProcessor{
		// TODO [danehans]: Failure mode should be set based on InferencePool extensionRef failureMode.
		FailureModeAllow: false,
		ProcessingMode: &ext_procv3.ProcessingMode{
			RequestHeaderMode:  ext_procv3.ProcessingMode_SEND,
			ResponseHeaderMode: ext_procv3.ProcessingMode_SKIP,
			RequestBodyMode:    ext_procv3.ProcessingMode_BUFFERED,
			ResponseBodyMode:   ext_procv3.ProcessingMode_NONE,
		},
	}
	anyExtProc, err := anypb.New(extProc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ext_proc filter config: %w", err)
	}

	// Assign the ext_proc filter to the pre-routing stage.
	stagedFilter, err := plugins.NewStagedFilter(
		"envoy.filters.http.ext_proc",
		anyExtProc,
		plugins.BeforeStage(plugins.RouteStage),
	)
	if err != nil {
		return nil, err
	}
	return []plugins.StagedHttpFilter{stagedFilter}, nil
}

// UpstreamHttpFilters: no upstream-level filters needed for ext_proc, so return nil.
func (e *extProcProxyPass) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

// NetworkFilters: no network-level filters for ext_proc, so return nil.
func (e *extProcProxyPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// ResourcesToAdd is called once to let this pass add new Envoy resources, e.g. clusters.
func (e *extProcProxyPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	var result ir.Resources

	for key, clusterName := range e.extProcClusters {
		// key is "namespace/name"
		nsName := strings.SplitN(key, "/", 2)
		if len(nsName) != 2 {
			continue
		}
		ns := nsName[0]
		name := nsName[1]
		var kctx krt.HandlerContext
		// Retrieve the InferencePool from infPoolUpstream.
		key := types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}
		upstream := krt.FetchOne(kctx, e.infPoolUpstream, krt.FilterObjectName(key))
		if upstream == nil {
			continue
		}
		pool, ok := upstream.Obj.(*infextv1a1.InferencePool)
		if !ok {
			continue
		}
		ref := pool.Spec.ExtensionRef
		if ref == nil {
			// Shouldn't happen if we validated above
			continue
		}
		port := int32(9002)
		if ref.TargetPortNumber != nil && *ref.TargetPortNumber > 0 {
			port = *ref.TargetPortNumber
		}

		// Build the ext_proc cluster
		svcHost := fmt.Sprintf("%s.%s.svc.cluster.local", ref.Name, ns)
		extProcCluster := &clusterv3.Cluster{
			Name:                 clusterName,
			ConnectTimeout:       durationpb.New(24 * time.Hour), // 86400s
			ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STRICT_DNS},
			LbPolicy:             clusterv3.Cluster_LEAST_REQUEST,
			CircuitBreakers: &clusterv3.CircuitBreakers{
				Thresholds: []*clusterv3.CircuitBreakers_Thresholds{{
					MaxConnections:     wrapperspb.UInt32(40000),
					MaxPendingRequests: wrapperspb.UInt32(40000),
					MaxRequests:        wrapperspb.UInt32(40000),
					MaxRetries:         wrapperspb.UInt32(1024),
				}},
			},
			LoadAssignment: &endpointv3.ClusterLoadAssignment{
				ClusterName: clusterName,
				Endpoints: []*endpointv3.LocalityLbEndpoints{{
					Locality: &corev3.Locality{
						// TODO [danehans]: Get this value from ir.PodLocality of extension service pods?
						Region: "ext_proc",
					},
					LbEndpoints: []*endpointv3.LbEndpoint{{
						HealthStatus:        corev3.HealthStatus_HEALTHY,
						LoadBalancingWeight: &wrapperspb.UInt32Value{Value: 1},
						HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
							Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{
									Address: &corev3.Address_SocketAddress{
										SocketAddress: &corev3.SocketAddress{
											Address: svcHost,
											PortSpecifier: &corev3.SocketAddress_PortValue{
												PortValue: uint32(port),
											},
											Protocol: corev3.SocketAddress_TCP,
										},
									},
								},
							},
						},
					}},
				}},
			},
			// Accept untrusted certs by leaving the validation context empty
			TransportSocket: &corev3.TransportSocket{
				Name: "envoy.transport_sockets.tls",
				ConfigType: &corev3.TransportSocket_TypedConfig{
					TypedConfig: func() *anypb.Any {
						tlsCtx := &tlsv3.UpstreamTlsContext{
							CommonTlsContext: &tlsv3.CommonTlsContext{
								ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{},
							},
						}
						anyTLS, _ := anypb.New(tlsCtx)
						return anyTLS
					}(),
				},
			},
		}
		// Enable HTTP/2. We attach typed extension protocol options for http2, with big window sizes
		http2Opts := &httpv3.HttpProtocolOptions{
			UpstreamProtocolOptions: &httpv3.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &httpv3.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &httpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
						Http2ProtocolOptions: &corev3.Http2ProtocolOptions{
							MaxConcurrentStreams:        wrapperspb.UInt32(100),
							InitialStreamWindowSize:     wrapperspb.UInt32(65536),
							InitialConnectionWindowSize: wrapperspb.UInt32(1048576),
						},
					},
				},
			},
		}
		anyHTTP2, _ := anypb.New(http2Opts)
		extProcCluster.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": anyHTTP2,
		}

		// Add the cluster to our resources
		result.Clusters = append(result.Clusters, extProcCluster)
	}

	return result
}
