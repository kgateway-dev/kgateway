package endpointpicker

import (
	"context"
	"fmt"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	headertometadata "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_to_metadata/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	upstreamsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	infv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

const (
	poolGroupKindName = "endpoint-picker"
	// Derived from upstream Gateway API Inference Extension defaults (testdata/envoy.yaml).
	defaultExtProcMaxRequests = 40000
	// envoyLbNamespace is the Envoy predefined namespace for load balancing metadata.
	envoyLbNamespace = "envoy.lb"
	// envoySubsetHint defines the outer key of the subset list metadata entry for Envoy
	// subset load balancing.
	envoySubsetKey = envoyLbNamespace + ".subset_hint"
	// dstEndpointKey defines the header and filter metadata key used to communicate the
	// selected endpoints.
	dstEndpointKey = "x-gateway-destination-endpoint"
	// subsetDstEndpointKey defines the filter metadata key used to communicate the list of subset
	// endpoints that the EPP selects from.
	subsetDstEndpointKey = dstEndpointKey + "-subset"
)

var (
	logger = logging.New("plugin/inference-epp")
)

func NewPlugin(ctx context.Context, commonCols *common.CommonCollections) *extplug.Plugin {
	p := initInferencePoolCollections(ctx, commonCols)

	// Wrap the init function so it can capture commonCols.Pods
	initBackend := func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
		return processPoolBackendObjIR(ctx, in, out, p.podIndex)
	}

	return &extplug.Plugin{
		ContributesBackends: map[schema.GroupKind]extplug.BackendPlugin{
			wellknown.InferencePoolGVK.GroupKind(): {
				BackendInit: ir.BackendInit{InitEnvoyBackend: initBackend},
				Backends:    p.backendsDP,
				Endpoints:   p.endpoints,
			},
		},
		ContributesPolicies: map[schema.GroupKind]extplug.PolicyPlugin{
			wellknown.InferencePoolGVK.GroupKind(): {
				Name:     poolGroupKindName,
				Policies: p.policies,
				NewGatewayTranslationPass: func(ctx context.Context, t ir.GwTranslationCtx, r reports.Reporter) ir.ProxyTranslationPass {
					return newEndpointPickerPass(r, p.podIndex)
				},
			},
		},
		ContributesRegistration: map[schema.GroupKind]func(){
			wellknown.InferencePoolGVK.GroupKind(): buildRegisterCallback(
				ctx,
				commonCols,
				p.backendsCtl,
				p.poolIndex,
				commonCols.LocalityPods,
			),
		},
	}
}

// buildPolicyWrapperCollection returns a krt.Collection[ir.PolicyWrapper]
// whose source is the supplied backends collection.
func buildPolicyWrapperCollection(
	commonCol *common.CommonCollections,
	backends krt.Collection[ir.BackendObjectIR],
) krt.Collection[ir.PolicyWrapper] {
	return krt.NewCollection(
		backends,
		func(_ krt.HandlerContext, be ir.BackendObjectIR) *ir.PolicyWrapper {
			irPool, ok := be.ObjIr.(*inferencePool)
			if !ok {
				return nil
			}

			return &ir.PolicyWrapper{
				ObjectSource: be.ObjectSource,
				Policy:       be.Obj.(*infv1a2.InferencePool),
				PolicyIR:     irPool,
			}
		},
		commonCol.KrtOpts.ToOptions("InferencePoolPolicies")...,
	)
}

func buildBackendObjIrFromPool(pool *inferencePool) *ir.BackendObjectIR {
	// Create a BackendObjectIR IR representation from the given InferencePool.
	objSrc := ir.ObjectSource{
		Kind:      wellknown.InferencePoolGVK.Kind,
		Group:     wellknown.InferencePoolGVK.Group,
		Namespace: pool.obj.GetNamespace(),
		Name:      pool.obj.GetName(),
	}
	backend := ir.NewBackendObjectIR(objSrc, pool.targetPort, "")
	backend.GvPrefix = poolGroupKindName
	backend.Obj = pool.obj
	backend.ObjIr = pool
	// TODO [danehans]: Look into using backend.AppProtocol to set H1/H2 for the static cluster.
	backend.CanonicalHostname = kubeutils.GetServiceHostname(objSrc.Name, objSrc.Namespace)
	return &backend
}

// endpointPickerPass implements ir.ProxyTranslationPass. It collects any references to IR inferencePools.
type endpointPickerPass struct {
	podIdx krt.Index[string, krtcollections.LocalityPod]
	// usedPools defines a map of IR inferencePools keyed by NamespacedName.
	usedPools map[types.NamespacedName]*inferencePool
	ir.UnimplementedProxyTranslationPass

	reporter reports.Reporter
}

var _ ir.ProxyTranslationPass = &endpointPickerPass{}

func newEndpointPickerPass(
	reporter reports.Reporter,
	podIdx krt.Index[string, krtcollections.LocalityPod],
) ir.ProxyTranslationPass {
	return &endpointPickerPass{
		usedPools: make(map[types.NamespacedName]*inferencePool),
		reporter:  reporter,
		podIdx:    podIdx,
	}
}

func (p *endpointPickerPass) Name() string {
	return poolGroupKindName
}

// ApplyForBackend updates the Envoy route for each InferencePool-backed HTTPRoute.
func (p *endpointPickerPass) ApplyForBackend(
	ctx context.Context,
	pCtx *ir.RouteBackendContext,
	in ir.HttpBackend,
	out *envoyroutev3.Route,
) error {
	if p == nil || pCtx == nil || pCtx.Backend == nil {
		return nil
	}

	// Ensure the backend object is an InferencePool.
	irPool, ok := pCtx.Backend.ObjIr.(*inferencePool)
	if !ok || irPool == nil {
		return nil
	}

	// Store this pool in our map, keyed by NamespacedName.
	nn := types.NamespacedName{
		Namespace: irPool.obj.GetNamespace(),
		Name:      irPool.obj.GetName(),
	}
	p.usedPools[nn] = irPool

	// Ensure RouteAction is initialized.
	if out.GetRoute() == nil {
		out.Action = &envoyroutev3.Route_Route{
			Route: &envoyroutev3.RouteAction{},
		}
	}

	// Get the RouteAction.
	ra := out.GetRoute()

	// Ensure the route's cluster points at the backend’s cluster name.
	ra.ClusterSpecifier = &envoyroutev3.RouteAction_Cluster{
		Cluster: pCtx.Backend.ClusterName(),
	}

	// Initialize the filter metadata.
	if ra.GetMetadataMatch() == nil {
		ra.MetadataMatch = &envoycorev3.Metadata{}
	}
	if ra.MetadataMatch.FilterMetadata == nil {
		ra.MetadataMatch.FilterMetadata = make(map[string]*structpb.Struct)
	}

	// Ensure we are working with the latest set of endpoints for the pool.
	eps := irPool.resolvePoolEndpoints(p.podIdx)
	if len(eps) == 0 {
		return fmt.Errorf("no endpoints found for InferencePool %s/%s",
			irPool.obj.GetNamespace(),
			irPool.obj.GetName())
	}
	irPool.endpoints = eps

	// Tell the EPP the subset of endpoints to choose from.
	vs := make([]*structpb.Value, 0, len(eps))
	for _, ep := range eps {
		vs = append(vs, structpb.NewStringValue(ep.string()))
	}
	hintStruct := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			subsetDstEndpointKey: {
				Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: vs}},
			},
		},
	}

	// Set the subset hint (sent to EPP as filter_metadata).
	if ra.MetadataMatch == nil {
		ra.MetadataMatch = &envoycorev3.Metadata{
			FilterMetadata: make(map[string]*structpb.Struct),
		}
	}
	ra.MetadataMatch.FilterMetadata[envoySubsetKey] = hintStruct

	// Build the route-level ext_proc override that points to this pool's ext_proc cluster.
	override := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{
				FailureModeAllow: wrapperspb.Bool(irPool.failOpen),
				GrpcService: &envoycorev3.GrpcService{
					Timeout: durationpb.New(10 * time.Second),
					TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
							ClusterName: clusterNameExtProc(
								irPool.obj.GetName(),
								irPool.obj.GetNamespace(),
							),
							Authority: authorityForPool(irPool),
						},
					},
				},
			},
		},
	}

	// Attach per-route override to typed_per_filter_config.
	pCtx.TypedFilterConfig.AddTypedConfig(wellknown.InfPoolTransformationFilterName, override)

	return nil
}

// HttpFilters returns one ext_proc filter, using the well-known filter name.
func (p *endpointPickerPass) HttpFilters(ctx context.Context, fc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	if p == nil || len(p.usedPools) == 0 {
		return nil, nil
	}

	// Create a pool as placeholder for the static config
	tmpPool := &inferencePool{
		obj: &infv1a2.InferencePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "placeholder-pool",
				Namespace: "placeholder-namespace",
			},
		},
		configRef: &service{
			ObjectSource: ir.ObjectSource{Name: "placeholder-service"},
			ports:        []servicePort{{name: "grpc", portNum: 9002}},
		},
	}

	// Static ExternalProcessor that will be overridden by ExtProcPerRoute
	extProcSettings := &extprocv3.ExternalProcessor{
		GrpcService: &envoycorev3.GrpcService{
			TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
					ClusterName: clusterNameExtProc(
						tmpPool.obj.GetName(),
						tmpPool.obj.GetNamespace(),
					),
					Authority: authorityForPool(tmpPool),
				},
			},
		},
		ProcessingMode: &extprocv3.ProcessingMode{
			RequestHeaderMode:   extprocv3.ProcessingMode_SEND,
			RequestBodyMode:     extprocv3.ProcessingMode_FULL_DUPLEX_STREAMED,
			RequestTrailerMode:  extprocv3.ProcessingMode_SEND,
			ResponseBodyMode:    extprocv3.ProcessingMode_FULL_DUPLEX_STREAMED,
			ResponseHeaderMode:  extprocv3.ProcessingMode_SEND,
			ResponseTrailerMode: extprocv3.ProcessingMode_SEND,
		},
		MessageTimeout:   durationpb.New(5 * time.Second),
		FailureModeAllow: false,
		MetadataOptions: &extprocv3.MetadataOptions{
			ForwardingNamespaces: &extprocv3.MetadataOptions_MetadataNamespaces{
				Untyped: []string{envoySubsetKey},
			},
			ReceivingNamespaces: &extprocv3.MetadataOptions_MetadataNamespaces{
				Untyped: []string{envoyLbNamespace},
			},
		},
	}

	extProcFilter, err := plugins.NewStagedFilter(
		wellknown.InfPoolTransformationFilterName,
		extProcSettings,
		plugins.BeforeStage(plugins.AuthNStage),
	)
	if err != nil {
		return nil, err
	}

	htm := &headertometadata.Config{
		RequestRules: []*headertometadata.Config_Rule{{
			Header: dstEndpointKey,
			OnHeaderPresent: &headertometadata.Config_KeyValuePair{
				MetadataNamespace: envoyLbNamespace,
				Key:               dstEndpointKey,
				Type:              headertometadata.Config_STRING,
			},
			Remove: false,
		}},
	}
	htmFilter, _ := plugins.NewStagedFilter(
		"envoy.filters.http.header_to_metadata",
		htm,
		plugins.BeforeStage(plugins.RouteStage),
	)

	return []plugins.StagedHttpFilter{extProcFilter, htmFilter}, nil
}

// ResourcesToAdd returns the ext_proc clusters for all used InferencePools.
func (p *endpointPickerPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	if p == nil || len(p.usedPools) == 0 {
		return ir.Resources{}
	}
	var clusters []*envoyclusterv3.Cluster
	for _, pool := range p.usedPools {
		if c := buildExtProcCluster(pool); c != nil {
			clusters = append(clusters, c)
		}
	}
	return ir.Resources{Clusters: clusters}
}

// buildExtProcCluster builds and returns a "STRICT_DNS" cluster from the given pool.
func buildExtProcCluster(pool *inferencePool) *envoyclusterv3.Cluster {
	if pool == nil || pool.configRef == nil || len(pool.configRef.ports) != 1 {
		return nil
	}

	name := clusterNameExtProc(pool.obj.GetName(), pool.obj.GetNamespace())
	c := &envoyclusterv3.Cluster{
		Name:           name,
		ConnectTimeout: durationpb.New(10 * time.Second),
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
			Type: envoyclusterv3.Cluster_STRICT_DNS,
		},
		LbPolicy: envoyclusterv3.Cluster_LEAST_REQUEST,
		LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{
				LbEndpoints: []*envoyendpointv3.LbEndpoint{{
					HealthStatus: envoycorev3.HealthStatus_HEALTHY,
					HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
						Endpoint: &envoyendpointv3.Endpoint{
							Address: &envoycorev3.Address{
								Address: &envoycorev3.Address_SocketAddress{
									SocketAddress: &envoycorev3.SocketAddress{
										Address:  fmt.Sprintf("%s.%s.svc", pool.configRef.Name, pool.obj.GetNamespace()),
										Protocol: envoycorev3.SocketAddress_TCP,
										PortSpecifier: &envoycorev3.SocketAddress_PortValue{
											PortValue: uint32(pool.configRef.ports[0].portNum),
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
		TransportSocket: &envoycorev3.TransportSocket{
			Name: "envoy.transport_sockets.tls",
			ConfigType: &envoycorev3.TransportSocket_TypedConfig{
				TypedConfig: func() *anypb.Any {
					tlsCtx := &envoytlsv3.UpstreamTlsContext{
						CommonTlsContext: &envoytlsv3.CommonTlsContext{
							ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{},
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
					Http2ProtocolOptions: &envoycorev3.Http2ProtocolOptions{},
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

// authorityForPool formats the gRPC authority based on the given InferencePool IR.
func authorityForPool(pool *inferencePool) string {
	ns := pool.obj.GetNamespace()
	svc := pool.configRef.Name
	port := pool.configRef.ports[0].portNum
	return fmt.Sprintf("%s.%s.svc:%d", svc, ns, port)
}
