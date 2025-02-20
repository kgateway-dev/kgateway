package upstream

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"strconv"
	"strings"
	"time"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"github.com/rotisserie/eris"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	awspb "github.com/solo-io/envoy-gloo/go/config/filter/http/aws_lambda/v2"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"

	// Check
	upstream_wait "github.com/solo-io/envoy-gloo/go/config/filter/http/upstream_wait/v2"
)

const (
	ParameterGroup = "kgateway.io"
	ParameterKind  = "Parameter"

	FilterName = "io.solo.aws_lambda"

	// TODO: clean up
	extProcUDSClusterName = "ai_ext_proc_uds_cluster"
	extProcUDSSocketPath  = "@kgateway-ai-sock"
	waitFilterName        = "io.kgateway.wait"
)

var (
	ParameterGK = schema.GroupKind{
		Group: ParameterGroup,
		Kind:  ParameterKind,
	}
)

type upstreamDestination struct {
	FunctionName string
}

func (d *upstreamDestination) CreationTime() time.Time {
	return time.Time{}
}

func (d *upstreamDestination) Equals(in any) bool {
	d2, ok := in.(*upstreamDestination)
	if !ok {
		return false
	}
	return d.FunctionName == d2.FunctionName
}

type UpstreamIr struct {
	AwsSecret     *ir.Secret
	AISecret      *ir.Secret
	AIMultiSecret map[string]*ir.Secret
}

func (u *UpstreamIr) data() map[string][]byte {
	if u.AwsSecret == nil {
		return nil
	}
	return u.AwsSecret.Data
}

func (u *UpstreamIr) Equals(other any) bool {
	otherUpstream, ok := other.(*UpstreamIr)
	if !ok {
		return false
	}
	return maps.EqualFunc(u.data(), otherUpstream.data(), func(a, b []byte) bool {
		return bytes.Equal(a, b)
	})
}

type upstreamPlugin struct {
	needFilter       map[string]bool
	aiGatewayEnabled map[string]bool
}

func registerTypes(ourCli versioned.Interface) {
	skubeclient.Register[*v1alpha1.Upstream](
		v1alpha1.UpstreamGVK.GroupVersion().WithResource("upstreams"),
		v1alpha1.UpstreamGVK,
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().Upstreams(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().Upstreams(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes(commoncol.OurClient)

	col := krt.WrapClient(kclient.New[*v1alpha1.Upstream](commoncol.Client), commoncol.KrtOpts.ToOptions("Upstreams")...)

	gk := v1alpha1.UpstreamGVK.GroupKind()
	translate := buildTranslateFunc(ctx, commoncol.Secrets)
	ucol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *ir.Upstream {
		// resolve secrets
		return &ir.Upstream{
			ObjectSource: ir.ObjectSource{
				Kind:      gk.Kind,
				Group:     gk.Group,
				Namespace: i.GetNamespace(),
				Name:      i.GetName(),
			},
			GvPrefix:          "upstream",
			CanonicalHostname: hostname(i),
			Obj:               i,
			ObjIr:             translate(krtctx, i),
		}
	})

	endpoints := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *ir.EndpointsForUpstream {
		return processEndpoints(i)
	})
	return extensionsplug.Plugin{
		ContributesUpstreams: map[schema.GroupKind]extensionsplug.UpstreamPlugin{
			gk: {
				UpstreamInit: ir.UpstreamInit{
					InitUpstream: processUpstream,
				},
				Endpoints: endpoints,
				Upstreams: ucol,
			},
		},
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			// TODO: remove Parameters?
			ParameterGK: {
				Name:                      "upstream",
				NewGatewayTranslationPass: newPlug,
				//			AttachmentPoints: []ir.AttachmentPoints{ir.HttpBackendRefAttachmentPoint},
				PoliciesFetch: func(n, ns string) ir.PolicyIR {
					// virtual policy - we don't have a real policy object
					return &upstreamDestination{
						FunctionName: n,
					}
				},
			},
			v1alpha1.UpstreamGVK.GroupKind(): {
				Name:                      "upstream",
				NewGatewayTranslationPass: newPlug,
				//			AttachmentPoints: []ir.AttachmentPoints{ir.HttpBackendRefAttachmentPoint},
				PoliciesFetch: func(n, ns string) ir.PolicyIR {
					// virtual policy - we don't have a real policy object
					return &upstreamDestination{
						FunctionName: n,
					}
				},
			},
		},
	}
}

func buildTranslateFunc(ctx context.Context, secrets *krtcollections.SecretIndex) func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *UpstreamIr {
	return func(krtctx krt.HandlerContext, i *v1alpha1.Upstream) *UpstreamIr {
		// resolve secrets
		var upstreamIr UpstreamIr
		if i.Spec.Aws != nil {
			ns := i.GetNamespace()
			secret, err := getSecretIr(secrets, krtctx, i.Spec.Aws.SecretRef.Name, ns)
			if err != nil {
				contextutils.LoggerFrom(ctx).Error(err)
			}
			upstreamIr.AwsSecret = secret
		}
		if i.Spec.AI != nil {
			ns := i.GetNamespace()
			if i.Spec.AI.LLM != nil {
				secretRef := getAISecretRef(i.Spec.AI.LLM)
				// if secretRef is used, set the secret on the upstream ir
				if secretRef != nil {
					secret, err := getSecretIr(secrets, krtctx, secretRef.Name, ns)
					if err != nil {
						contextutils.LoggerFrom(ctx).Error(err)
					}
					upstreamIr.AISecret = secret
				}
			} else if i.Spec.AI.MultiPool != nil {
				upstreamIr.AIMultiSecret = map[string]*ir.Secret{}
				for idx, priority := range i.Spec.AI.MultiPool.Priorities {
					for jdx, pool := range priority.Pool {
						secretRef := getAISecretRef(&pool)
						// if secretRef is used, set the secret on the upstream ir
						if secretRef != nil {
							secret, err := getSecretIr(secrets, krtctx, secretRef.Name, ns)
							if err != nil {
								contextutils.LoggerFrom(ctx).Error(err)
							}
							upstreamIr.AIMultiSecret[getMultiPoolSecretKey(idx, jdx, secretRef.Name)] = secret
						}
					}
				}
			}

		}
		return &upstreamIr
	}
}

func getAISecretRef(llm *v1alpha1.LLMProviders) *corev1.LocalObjectReference {
	var secretRef *corev1.LocalObjectReference
	if llm.OpenAI != nil {
		secretRef = llm.OpenAI.AuthToken.SecretRef
	} else if llm.Anthropic != nil {
		secretRef = llm.Anthropic.AuthToken.SecretRef
	} else if llm.AzureOpenAI != nil {
		secretRef = llm.AzureOpenAI.AuthToken.SecretRef
	} else if llm.Gemini != nil {
		secretRef = llm.Gemini.AuthToken.SecretRef
	} else if llm.VertexAI != nil {
		secretRef = llm.VertexAI.AuthToken.SecretRef
	}

	return secretRef
}

func processUpstream(ctx context.Context, in ir.Upstream, out *envoy_config_cluster_v3.Cluster) {
	up, ok := in.Obj.(*v1alpha1.Upstream)
	if !ok {
		// log - should never happen
		return
	}

	ir, ok := in.ObjIr.(*UpstreamIr)
	if !ok {
		// log - should never happen
		return
	}

	spec := up.Spec

	switch {
	case spec.Static != nil:
		processStatic(ctx, spec.Static, out)
	case spec.Aws != nil:
		processAws(ctx, spec.Aws, ir, out)
	case spec.AI != nil:
		err := processAIUpstream(ctx, spec.AI, ir, out)
		if err != nil {
			// TODO: report error on status
			contextutils.LoggerFrom(ctx).Error(err)
		}
	}
}

func hostname(in *v1alpha1.Upstream) string {
	if in.Spec.Static != nil {
		if len(in.Spec.Static.Hosts) > 0 {
			return string(in.Spec.Static.Hosts[0].Host)
		}
	}
	return ""
}

func processEndpoints(up *v1alpha1.Upstream) *ir.EndpointsForUpstream {
	spec := up.Spec
	switch {
	case spec.Static != nil:
		return processEndpointsStatic(spec.Static)
	case spec.Aws != nil:
		return processEndpointsAws(spec.Aws)
	}
	return nil
}

func newPlug(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &upstreamPlugin{}
}

func (p *upstreamPlugin) Name() string {
	return "upstream"
}

// called 1 time for each listener
func (p *upstreamPlugin) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *upstreamPlugin) ApplyHCM(ctx context.Context,
	pCtx *ir.HcmContext,
	out *envoy_hcm.HttpConnectionManager) error { //no-op
	return nil
}

func (p *upstreamPlugin) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *upstreamPlugin) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	return nil
}

// Run on upstream, regardless of policy (based on upstream gvk)
// share route proto message
func (p *upstreamPlugin) ApplyForBackend(ctx context.Context, pCtx *ir.RouteBackendContext, in ir.HttpBackend, out *envoy_config_route_v3.Route) error {
	upstream := pCtx.Upstream.Obj.(*v1alpha1.Upstream)
	if upstream.Spec.AI != nil {
		err := applyAIBackend(ctx, upstream.Spec.AI, pCtx, in, out)
		if err != nil {
			return err
		}
	}

	return nil
}

func getUpstreamModel(llm *v1alpha1.LLMProviders, byType map[string]struct{}) string {
	llmModel := ""
	if llm.OpenAI != nil {
		byType["openai"] = struct{}{}
		if llm.OpenAI.Model != nil {
			llmModel = *llm.OpenAI.Model
		}
	} else if llm.Anthropic != nil {
		byType["anthropic"] = struct{}{}
		if llm.Anthropic.Model != nil {
			llmModel = *llm.Anthropic.Model
		}
	} else if llm.AzureOpenAI != nil {
		byType["azure_openai"] = struct{}{}
		llmModel = llm.AzureOpenAI.DeploymentName
	} else if llm.Gemini != nil {
		byType["gemini"] = struct{}{}
		llmModel = llm.Gemini.Model
	} else if llm.VertexAI != nil {
		byType["vertex-ai"] = struct{}{}
		llmModel = llm.VertexAI.Model
	}
	return llmModel
}

// Only called if policy attatched (extension ref)
// Can implement in route policy for ai (prompt guard, etc.)
// Alt. apply regardless if policy is present...?
func (p *upstreamPlugin) ApplyForRouteBackend(
	ctx context.Context, policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	pol, ok := policy.(*upstreamDestination)
	if !ok {
		return nil
		// todo: should we return fmt.Errorf("internal error: policy is not a upstreamDestination")
	}

	// TODO: clean up
	up, ok := pCtx.Upstream.Obj.(*v1alpha1.Upstream)
	if !ok {
		// log - should never happen
		return fmt.Errorf("invalid upstream %s.%s", pCtx.Upstream.GetName(), pCtx.Upstream.GetNamespace())
	}
	if up.Spec.AI != nil {
		if p.aiGatewayEnabled == nil {
			p.aiGatewayEnabled = make(map[string]bool)
		}
		p.aiGatewayEnabled[pCtx.FilterChainName] = true
	}

	return p.processBackendAws(ctx, pCtx, pol)

}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *upstreamPlugin) HttpFilters(ctx context.Context, fc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	result := []plugins.StagedHttpFilter{}

	if p.aiGatewayEnabled[fc.FilterChainName] {
		// TODO: add ratelimit and jwt_authn if AI Upstream is configured
		extProcSettings := &envoy_ext_proc_v3.ExternalProcessor{
			GrpcService: &envoy_config_core_v3.GrpcService{
				Timeout: durationpb.New(5 * time.Second),
				RetryPolicy: &envoy_config_core_v3.RetryPolicy{
					NumRetries: wrapperspb.UInt32(3),
				},
				TargetSpecifier: &envoy_config_core_v3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &envoy_config_core_v3.GrpcService_EnvoyGrpc{
						ClusterName: extProcUDSClusterName,
					},
				},
			},
			ProcessingMode: &envoy_ext_proc_v3.ProcessingMode{
				RequestHeaderMode:   envoy_ext_proc_v3.ProcessingMode_SEND,
				RequestBodyMode:     envoy_ext_proc_v3.ProcessingMode_STREAMED,
				RequestTrailerMode:  envoy_ext_proc_v3.ProcessingMode_SKIP,
				ResponseHeaderMode:  envoy_ext_proc_v3.ProcessingMode_SEND,
				ResponseBodyMode:    envoy_ext_proc_v3.ProcessingMode_STREAMED,
				ResponseTrailerMode: envoy_ext_proc_v3.ProcessingMode_SKIP,
			},
			MessageTimeout: durationpb.New(5 * time.Second),
			MetadataOptions: &envoy_ext_proc_v3.MetadataOptions{
				ForwardingNamespaces: &envoy_ext_proc_v3.MetadataOptions_MetadataNamespaces{
					Untyped: []string{"io.solo.transformation", "envoy.filters.ai.solo.io", "envoy.filters.http.jwt_authn"},
					Typed:   []string{"envoy.filters.ai.solo.io"},
				},
				ReceivingNamespaces: &envoy_ext_proc_v3.MetadataOptions_MetadataNamespaces{
					Untyped: []string{"ai.kgateway.io"},
				},
			},
		}
		// Run before rate limiting
		stagedFilter, err := plugins.NewStagedFilter(
			wellknown.AIExtProcFilterName,
			extProcSettings,
			plugins.FilterStage[plugins.WellKnownFilterStage]{
				RelativeTo: plugins.RateLimitStage,
				Weight:     -2,
			},
		)
		if err != nil {
			return nil, err
		}
		result = append(result, stagedFilter)
	}
	if p.needFilter[fc.FilterChainName] {

		filterConfig := &awspb.AWSLambdaConfig{}
		pluginStage := plugins.DuringStage(plugins.OutAuthStage)
		f, _ := plugins.NewStagedFilter(FilterName, filterConfig, pluginStage)

		result = append(result, f)
	}
	return result, nil
}

func (p *upstreamPlugin) UpstreamHttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedUpstreamHttpFilter, error) {
	if !p.aiGatewayEnabled[fcc.FilterChainName] {
		return nil, nil
	}

	transformationMsg, err := utils.MessageToAny(&envoytransformation.FilterTransformations{})
	if err != nil {
		return nil, err
	}

	upstreamWaitMsg, err := utils.MessageToAny(&upstream_wait.UpstreamWaitFilterConfig{})
	if err != nil {
		return nil, err
	}

	filters := []plugins.StagedUpstreamHttpFilter{
		// The wait filter essentially blocks filter iteration until a host has been selected.
		// This is important because running as an upstream filter allows access to host
		// metadata iff the host has already been selected, and that's a
		// major benefit of running the filter at this stage.
		{
			Filter: &envoy_hcm.HttpFilter{
				Name: waitFilterName,
				ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
					TypedConfig: upstreamWaitMsg,
				},
			},
			Stage: plugins.UpstreamHTTPFilterStage{
				RelativeTo: plugins.TransformationStage,
				Weight:     -1,
			},
		},
		{
			Filter: &envoy_hcm.HttpFilter{
				Name: wellknown.TransformationFilterName,
				ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
					TypedConfig: transformationMsg,
				},
			},
			Stage: plugins.UpstreamHTTPFilterStage{
				RelativeTo: plugins.TransformationStage,
				Weight:     0,
			},
		},
	}

	return filters, nil
}

func (p *upstreamPlugin) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *upstreamPlugin) ResourcesToAdd(ctx context.Context) ir.Resources {
	// This env var can be used to test the ext-proc filter locally.
	// On linux this should be set to `172.17.0.1` and on mac to `host.docker.internal`
	// Note: Mac doesn't work yet because it needs to be a DNS cluster
	// The port can be whatever you want.
	// When running the ext-proc filter locally, you also need to set
	// `LISTEN_ADDR` to `0.0.0.0:PORT`. Where port is the same port as above.
	listenAddr := strings.Split(os.Getenv("AI_PLUGIN_LISTEN_ADDR"), ":")

	var ep *envoy_config_endpoint_v3.LbEndpoint
	if len(listenAddr) == 2 {
		port, _ := strconv.Atoi(listenAddr[1])
		ep = &envoy_config_endpoint_v3.LbEndpoint{
			HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
				Endpoint: &envoy_config_endpoint_v3.Endpoint{
					Address: &envoy_config_core_v3.Address{
						Address: &envoy_config_core_v3.Address_SocketAddress{
							SocketAddress: &envoy_config_core_v3.SocketAddress{
								Address: listenAddr[0],
								PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
									PortValue: uint32(port),
								},
							},
						},
					},
				},
			},
		}
	} else {
		ep = &envoy_config_endpoint_v3.LbEndpoint{
			HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
				Endpoint: &envoy_config_endpoint_v3.Endpoint{
					Address: &envoy_config_core_v3.Address{
						Address: &envoy_config_core_v3.Address_Pipe{
							Pipe: &envoy_config_core_v3.Pipe{
								Path: extProcUDSSocketPath,
							},
						},
					},
				},
			},
		}
	}
	// Add UDS cluster for the ext-proc filter
	udsCluster := &envoy_config_cluster_v3.Cluster{
		Name: extProcUDSClusterName,
		ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
			Type: envoy_config_cluster_v3.Cluster_STATIC,
		},
		Http2ProtocolOptions: &envoy_config_core_v3.Http2ProtocolOptions{},
		LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: extProcUDSClusterName,
			Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
				{
					LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
						ep,
					},
				},
			},
		},
	}

	additionalCluster := []*envoy_config_cluster_v3.Cluster{udsCluster}

	return ir.Resources{
		Clusters: additionalCluster,
	}
}

func getSecretIr(secrets *krtcollections.SecretIndex, krtctx krt.HandlerContext, secretName, ns string) (*ir.Secret, error) {
	secretRef := gwv1.SecretObjectReference{
		Name: gwv1.ObjectName(secretName),
	}
	secret, err := secrets.GetSecret(krtctx, krtcollections.From{GroupKind: v1alpha1.UpstreamGVK.GroupKind(), Namespace: ns}, secretRef)
	if secret != nil {
		return secret, nil
	} else {
		return nil, eris.Wrapf(err, fmt.Sprintf("unable to find the secret %s", secretRef.Name))
	}
}

func getMultiPoolSecretKey(priorityIdx, poolIdx int, secretName string) string {
	return fmt.Sprintf("%d-%d-%s", priorityIdx, poolIdx, secretName)
}
