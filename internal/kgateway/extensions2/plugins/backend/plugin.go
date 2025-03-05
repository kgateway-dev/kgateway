package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	awspb "github.com/solo-io/envoy-gloo/go/config/filter/http/aws_lambda/v2"
	"github.com/solo-io/go-utils/contextutils"
	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/backend/ai"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
)

const (
	ParameterGroup = "kgateway.io"
	ParameterKind  = "Parameter"

	ExtensionName = "Backend"
	FilterName    = "io.solo.aws_lambda"
)

var (
	ParameterGK = schema.GroupKind{
		Group: ParameterGroup,
		Kind:  ParameterKind,
	}
)

type backendDestination struct {
	FunctionName string
}

func (d *backendDestination) CreationTime() time.Time {
	return time.Time{}
}

func (d *backendDestination) Equals(in any) bool {
	d2, ok := in.(*backendDestination)
	if !ok {
		return false
	}
	return d.FunctionName == d2.FunctionName
}

type BackendIr struct {
	AwsSecret     *ir.Secret
	AISecret      *ir.Secret
	AIMultiSecret map[string]*ir.Secret
}

func data(s *ir.Secret) map[string][]byte {
	if s == nil {
		return nil
	}
	return s.Data
}

func (u *BackendIr) Equals(other any) bool {
	otherBackend, ok := other.(*BackendIr)
	if !ok {
		return false
	}
	if !maps.EqualFunc(data(u.AwsSecret), data(otherBackend.AwsSecret), func(a, b []byte) bool {
		return bytes.Equal(a, b)
	}) {
		return false
	}
	if !maps.EqualFunc(data(u.AISecret), data(otherBackend.AISecret), func(a, b []byte) bool {
		return bytes.Equal(a, b)
	}) {
		return false
	}
	if !maps.EqualFunc(u.AIMultiSecret, otherBackend.AIMultiSecret, func(a, b *ir.Secret) bool {
		return maps.EqualFunc(data(a), data(b), func(a, b []byte) bool {
			return bytes.Equal(a, b)
		})
	}) {
		return false
	}

	return true
}

type backendPlugin struct {
	ir.UnimplementedProxyTranslationPass
	needFilter       map[string]bool
	aiGatewayEnabled map[string]bool
}

func registerTypes(ourCli versioned.Interface) {
	kubeclient.Register[*v1alpha1.Backend](
		v1alpha1.BackendGVK.GroupVersion().WithResource("backends"),
		v1alpha1.BackendGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().Backends(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().Backends(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes(commoncol.OurClient)

	col := krt.WrapClient(kclient.New[*v1alpha1.Backend](commoncol.Client), commoncol.KrtOpts.ToOptions("Backends")...)

	gk := v1alpha1.BackendGVK.GroupKind()
	translate := buildTranslateFunc(ctx, commoncol.Secrets)
	ucol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *ir.BackendObjectIR {
		// resolve secrets
		return &ir.BackendObjectIR{
			ObjectSource: ir.ObjectSource{
				Kind:      gk.Kind,
				Group:     gk.Group,
				Namespace: i.GetNamespace(),
				Name:      i.GetName(),
			},
			GvPrefix:          "backend",
			CanonicalHostname: hostname(i),
			Obj:               i,
			ObjIr:             translate(krtctx, i),
		}
	})

	endpoints := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *ir.EndpointsForBackend {
		return processEndpoints(i)
	})
	return extensionsplug.Plugin{
		ContributesBackends: map[schema.GroupKind]extensionsplug.BackendPlugin{
			gk: {
				BackendInit: ir.BackendInit{
					InitBackend: processBackend,
				},
				Endpoints: endpoints,
				Backends:  ucol,
			},
		},
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			ParameterGK: {
				Name:                      "backend",
				NewGatewayTranslationPass: newPlug,
				//			AttachmentPoints: []ir.AttachmentPoints{ir.HttpBackendRefAttachmentPoint},
				PoliciesFetch: func(n, ns string) ir.PolicyIR {
					// virtual policy - we don't have a real policy object
					return &backendDestination{
						FunctionName: n,
					}
				},
			},
			v1alpha1.BackendGVK.GroupKind(): {
				Name:                      "backend",
				NewGatewayTranslationPass: newPlug,
				//			AttachmentPoints: []ir.AttachmentPoints{ir.HttpBackendRefAttachmentPoint},
				PoliciesFetch: func(n, ns string) ir.PolicyIR {
					// virtual policy - we don't have a real policy object
					return &backendDestination{
						FunctionName: n,
					}
				},
			},
		},
	}
}

func buildTranslateFunc(ctx context.Context, secrets *krtcollections.SecretIndex) func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *BackendIr {
	return func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *BackendIr {
		// resolve secrets
		var backendIr BackendIr
		if i.Spec.Aws != nil {
			ns := i.GetNamespace()
			secret, err := pluginutils.GetSecretIr(secrets, krtctx, i.Spec.Aws.SecretRef.Name, ns)
			if err != nil {
				contextutils.LoggerFrom(ctx).Error(err)
			}
			backendIr.AwsSecret = secret
		}
		if i.Spec.AI != nil {
			ns := i.GetNamespace()
			if i.Spec.AI.LLM != nil {
				secretRef := getAISecretRef(i.Spec.AI.LLM.Provider)
				// if secretRef is used, set the secret on the backend ir
				if secretRef != nil {
					secret, err := pluginutils.GetSecretIr(secrets, krtctx, secretRef.Name, ns)
					if err != nil {
						contextutils.LoggerFrom(ctx).Error(err)
					}
					backendIr.AISecret = secret
				}
			} else if i.Spec.AI.MultiPool != nil {
				backendIr.AIMultiSecret = map[string]*ir.Secret{}
				for idx, priority := range i.Spec.AI.MultiPool.Priorities {
					for jdx, pool := range priority.Pool {
						secretRef := getAISecretRef(pool.Provider)
						// if secretRef is used, set the secret on the backend ir
						if secretRef != nil {
							secret, err := pluginutils.GetSecretIr(secrets, krtctx, secretRef.Name, ns)
							if err != nil {
								contextutils.LoggerFrom(ctx).Error(err)
							}
							backendIr.AIMultiSecret[getMultiPoolSecretKey(idx, jdx, secretRef.Name)] = secret
						}
					}
				}
			}
		}
		return &backendIr
	}
}

func getAISecretRef(llm v1alpha1.SupportedLLMProvider) *corev1.LocalObjectReference {
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

func processBackend(ctx context.Context, in ir.BackendObjectIR, out *envoy_config_cluster_v3.Cluster) {
	up, ok := in.Obj.(*v1alpha1.Backend)
	if !ok {
		// log - should never happen
		return
	}

	ir, ok := in.ObjIr.(*BackendIr)
	if !ok {
		// log - should never happen
		return
	}

	spec := up.Spec
	switch {
	case spec.Type == v1alpha1.BackendTypeStatic:
		processStatic(ctx, spec.Static, out)
	case spec.Type == v1alpha1.BackendTypeAWS:
		processAws(ctx, spec.Aws, ir, out)
	case spec.Type == v1alpha1.BackendTypeAI:
		err := ai.ProcessAIBackend(ctx, spec.AI, ir.AISecret, out)
		if err != nil {
			// TODO: report error on status
			contextutils.LoggerFrom(ctx).Error(err)
		}
		err = ai.AddUpstreamClusterHttpFilters(out)
		if err != nil {
			contextutils.LoggerFrom(ctx).Error(err)
		}
	}
}

func hostname(in *v1alpha1.Backend) string {
	if in.Spec.Type != v1alpha1.BackendTypeStatic {
		return ""
	}
	if in.Spec.Static != nil {
		if len(in.Spec.Static.Hosts) > 0 {
			return in.Spec.Static.Hosts[0].Host
		}
	}
	return ""
}

func processEndpoints(up *v1alpha1.Backend) *ir.EndpointsForBackend {
	spec := up.Spec
	switch {
	case spec.Type == v1alpha1.BackendTypeStatic:
		return processEndpointsStatic(spec.Static)
	case spec.Type == v1alpha1.BackendTypeAWS:
		return processEndpointsAws(spec.Aws)
	}
	return nil
}

func newPlug(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &backendPlugin{}
}

func (p *backendPlugin) Name() string {
	return ExtensionName
}

// called 1 time for each listener
func (p *backendPlugin) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *backendPlugin) ApplyHCM(ctx context.Context,
	pCtx *ir.HcmContext,
	out *envoy_hcm.HttpConnectionManager) error { //no-op
	return nil
}

func (p *backendPlugin) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times
func (p *backendPlugin) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	return nil
}

// Run on backend, regardless of policy (based on backend gvk)
// share route proto message
func (p *backendPlugin) ApplyForBackend(ctx context.Context, pCtx *ir.RouteBackendContext, in ir.HttpBackend, out *envoy_config_route_v3.Route) error {
	backend := pCtx.Backend.Obj.(*v1alpha1.Backend)
	if backend.Spec.AI != nil {
		err := ai.ApplyAIBackend(ctx, backend.Spec.AI, pCtx, out)
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
		// TODO: optimize this be on the route config so it applied to all routes (https://github.com/kgateway-dev/kgateway/issues/10721)
		disabledExtprocSettings := &envoy_ext_proc_v3.ExtProcPerRoute{
			Override: &envoy_ext_proc_v3.ExtProcPerRoute_Disabled{
				Disabled: true,
			},
		}
		pCtx.AddTypedConfig(wellknown.AIExtProcFilterName, disabledExtprocSettings)
	}

	return nil
}

// Only called if policy attached (extension ref)
// Can implement in route policy for ai (prompt guard, etc.)
// Alt. apply regardless if policy is present...?
func (p *backendPlugin) ApplyForRouteBackend(
	ctx context.Context, policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	pol, ok := policy.(*backendDestination)
	if !ok {
		return nil
		// todo: should we return fmt.Errorf("internal error: policy is not a backend destination")
	}

	// TODO: AI config for ApplyToRouteBackend

	return p.processBackendAws(ctx, pCtx, pol)
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *backendPlugin) HttpFilters(ctx context.Context, fc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	result := []plugins.StagedHttpFilter{}

	var errs []error
	if p.aiGatewayEnabled[fc.FilterChainName] {
		aiFilters, err := ai.AddExtprocHTTPFilter()
		if err != nil {
			errs = append(errs, err)
		}
		result = append(result, aiFilters...)
	}
	if p.needFilter[fc.FilterChainName] {
		filterConfig := &awspb.AWSLambdaConfig{}
		pluginStage := plugins.DuringStage(plugins.OutAuthStage)
		f, _ := plugins.NewStagedFilter(FilterName, filterConfig, pluginStage)

		result = append(result, f)
	}
	return result, errors.Join(errs...)
}

func (p *backendPlugin) UpstreamHttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *backendPlugin) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *backendPlugin) ResourcesToAdd(ctx context.Context) ir.Resources {
	var additionalClusters []*envoy_config_cluster_v3.Cluster

	if len(p.aiGatewayEnabled) > 0 {
		aiClusters := ai.GetAIAdditionalResources(ctx)

		additionalClusters = append(additionalClusters, aiClusters...)
	}
	return ir.Resources{
		Clusters: additionalClusters,
	}
}

func getMultiPoolSecretKey(priorityIdx, poolIdx int, secretName string) string {
	return fmt.Sprintf("%d-%d-%s", priorityIdx, poolIdx, secretName)
}
