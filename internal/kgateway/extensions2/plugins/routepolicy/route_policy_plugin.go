package routepolicy

import (
	"context"
	"time"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	common_set_filter_state_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/set_filter_state/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	http_set_filter_state_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/set_filter_state/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

type routePolicy struct {
	ct       time.Time
	spec     v1alpha1.RoutePolicySpec
	AISecret *ir.Secret
}

func (d *routePolicy) CreationTime() time.Time {
	return d.ct
}

func (d *routePolicy) Equals(in any) bool {
	d2, ok := in.(*routePolicy)
	if !ok {
		return false
	}
	return d.spec == d2.spec
}

type routePolicyPluginGwPass struct {
	setAIFilter bool
}

func (p *routePolicyPluginGwPass) ApplyHCM(ctx context.Context, pCtx *ir.HcmContext, out *envoyhttp.HttpConnectionManager) error {
	// no op
	return nil
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionplug.Plugin {
	col := krtutil.SetupCollectionDynamic[v1alpha1.RoutePolicy](
		ctx,
		commoncol.Client,
		v1alpha1.SchemeGroupVersion.WithResource("routepolicies"),
		commoncol.KrtOpts.ToOptions("RoutePolicy")...,
	)
	gk := v1alpha1.RoutePolicyGVK.GroupKind()
	translate := buildTranslateFunc(ctx, commoncol.Secrets)
	// RoutePolicy IR will have TypedConfig -> implement backendroute method to add prompt guard, etc.
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.RoutePolicy) *ir.PolicyWrapper {
		var pol = &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: policyCR.Namespace,
				Name:      policyCR.Name,
			},
			Policy:     policyCR,
			PolicyIR:   translate(krtctx, policyCR),
			TargetRefs: convert(policyCR.Spec.TargetRef),
		}
		return pol
	})

	return extensionplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			v1alpha1.RoutePolicyGVK.GroupKind(): {
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
func (p *routePolicyPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	policy, ok := pCtx.Policy.(*routePolicy)
	if !ok {
		return nil
	}

	if policy.spec.Timeout > 0 && outputRoute.GetRoute() != nil {
		outputRoute.GetRoute().Timeout = durationpb.New(time.Second * time.Duration(policy.spec.Timeout))
	}

	// TODO: err/warn/ignore if targetRef is set on non-AI Upstream

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

func (p *routePolicyPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	extprocSettingsProto := pCtx.GetConfig(wellknown.AIExtProcFilterName)
	if extprocSettingsProto == nil {
		return nil
	}
	extprocSettings, ok := extprocSettingsProto.(*envoy_ext_proc_v3.ExtProcPerRoute)
	if !ok {
		// TODO: internal error
		return nil
	}

	transformationProto := pCtx.GetConfig(wellknown.TransformationFilterName)
	if transformationProto == nil {
		return nil
	}
	transformations, ok := transformationProto.(*envoytransformation.RouteTransformations)

	rtPolicy, ok := policy.(*routePolicy)
	if !ok {
		return nil
	}

	err := p.processAIRoutePolicy(ctx, rtPolicy.spec.AI, pCtx, extprocSettings, transformations, rtPolicy.AISecret)
	if err != nil {
		// TODO: report error on status
		return err
	}
	pCtx.AddTypedConfig(wellknown.AIExtProcFilterName, extprocSettings)

	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *routePolicyPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	var filters []plugins.StagedHttpFilter

	if p.setAIFilter {
		// handle route policy RouteType by setting it in the dynamic metadata
		fsConfig := &http_set_filter_state_v3.Config{
			OnRequestHeaders: []*common_set_filter_state_v3.FilterStateValue{
				{
					Key: &common_set_filter_state_v3.FilterStateValue_ObjectKey{
						ObjectKey: "envoy.route_type",
					},
					FactoryKey: "envoy.route_type",
					Value: &common_set_filter_state_v3.FilterStateValue_FormatString{
						FormatString: &envoy_config_core_v3.SubstitutionFormatString{
							Format: &envoy_config_core_v3.SubstitutionFormatString_TextFormatSource{
								TextFormatSource: &envoy_config_core_v3.DataSource{
									Specifier: &envoy_config_core_v3.DataSource_InlineString{
										InlineString: "%DYNAMIC_METADATA(envoy.route_type)%",
									},
								},
							},
						},
					},
				},
			},
		}
		stagedFilter, err := plugins.NewStagedFilter(
			wellknown.SetMetadataFilterName,
			fsConfig,
			plugins.BeforeStage(plugins.RouteStage),
		)
		if err != nil {
			return nil, err
		}
		filters = append(filters, stagedFilter)
	}

	return filters, nil
}

func (p *routePolicyPluginGwPass) UpstreamHttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedUpstreamHttpFilter, error) {
	return nil, nil
}

func (p *routePolicyPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *routePolicyPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
}

func buildTranslateFunc(ctx context.Context, secrets *krtcollections.SecretIndex) func(krtctx krt.HandlerContext, i *v1alpha1.RoutePolicy) *routePolicy {
	return func(krtctx krt.HandlerContext, policyCR *v1alpha1.RoutePolicy) *routePolicy {
		policyIr := routePolicy{ct: policyCR.CreationTimestamp.Time, spec: policyCR.Spec}

		// Check for the presence of the OpenAI Moderation which may require a secret reference
		if policyCR.Spec.AI == nil || policyCR.Spec.AI.PromptGuard == nil || policyCR.Spec.AI.PromptGuard.Request.Moderation == nil {
			return &policyIr
		}

		secretRef := policyCR.Spec.AI.PromptGuard.Request.Moderation.OpenAIModeration.AuthToken.SecretRef
		if secretRef == nil {
			// no secret ref is set
			return &policyIr
		}

		// Retrieve and assign the secret
		secret, err := pluginutils.GetSecretIr(secrets, krtctx, secretRef.Name, policyCR.GetNamespace())
		if err != nil {
			contextutils.LoggerFrom(ctx).Error(err)
			return &policyIr
		}

		policyIr.AISecret = secret
		return &policyIr
	}
}
