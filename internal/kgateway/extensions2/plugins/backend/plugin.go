package backend

import (
	"context"
	"errors"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/backend/ai"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var logger = logging.New("plugin/backend")

const (
	ExtensionName = "backend"
)

// BackendIr is the internal representation of a backend.
// TODO: unexport
type BackendIr struct {
	AwsIr  *AwsIr
	AIIr   *ai.IR
	Errors []error
}

func (u *BackendIr) Equals(other any) bool {
	otherBackend, ok := other.(*BackendIr)
	if !ok {
		return false
	}
	// AI
	if !u.AIIr.Equals(otherBackend.AIIr) {
		return false
	}
	// AWS
	if !u.AwsIr.Equals(otherBackend.AwsIr) {
		return false
	}
	return true
}

func registerTypes(ourCli versioned.Interface) {
	kubeclient.Register[*v1alpha1.Backend](
		wellknown.BackendGVR,
		wellknown.BackendGVK,
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

	col := krt.WrapClient(kclient.NewFiltered[*v1alpha1.Backend](
		commoncol.Client,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	), commoncol.KrtOpts.ToOptions("Backends")...)

	gk := wellknown.BackendGVK.GroupKind()
	translateFn := buildTranslateFunc(ctx, commoncol.Secrets, commoncol.Services, commoncol.Namespaces)
	bcol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *ir.BackendObjectIR {
		backendIR := translateFn(krtctx, i)
		if len(backendIR.Errors) > 0 {
			logger.Error("failed to translate backend", "backend", i.GetName(), "error", errors.Join(backendIR.Errors...))
		}
		objSrc := ir.ObjectSource{
			Kind:      gk.Kind,
			Group:     gk.Group,
			Namespace: i.GetNamespace(),
			Name:      i.GetName(),
		}
		backend := ir.NewBackendObjectIR(objSrc, 0, "")
		backend.GvPrefix = ExtensionName
		backend.CanonicalHostname = hostname(i)
		backend.AppProtocol = parseAppProtocol(i)
		backend.Obj = i
		backend.ObjIr = backendIR
		backend.Errors = backendIR.Errors
		return &backend
	})
	endpoints := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *ir.EndpointsForBackend {
		return processEndpoints(i)
	})
	return extensionsplug.Plugin{
		ContributesBackends: map[schema.GroupKind]extensionsplug.BackendPlugin{
			gk: {
				BackendInit: ir.BackendInit{
					InitEnvoyBackend: processBackendForEnvoy,
				},
				Endpoints: endpoints,
				Backends:  bcol,
			},
		},
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			wellknown.BackendGVK.GroupKind(): {
				Name:                      "backend",
				NewGatewayTranslationPass: newPlug,
			},
		},
		ContributesLeaderAction: map[schema.GroupKind]func(){
			wellknown.BackendGVK.GroupKind(): buildRegisterCallback(ctx, commoncol.CrudClient, bcol),
		},
	}
}

// buildTranslateFunc builds a function that translates a Backend to a BackendIr that
// the plugin can use to build the envoy config.
func buildTranslateFunc(
	ctx context.Context,
	secrets *krtcollections.SecretIndex,
	services krt.Collection[*corev1.Service],
	namespaces krt.Collection[krtcollections.NamespaceMetadata],
) func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *BackendIr {
	return func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *BackendIr {
		var backendIr BackendIr
		switch i.Spec.Type {
		case v1alpha1.BackendTypeAWS:
			region := getRegion(i.Spec.Aws)
			invokeMode := getLambdaInvocationMode(i.Spec.Aws)

			lambdaArn, err := buildLambdaARN(i.Spec.Aws, region)
			if err != nil {
				backendIr.Errors = append(backendIr.Errors, err)
			}

			endpointConfig, err := configureLambdaEndpoint(i.Spec.Aws)
			if err != nil {
				backendIr.Errors = append(backendIr.Errors, err)
			}

			var lambdaTransportSocket *envoycorev3.TransportSocket
			if endpointConfig.useTLS {
				// TODO(yuval-k): Add verification context
				typedConfig, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{
					Sni: endpointConfig.hostname,
				})
				if err != nil {
					backendIr.Errors = append(backendIr.Errors, err)
				}
				lambdaTransportSocket = &envoycorev3.TransportSocket{
					Name: envoywellknown.TransportSocketTls,
					ConfigType: &envoycorev3.TransportSocket_TypedConfig{
						TypedConfig: typedConfig,
					},
				}
			}

			var secret *ir.Secret
			if i.Spec.Aws.Auth != nil && i.Spec.Aws.Auth.Type == v1alpha1.AwsAuthTypeSecret {
				var err error
				secret, err = pluginutils.GetSecretIr(secrets, krtctx, i.Spec.Aws.Auth.SecretRef.Name, i.GetNamespace())
				if err != nil {
					backendIr.Errors = append(backendIr.Errors, err)
				}
			}

			lambdaFilters, err := buildLambdaFilters(
				lambdaArn, region, secret, invokeMode, i.Spec.Aws.Lambda.PayloadTransformMode)
			if err != nil {
				backendIr.Errors = append(backendIr.Errors, err)
			}

			backendIr.AwsIr = &AwsIr{
				lambdaEndpoint:        endpointConfig,
				lambdaTransportSocket: lambdaTransportSocket,
				lambdaFilters:         lambdaFilters,
			}
		case v1alpha1.BackendTypeAI:
			backendIr.AIIr = &ai.IR{}
			err := ai.PreprocessAIBackend(ctx, i.Spec.AI, backendIr.AIIr)
			if err != nil {
				backendIr.Errors = append(backendIr.Errors, err)
			}
			ns := i.GetNamespace()
			if i.Spec.AI.LLM != nil {
				secretRef := getAISecretRef(i.Spec.AI.LLM.Provider)
				// if secretRef is used, set the secret on the backend ir
				if secretRef != nil {
					secret, err := pluginutils.GetSecretIr(secrets, krtctx, secretRef.Name, ns)
					if err != nil {
						backendIr.Errors = append(backendIr.Errors, err)
					}
					backendIr.AIIr.AISecret = secret
				}
				return &backendIr
			}
			if i.Spec.AI.MultiPool != nil {
				backendIr.AIIr.AIMultiSecret = map[string]*ir.Secret{}
				for idx, priority := range i.Spec.AI.MultiPool.Priorities {
					for jdx, pool := range priority.Pool {
						secretRef := getAISecretRef(pool.Provider)
						if secretRef == nil {
							continue
						}
						// if secretRef is used, set the secret on the backend ir
						secret, err := pluginutils.GetSecretIr(secrets, krtctx, secretRef.Name, ns)
						if err != nil {
							backendIr.Errors = append(backendIr.Errors, err)
						}
						backendIr.AIIr.AIMultiSecret[ai.GetMultiPoolSecretKey(idx, jdx, secretRef.Name)] = secret
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

func processBackendForEnvoy(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
	be, ok := in.Obj.(*v1alpha1.Backend)
	if !ok {
		logger.Error("failed to cast backend object")
		return nil
	}
	backendIr, ok := in.ObjIr.(*BackendIr)
	if !ok {
		logger.Error("failed to cast backend ir")
		return nil
	}

	// TODO(tim): Bubble up error to Backend status once https://github.com/kgateway-dev/kgateway/issues/10555
	// is resolved and add test cases for invalid endpoint URLs.
	spec := be.Spec
	switch spec.Type {
	case v1alpha1.BackendTypeStatic:
		if err := processStaticBackendForEnvoy(spec.Static, out); err != nil {
			logger.Error("failed to process static backend", "error", err)
			backendIr.Errors = append(backendIr.Errors, err)
		}
	case v1alpha1.BackendTypeAWS:
		if err := processAws(backendIr.AwsIr, out); err != nil {
			logger.Error("failed to process aws backend", "error", err)
			backendIr.Errors = append(backendIr.Errors, err)
		}
	case v1alpha1.BackendTypeAI:
		err := ai.ProcessAIBackend(spec.AI, backendIr.AIIr.AISecret, backendIr.AIIr.AIMultiSecret, out)
		if err != nil {
			logger.Error("failed to process ai backend", "error", err)
			backendIr.Errors = append(backendIr.Errors, err)
		}
		err = ai.AddUpstreamClusterHttpFilters(out)
		if err != nil {
			logger.Error("failed to add upstream cluster http filters", "error", err)
			backendIr.Errors = append(backendIr.Errors, err)
		}
	case v1alpha1.BackendTypeDynamicForwardProxy:
		if err := processDynamicForwardProxy(spec.DynamicForwardProxy, out); err != nil {
			logger.Error("failed to process dynamic forward proxy backend", "error", err)
			backendIr.Errors = append(backendIr.Errors, err)
		}
	}
	return nil
}

func parseAppProtocol(b *v1alpha1.Backend) ir.AppProtocol {
	switch b.Spec.Type {
	case v1alpha1.BackendTypeStatic:
		appProtocol := b.Spec.Static.AppProtocol
		if appProtocol != nil {
			return ir.ParseAppProtocol(ptr.To(string(*appProtocol)))
		}
	}
	return ir.DefaultAppProtocol
}

// hostname returns the hostname for the backend. Only static backends are supported.
func hostname(in *v1alpha1.Backend) string {
	if in.Spec.Type != v1alpha1.BackendTypeStatic {
		return ""
	}
	if len(in.Spec.Static.Hosts) == 0 {
		return ""
	}
	return in.Spec.Static.Hosts[0].Host
}

func processEndpoints(be *v1alpha1.Backend) *ir.EndpointsForBackend {
	spec := be.Spec
	switch {
	case spec.Type == v1alpha1.BackendTypeStatic:
		return processEndpointsStatic(spec.Static)
	case spec.Type == v1alpha1.BackendTypeAWS:
		return processEndpointsAws(spec.Aws)
	}
	return nil
}

type backendPlugin struct {
	ir.UnimplementedProxyTranslationPass
	aiGatewayEnabled map[string]bool
	needsDfpFilter   map[string]bool
}

var _ ir.ProxyTranslationPass = &backendPlugin{}

func newPlug(ctx context.Context, tctx ir.GwTranslationCtx, reporter reports.Reporter) ir.ProxyTranslationPass {
	return &backendPlugin{}
}

func (p *backendPlugin) Name() string {
	return ExtensionName
}

func (p *backendPlugin) ApplyForBackend(ctx context.Context, pCtx *ir.RouteBackendContext, in ir.HttpBackend, out *envoyroutev3.Route) error {
	backend := pCtx.Backend.Obj.(*v1alpha1.Backend)
	backendIr := pCtx.Backend.ObjIr.(*BackendIr)
	switch backend.Spec.Type {
	case v1alpha1.BackendTypeAI:
		err := ai.ApplyAIBackend(backendIr.AIIr, pCtx, out)
		if err != nil {
			return err
		}

		if p.aiGatewayEnabled == nil {
			p.aiGatewayEnabled = make(map[string]bool)
		}
		p.aiGatewayEnabled[pCtx.FilterChainName] = true
	default:
		// If it's not an AI route we want to disable our ext-proc filter just in case.
		// This will have no effect if we don't add the listener filter.
		// TODO: optimize this be on the route config so it applied to all routes (https://github.com/kgateway-dev/kgateway/issues/10721)
		disabledExtprocSettings := &envoy_ext_proc_v3.ExtProcPerRoute{
			Override: &envoy_ext_proc_v3.ExtProcPerRoute_Disabled{
				Disabled: true,
			},
		}
		pCtx.TypedFilterConfig.AddTypedConfig(wellknown.AIExtProcFilterName, disabledExtprocSettings)
	case v1alpha1.BackendTypeDynamicForwardProxy:
		if p.needsDfpFilter == nil {
			p.needsDfpFilter = make(map[string]bool)
		}
		p.needsDfpFilter[pCtx.FilterChainName] = true
	}

	return nil
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
	if p.needsDfpFilter[fc.FilterChainName] {
		pluginStage := plugins.DuringStage(plugins.OutAuthStage)
		f := plugins.MustNewStagedFilter("envoy.filters.http.dynamic_forward_proxy", dfpFilterConfig, pluginStage)
		result = append(result, f)
	}
	return result, errors.Join(errs...)
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *backendPlugin) ResourcesToAdd(ctx context.Context) ir.Resources {
	var additionalClusters []*envoyclusterv3.Cluster
	if len(p.aiGatewayEnabled) > 0 {
		aiClusters := ai.GetAIAdditionalResources(ctx)
		additionalClusters = append(additionalClusters, aiClusters...)
	}
	return ir.Resources{
		Clusters: additionalClusters,
	}
}
