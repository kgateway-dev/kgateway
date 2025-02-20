package upstream

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"time"

	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/upstream/ai"
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
)

const (
	ParameterGroup = "kgateway.io"
	ParameterKind  = "Parameter"

	FilterName = "io.solo.aws_lambda"
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
		err := ai.ProcessAIUpstream(ctx, spec.AI, ir.AISecret, out)
		if err != nil {
			// TODO: report error on status
			contextutils.LoggerFrom(ctx).Error(err)
		}
	}
}

func hostname(in *v1alpha1.Upstream) string {
	if in.Spec.Static != nil {
		if len(in.Spec.Static.Hosts) > 0 {
			return in.Spec.Static.Hosts[0].Host
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
		err := ai.ApplyAIBackend(ctx, upstream.Spec.AI, pCtx, in, out)
		if err != nil {
			return err
		}

		if p.aiGatewayEnabled == nil {
			p.aiGatewayEnabled = make(map[string]bool)
		}
		p.aiGatewayEnabled[pCtx.FilterChainName] = true
	} else {
		// If it's not an AI route we want to disable our ext-proc filter just in case.
		// This will have no effect if we don't add the listener filter
		disabledExtprocSettings := &envoy_ext_proc_v3.ExtProcPerRoute{
			Override: &envoy_ext_proc_v3.ExtProcPerRoute_Disabled{
				Disabled: true,
			},
		}
		pCtx.AddTypedConfig(wellknown.AIExtProcFilterName, disabledExtprocSettings)
	}

	return nil
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

	// TODO: AI config for ApplyToRouteBackend

	return p.processBackendAws(ctx, pCtx, pol)

}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *upstreamPlugin) HttpFilters(ctx context.Context, fc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	result := []plugins.StagedHttpFilter{}

	if p.aiGatewayEnabled[fc.FilterChainName] {
		aiFilters, err := ai.AddExtprocHTTPFilter()
		if err != nil {
			return nil, err
		}
		result = append(result, aiFilters...)
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
	filters := []plugins.StagedUpstreamHttpFilter{}
	if p.aiGatewayEnabled[fcc.FilterChainName] {
		aiFilters, err := ai.AddUpstreamHttpFilters()
		if err != nil {
			return nil, err
		}
		filters = append(filters, aiFilters...)
	}

	return filters, nil
}

func (p *upstreamPlugin) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *upstreamPlugin) ResourcesToAdd(ctx context.Context) ir.Resources {
	var additionalClusters []*envoy_config_cluster_v3.Cluster

	if len(p.aiGatewayEnabled) > 0 {
		aiClusters := ai.GetAIAdditionalResources()

		additionalClusters = append(additionalClusters, aiClusters...)
	}
	return ir.Resources{
		Clusters: additionalClusters,
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
