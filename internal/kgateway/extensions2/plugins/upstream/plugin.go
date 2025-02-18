package upstream

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"time"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
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
)

const (
	FilterName = "io.solo.aws_lambda"
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
	needFilter map[string]bool
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
	} else if llm.Mistral != nil {
		secretRef = llm.Mistral.AuthToken.SecretRef
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

		// Setup ext-proc route filter config, we will conditionally modify it based on certain route options.
		// A heavily used part of this config is the `GrpcInitialMetadata`.
		// This is used to add headers to the ext-proc request.
		// These headers are used to configure the AI server on a per-request basis.
		// This was the best available way to pass per-route configuration to the AI server.
		extProcRouteSettings := &envoy_ext_proc_v3.ExtProcPerRoute{
			Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
				Overrides: &envoy_ext_proc_v3.ExtProcOverrides{},
			},
		}

		var llmModel string
		byType := map[string]struct{}{}
		aiUpstream := upstream.Spec.AI
		if aiUpstream.LLM != nil {
			llmModel = getUpstreamModel(aiUpstream.LLM, byType)
		} else if aiUpstream.MultiPool != nil {
			for _, priority := range aiUpstream.MultiPool.Priorities {
				for _, pool := range priority.Pool {
					llmModel = getUpstreamModel(&pool, byType)
				}
			}
		}

		if len(byType) != 1 {
			return eris.Errorf("multiple AI backend types found for single ai route %+v", byType)
		}

		// This is only len(1)
		var llmProvider string
		for k := range byType {
			llmProvider = k
		}

		// Add things which require basic AI upstream.
		pCtx.AddTypedConfig("AutoHostRewrite", wrapperspb.Bool(true))

		// We only want to add the transformation filter if we have a single AI backend
		// Otherwise we already have the transformation filter added by the weighted destination
		// Setup initial transformation template. This may be modified by further AI RoutePolicy config.
		//if _, ok := p.transformationsByRoute[in]; !ok {
		//	p.transformationsByRoute[in] = []*transformationWithOutput{
		//		{
		//			// It's safe to use the first as they will all be of the same type at this point in the code
		//			transformation:  getTransformationTemplateForUpstream(params.Ctx, aiUpstreams[0], in.GetOptions()),
		//			perFilterConfig: out.TypedPerFilterConfig,
		//		},
		//	}
		//}

		extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
			&envoy_config_core_v3.HeaderValue{
				Key:   "x-llm-provider",
				Value: llmProvider,
			},
		)
		// If the Upstream specifies a model, add a header to the ext-proc request
		if llmModel != "" {
			extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
				&envoy_config_core_v3.HeaderValue{
					Key:   "x-llm-model",
					Value: llmModel,
				})
		}

		// Add the x-request-id header to the ext-proc request.
		// This is an optimization to allow us to not have to wait for the headers request to
		// Initialize our logger/handler classes.
		extProcRouteSettings.GetOverrides().GrpcInitialMetadata = append(extProcRouteSettings.GetOverrides().GetGrpcInitialMetadata(),
			&envoy_config_core_v3.HeaderValue{
				Key:   "x-request-id",
				Value: "%REQ(X-REQUEST-ID)%",
			},
		)

		pCtx.AddTypedConfig(wellknown.ExtProcFilterName, extProcRouteSettings)
	}

	return nil
}

func getUpstreamModel(llm *v1alpha1.LLMProviders, byType map[string]struct{}) string {
	llmModel := ""
	if llm.OpenAI != nil {
		byType["openai"] = struct{}{}
		llmModel = llm.OpenAI.Model
	} else if llm.Mistral != nil {
		byType["mistral"] = struct{}{}
		llmModel = llm.Mistral.Model
	} else if llm.Anthropic != nil {
		byType["anthropic"] = struct{}{}
		llmModel = llm.Anthropic.Model
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
	return p.processBackendAws(ctx, pCtx, pol)
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *upstreamPlugin) HttpFilters(ctx context.Context, fc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	if !p.needFilter[fc.FilterChainName] {
		return nil, nil
	}
	filterConfig := &awspb.AWSLambdaConfig{}
	pluginStage := plugins.DuringStage(plugins.OutAuthStage)
	f, _ := plugins.NewStagedFilter(FilterName, filterConfig, pluginStage)

	return []plugins.StagedHttpFilter{
		f,
	}, nil
}

func (p *upstreamPlugin) UpstreamHttpFilters(ctx context.Context) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *upstreamPlugin) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *upstreamPlugin) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
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
