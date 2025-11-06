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
	"istio.io/istio/pkg/kube/kubetypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

var logger = logging.New("plugin/backend")

const (
	ExtensionName = "backend"
)

// backendIr is the internal representation of a backend.
type backendIr struct {
	awsIr    *AwsIr
	staticIr *StaticIr
	dfpIr    *DfpIr
	errors   []error
}

func (u *backendIr) Equals(other any) bool {
	otherBackend, ok := other.(*backendIr)
	if !ok {
		return false
	}
	// AWS
	if !u.awsIr.Equals(otherBackend.awsIr) {
		return false
	}
	// Static
	if !u.staticIr.Equals(otherBackend.staticIr) {
		return false
	}
	// DFP
	if !u.dfpIr.Equals(otherBackend.dfpIr) {
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
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.Backend] {
			return ourCli.GatewayV1alpha1().Backends(namespace)
		},
	)
}

func NewPlugin(commoncol *collections.CommonCollections) sdk.Plugin {
	registerTypes(commoncol.OurClient)

	cli := kclient.NewFilteredDelayed[*v1alpha1.Backend](
		commoncol.Client,
		wellknown.BackendGVR,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	)

	col := krt.WrapClient(cli, commoncol.KrtOpts.ToOptions("Backends")...)

	gk := wellknown.BackendGVK.GroupKind()
	translateFn := buildTranslateFunc(commoncol.Secrets)
	bcol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *ir.BackendObjectIR {
		backendIR := translateFn(krtctx, i)
		if len(backendIR.errors) > 0 {
			logger.Error("failed to translate backend", "backend", i.GetName(), "error", errors.Join(backendIR.errors...))
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
		backend.Errors = backendIR.errors

		// Parse common annotations
		ir.ParseObjectAnnotations(&backend, i)

		return &backend
	})
	endpoints := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *ir.EndpointsForBackend {
		return processEndpoints(i)
	})
	return sdk.Plugin{
		ContributesBackends: map[schema.GroupKind]sdk.BackendPlugin{
			gk: {
				BackendInit: ir.BackendInit{
					InitEnvoyBackend: processBackendForEnvoy,
				},
				Endpoints: endpoints,
				Backends:  bcol,
			},
		},
		ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			wellknown.BackendGVK.GroupKind(): {
				Name:                      "backend",
				NewGatewayTranslationPass: newPlug,
			},
		},
		ContributesLeaderAction: map[schema.GroupKind]func(){
			wellknown.BackendGVK.GroupKind(): buildRegisterCallback(cli, bcol),
		},
	}
}

// buildTranslateFunc builds a function that translates a Backend to a backendIr that
// the plugin can use to build the envoy config.
func buildTranslateFunc(
	secrets *krtcollections.SecretIndex,
) func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *backendIr {
	return func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *backendIr {
		var beIr backendIr
		switch i.Spec.Type {
		case v1alpha1.BackendTypeStatic:
			staticIr, err := buildStaticIr(i.Spec.Static)
			if err != nil {
				beIr.errors = append(beIr.errors, err)
			}
			beIr.staticIr = staticIr
		case v1alpha1.BackendTypeDynamicForwardProxy:
			dfpIr, err := buildDfpIr(i.Spec.DynamicForwardProxy)
			if err != nil {
				beIr.errors = append(beIr.errors, err)
			}
			beIr.dfpIr = dfpIr
		case v1alpha1.BackendTypeAWS:
			region := getRegion(i.Spec.Aws)
			invokeMode := getLambdaInvocationMode(i.Spec.Aws)

			lambdaArn, err := buildLambdaARN(i.Spec.Aws, region)
			if err != nil {
				beIr.errors = append(beIr.errors, err)
			}

			endpointConfig, err := configureLambdaEndpoint(i.Spec.Aws)
			if err != nil {
				beIr.errors = append(beIr.errors, err)
			}

			var lambdaTransportSocket *envoycorev3.TransportSocket
			if endpointConfig.useTLS {
				// TODO(yuval-k): Add verification context
				typedConfig, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{
					Sni: endpointConfig.hostname,
				})
				if err != nil {
					beIr.errors = append(beIr.errors, err)
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
					beIr.errors = append(beIr.errors, err)
				}
			}

			lambdaFilters, err := buildLambdaFilters(
				lambdaArn, region, secret, invokeMode, i.Spec.Aws.Lambda.PayloadTransformMode)
			if err != nil {
				beIr.errors = append(beIr.errors, err)
			}

			beIr.awsIr = &AwsIr{
				lambdaEndpoint:        endpointConfig,
				lambdaTransportSocket: lambdaTransportSocket,
				lambdaFilters:         lambdaFilters,
			}
		}
		return &beIr
	}
}

func processBackendForEnvoy(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
	be, ok := in.Obj.(*v1alpha1.Backend)
	if !ok {
		logger.Error("failed to cast backend object")
		return nil
	}
	beIr, ok := in.ObjIr.(*backendIr)
	if !ok {
		logger.Error("failed to cast backend ir")
		return nil
	}

	// TODO: propagated error to CRD #11558.
	// TODO(tim): do we need to do anything here for AI backends?
	spec := be.Spec
	switch spec.Type {
	case v1alpha1.BackendTypeStatic:
		processStatic(beIr.staticIr, out)
	case v1alpha1.BackendTypeAWS:
		if err := processAws(beIr.awsIr, out); err != nil {
			logger.Error("failed to process aws backend", "error", err)
			beIr.errors = append(beIr.errors, err)
		}
	case v1alpha1.BackendTypeDynamicForwardProxy:
		processDynamicForwardProxy(beIr.dfpIr, out)
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
	needsDfpFilter map[string]bool
}

var _ ir.ProxyTranslationPass = &backendPlugin{}

func newPlug(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass {
	return &backendPlugin{}
}

func (p *backendPlugin) Name() string {
	return ExtensionName
}

func (p *backendPlugin) ApplyForBackend(pCtx *ir.RouteBackendContext, in ir.HttpBackend, out *envoyroutev3.Route) error {
	backend := pCtx.Backend.Obj.(*v1alpha1.Backend)
	switch backend.Spec.Type {
	case v1alpha1.BackendTypeDynamicForwardProxy:
		if p.needsDfpFilter == nil {
			p.needsDfpFilter = make(map[string]bool)
		}
		p.needsDfpFilter[pCtx.FilterChainName] = true
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
	}

	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *backendPlugin) HttpFilters(fc ir.FilterChainCommon) ([]filters.StagedHttpFilter, error) {
	result := []filters.StagedHttpFilter{}

	var errs []error
	if p.needsDfpFilter[fc.FilterChainName] {
		pluginStage := filters.DuringStage(filters.OutAuthStage)
		f := filters.MustNewStagedFilter("envoy.filters.http.dynamic_forward_proxy", dfpFilterConfig, pluginStage)
		result = append(result, f)
	}
	return result, errors.Join(errs...)
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *backendPlugin) ResourcesToAdd() ir.Resources {
	return ir.Resources{}
}
