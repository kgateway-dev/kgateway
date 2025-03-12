package routepolicy

import (
	"context"
	"time"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

type routePolicy struct {
	ct       time.Time
	spec     v1alpha1.RoutePolicySpec
	AISecret *ir.Secret
	ExtProc  *ExtprocIR
}

type ExtprocIR struct {
	Name string
	// BackendRef *envoy_config_route_v3.Route_Route
	// Extproc    *v1alpha1.ExtProcPolicy
	ExtProc *envoy_ext_proc_v3.ExternalProcessor
}

func (d *routePolicy) CreationTime() time.Time {
	return d.ct
}

func (d *routePolicy) Equals(in any) bool {
	d2, ok := in.(*routePolicy)
	if !ok {
		return false
	}

	// if d.ExtProc != nil && d2.ExtProc != nil {
	return d.spec == d2.spec
}

type routePolicyPluginGwPass struct {
	ir.UnimplementedProxyTranslationPass
	setAIFilter bool
	// extprocConfig []*envoy_ext_proc_v3.ExternalProcessor // could have list of backend, name, and config
	// extprocStage  *v1alpha1.FilterStage
}

func (p *routePolicyPluginGwPass) ApplyHCM(ctx context.Context, pCtx *ir.HcmContext, out *envoyhttp.HttpConnectionManager) error {
	routePolicy := pCtx.Policy.(*routePolicy)
	if routePolicy.ExtProc != nil {
		extprocFilters, err := ExtprocHCMFilter(routePolicy.ExtProc)
		if err != nil {
			return err
		}
		out.HttpFilters = append(out.GetHttpFilters(), extprocFilters)
	}
	return nil
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionplug.Plugin {
	errors := []error{}
	col := krtutil.SetupCollectionDynamic[v1alpha1.RoutePolicy](
		ctx,
		commoncol.Client,
		v1alpha1.SchemeGroupVersion.WithResource("routepolicies"),
		commoncol.KrtOpts.ToOptions("RoutePolicy")...,
	)
	gk := wellknown.RoutePolicyGVK.GroupKind()
	translate := buildTranslateFunc(ctx, commoncol.Secrets, commoncol)
	// RoutePolicy IR will have TypedConfig -> implement backendroute method to add prompt guard, etc.
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.RoutePolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: policyCR.Namespace,
			Name:      policyCR.Name,
		}
		policyIr, err := translate(krtctx, policyCR, objSrc)
		if err != nil {
			contextutils.LoggerFrom(ctx).Error(err)
			errors = append(errors, err)
			return nil
		}
		var pol = &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       policyCR,
			PolicyIR:     policyIr,
			TargetRefs:   convert(policyCR.Spec.TargetRef),
			Errors:       errors,
		}
		return pol
	})

	return extensionplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			wellknown.RoutePolicyGVK.GroupKind(): {
				//AttachmentPoints: []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				NewGatewayTranslationPass: NewGatewayTranslationPass,
				Policies:                  policyCol,
			},
		},
	}
}

func convert(targetRef v1alpha1.LocalPolicyTargetReference) []ir.PolicyTargetRef {
	return []ir.PolicyTargetRef{{
		Kind:  string(targetRef.Kind),
		Name:  string(targetRef.Name),
		Group: string(targetRef.Group),
	}}
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass {
	return &routePolicyPluginGwPass{}
}
func (p *routePolicy) Name() string {
	return "routepolicies"
}

// called 1 time for each listener
func (p *routePolicyPluginGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *routePolicyPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called 0 or more times

// figure out apply for route or route backend
// maybe both
func (p *routePolicyPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	policy, ok := pCtx.Policy.(*routePolicy)
	if !ok {
		return nil
	}

	if policy.spec.Timeout > 0 && outputRoute.GetRoute() != nil {
		outputRoute.GetRoute().Timeout = durationpb.New(time.Second * time.Duration(policy.spec.Timeout))
	}

	// if policy.ExtProc != nil {
	// 	err := enableExtprocFilter(outputRoute, policy.ExtProc.Name)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	// here or apply for route backend????

	// TODO: err/warn/ignore if targetRef is set on non-AI Backend

	return nil
}

// ApplyForBackend applies regardless if policy is attached
func (p *routePolicyPluginGwPass) ApplyForBackend(
	ctx context.Context,
	pCtx *ir.RouteBackendContext,
	in ir.HttpBackend,
	out *envoy_config_route_v3.Route,
) error {
	return nil
}

func (p *routePolicyPluginGwPass) ApplyForRouteBackend( //Apply for route policy
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	rtPolicy, ok := policy.(*routePolicy)
	if !ok {
		return nil
	}
	if rtPolicy.ExtProc != nil {
		enableExtprocFilter(pCtx, rtPolicy.ExtProc.Name)
	}

	extprocSettingsProto := pCtx.GetTypedConfig(wellknown.AIExtProcFilterName)
	if extprocSettingsProto == nil {
		return nil
	}
	extprocSettings, ok := extprocSettingsProto.(*envoy_ext_proc_v3.ExtProcPerRoute)
	if !ok {
		// TODO: internal error
		return nil
	}

	err := p.processAIRoutePolicy(ctx, rtPolicy.spec.AI, pCtx, extprocSettings, rtPolicy.AISecret)
	if err != nil {
		// TODO: report error on status
		return err
	}

	// policy has override
	// pCtx.AddTypedConfig(wellknown.ExtProcFilterName, extprocSettings)
	// marshelling will happen for you
	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *routePolicyPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	// here do nothing
	// actually can put here instead of httplistenerpolicy
	// if p.extprocConfig != nil {
	// 	extprocFilters, err := AddExtprocHTTPFilter(p.extprocConfig, p.extprocStage)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return extprocFilters, nil
	// }

	// add empty extproc filter
	// if condition
	extprocFilter := plugins.MustNewStagedFilter(
		wellknown.ExtprocFilterName,
		&envoy_ext_proc_v3.ExternalProcessor{},
		plugins.FilterStage[plugins.WellKnownFilterStage]{
			RelativeTo: plugins.WellKnownFilterStage(plugins.AuthZStage),
			Weight:     1,
		},
	)

	return []plugins.StagedHttpFilter{extprocFilter}, nil
}

func (p *routePolicyPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *routePolicyPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}

func buildTranslateFunc(ctx context.Context, secrets *krtcollections.SecretIndex, commoncol *common.CommonCollections) func(krtctx krt.HandlerContext, policyCR *v1alpha1.RoutePolicy, objSrc ir.ObjectSource) (*routePolicy, error) {
	return func(krtctx krt.HandlerContext, policyCR *v1alpha1.RoutePolicy, objSrc ir.ObjectSource) (*routePolicy, error) {
		policyIr := routePolicy{ct: policyCR.CreationTimestamp.Time, spec: policyCR.Spec}

		if policyCR.Spec.ExtProc != nil {
			extproc, err := toEnvoyExtProc(policyCR.Spec.ExtProc, krtctx, commoncol, objSrc)
			if err != nil {
				return nil, err
			}
			policyIr.ExtProc = &ExtprocIR{
				Name:    policyCR.Name, // TODO format
				ExtProc: extproc,
			}
		}

		// Check for the presence of the OpenAI Moderation which may require a secret reference
		if policyCR.Spec.AI == nil ||
			policyCR.Spec.AI.PromptGuard == nil ||
			policyCR.Spec.AI.PromptGuard.Request == nil ||
			policyCR.Spec.AI.PromptGuard.Request.Moderation == nil {
			return &policyIr, nil
		}

		secretRef := policyCR.Spec.AI.PromptGuard.Request.Moderation.OpenAIModeration.AuthToken.SecretRef
		if secretRef == nil {
			// no secret ref is set
			return &policyIr, nil
		}

		// Retrieve and assign the secret
		secret, err := pluginutils.GetSecretIr(secrets, krtctx, secretRef.Name, policyCR.GetNamespace())
		if err != nil {
			contextutils.LoggerFrom(ctx).Error(err)
			return &policyIr, nil
		}

		policyIr.AISecret = secret
		return &policyIr, nil
	}
}
