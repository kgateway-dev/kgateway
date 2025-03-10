package routepolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	exteniondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"

	// TODO(nfuden): remove once rustformations are able to be used in a production environment
	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"github.com/solo-io/go-utils/contextutils"
)

const transformationFilterNamePrefix = "transformation"
const rustformationFilterNamePrefix = "dynamic_modules/simple_mutations"

type routePolicy struct {
	ct       time.Time
	spec     routeSpecIr
	AISecret *ir.Secret
}

type routeSpecIr struct {
	timeout                    *durationpb.Duration
	AI                         *v1alpha1.AIRoutePolicy
	transform                  *anypb.Any
	rustformation              *anypb.Any
	rustformationStringToStash string
	errors                     []error
}

func (d *routePolicy) CreationTime() time.Time {
	return d.ct
}

func (d *routePolicy) Equals(in any) bool {
	d2, ok := in.(*routePolicy)
	if !ok {
		return false
	}

	if !proto.Equal(d.spec.timeout, d2.spec.timeout) {
		return false
	}
	if !proto.Equal(d.spec.transform, d2.spec.transform) {
		return false
	}
	if !proto.Equal(d.spec.rustformation, d2.spec.rustformation) {
		return false
	}

	return true
}

type routePolicyPluginGwPass struct {
	setTransformationInChain bool // TODO(nfuden): make this multi stage
	// TODO(nfuden): dont abuse httplevel filter in favor of route level
	rustformationStash map[string]string
	ir.UnimplementedProxyTranslationPass
	setAIFilter bool
}

func (p *routePolicyPluginGwPass) ApplyHCM(ctx context.Context, pCtx *ir.HcmContext, out *envoyhttp.HttpConnectionManager) error {
	// no op
	return nil
}

var useRustformations bool

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionplug.Plugin {
	useRustformations = commoncol.Settings.UseRustFormations // stash the state of the env setup for rustformation usage

	col := krtutil.SetupCollectionDynamic[v1alpha1.RoutePolicy](
		ctx,
		commoncol.Client,
		v1alpha1.SchemeGroupVersion.WithResource("routepolicies"),
		commoncol.KrtOpts.ToOptions("RoutePolicy")...,
	)
	gk := wellknown.RoutePolicyGVK.GroupKind()
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
func (p *routePolicyPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	policy, ok := pCtx.Policy.(*routePolicy)
	if !ok {
		return nil
	}
	if policy.spec.timeout != nil && outputRoute.GetRoute() != nil {
		outputRoute.GetRoute().Timeout = policy.spec.timeout
	}

	if policy.spec.transform != nil {
		if outputRoute.GetTypedPerFilterConfig() == nil {
			outputRoute.TypedPerFilterConfig = make(map[string]*anypb.Any)
		}
		if policy.spec.transform != nil {
			outputRoute.GetTypedPerFilterConfig()[transformationFilterNamePrefix] = policy.spec.transform
		}
		p.setTransformationInChain = true
	}
	if policy.spec.rustformation != nil {
		if outputRoute.GetTypedPerFilterConfig() == nil {
			outputRoute.TypedPerFilterConfig = make(map[string]*anypb.Any)
		}
		// TODO(nfuden): get back to this path once we have valid perroute
		// outputRoute.GetTypedPerFilterConfig()[rustformationFilterNamePrefix] = policy.spec.rustformation

		// Hack around not having route level.
		// Note this is really really bad and rather fragile due to listener draining behaviors
		routeHash := strconv.Itoa(int(utils.HashProto(outputRoute)))
		if p.rustformationStash == nil {
			p.rustformationStash = make(map[string]string)
		}
		// encode the configuration that would be route level and stash the serialized version in a map
		p.rustformationStash[routeHash] = string(policy.spec.rustformationStringToStash)

		// augment the dynamic metadata so that we can do our route hack
		// set_dynamic_metadata filter DOES NOT have a route level configuration
		// set_filter_state can be used but the dynamic modules cannot access it on the current version of envoy
		// therefore use the old transformation just for rustformation
		reqm := &transformationpb.RouteTransformations_RouteTransformation_RequestMatch{
			RequestTransformation: &transformationpb.Transformation{
				TransformationType: &transformationpb.Transformation_TransformationTemplate{
					TransformationTemplate: &transformationpb.TransformationTemplate{
						ParseBodyBehavior: transformationpb.TransformationTemplate_DontParse, // Default is to try for JSON... Its kinda nice but failure is bad...
						DynamicMetadataValues: []*transformationpb.TransformationTemplate_DynamicMetadataValue{
							&transformationpb.TransformationTemplate_DynamicMetadataValue{
								MetadataNamespace: "kgateway",
								Key:               "route",
								Value: &transformationpb.InjaTemplate{
									Text: routeHash,
								},
							},
						},
					},
				},
			},
		}

		setmetaTransform := &transformationpb.RouteTransformations{
			Transformations: []*transformationpb.RouteTransformations_RouteTransformation{
				{

					Match: &transformationpb.RouteTransformations_RouteTransformation_RequestMatch_{
						RequestMatch: reqm,
					},
				},
			},
		}
		outputRoute.GetTypedPerFilterConfig()["helper/perroute/transform"], _ = utils.MessageToAny(setmetaTransform)

		p.setTransformationInChain = true
	}

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

func (p *routePolicyPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	extprocSettingsProto := pCtx.GetTypedConfig(wellknown.AIExtProcFilterName)
	if extprocSettingsProto == nil {
		return nil
	}
	extprocSettings, ok := extprocSettingsProto.(*envoy_ext_proc_v3.ExtProcPerRoute)
	if !ok {
		// TODO: internal error
		return nil
	}

	rtPolicy, ok := policy.(*routePolicy)
	if !ok {
		return nil
	}

	err := p.processAIRoutePolicy(ctx, rtPolicy.spec.AI, pCtx, extprocSettings, rtPolicy.AISecret)
	if err != nil {
		// TODO: report error on status
		return err
	}

	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *routePolicyPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	filters := []plugins.StagedHttpFilter{}
	if p.setTransformationInChain {
		// TODO(nfuden): support stages such as early
		// first register classic
		filters = append(filters, plugins.MustNewStagedFilter(transformationFilterNamePrefix,
			&transformationpb.FilterTransformations{},
			plugins.BeforeStage(plugins.AcceptedStage)))

		// ---------------
		// | END CLASSIC |
		// ---------------

		// TODO(nfuden/yuvalk): how to do route level correctly probably contribute to dynamic module upstream
		// smash together configuration
		filterRouteHashConfig := map[string]string{}

		for k, v := range p.rustformationStash {
			fmt.Println("k", k, "v", v)
			filterRouteHashConfig[k] = v
		}

		filterConfig, _ := json.Marshal(filterRouteHashConfig)

		rustCfg := dynamicmodulesv3.DynamicModuleFilter{
			DynamicModuleConfig: &exteniondynamicmodulev3.DynamicModuleConfig{
				Name: "rust_module",
			},
			FilterName:   "http_simple_mutations",
			FilterConfig: fmt.Sprintf(`{"route_specific": %s}`, string(filterConfig)),
		}

		filters = append(filters, plugins.MustNewStagedFilter(rustformationFilterNamePrefix,
			&rustCfg,
			plugins.BeforeStage(plugins.AcceptedStage)))

		// filters = append(filters, plugins.MustNewStagedFilter(setFilterStateFilterName,
		// 	&set_filter_statev3.Config{}, plugins.AfterStage(plugins.FaultStage)))
		filters = append(filters, plugins.MustNewStagedFilter("helper/perroute/transform",
			&transformationpb.FilterTransformations{},
			plugins.AfterStage(plugins.FaultStage)))
	}
	if len(filters) == 0 {
		return nil, nil
	}
	return filters, nil
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
		policyIr := routePolicy{ct: policyCR.CreationTimestamp.Time}

		outSpec := routeSpecIr{}

		if policyCR.Spec.Timeout > 0 {
			outSpec.timeout = durationpb.New(time.Second * time.Duration(policyCR.Spec.Timeout))
		}

		// Pass along the AI spec as is
		outSpec.AI = policyCR.Spec.AI
		// Augment with AI secrets as needed
		policyIr.AISecret = aiSecretForSpec(ctx, secrets, krtctx, policyCR)

		// Apply transformation specific translation
		transformationForSpec(policyCR.Spec, &outSpec)

		for _, err := range outSpec.errors {
			contextutils.LoggerFrom(ctx).Error(policyCR.GetNamespace(), policyCR.GetName(), err)
		}
		policyIr.spec = outSpec

		return &policyIr
	}
}

// aiSecret checks for the presence of the OpenAI Moderation which may require a secret reference
// will log an error if the secret is needed but not found
func aiSecretForSpec(
	ctx context.Context, secrets *krtcollections.SecretIndex,
	krtctx krt.HandlerContext, policyCR *v1alpha1.RoutePolicy) *ir.Secret {
	if policyCR.Spec.AI == nil ||
		policyCR.Spec.AI.PromptGuard == nil ||
		policyCR.Spec.AI.PromptGuard.Request == nil ||
		policyCR.Spec.AI.PromptGuard.Request.Moderation == nil {
		return nil
	}

	secretRef := policyCR.Spec.AI.PromptGuard.Request.Moderation.OpenAIModeration.AuthToken.SecretRef
	if secretRef == nil {
		// no secret ref is set
		return nil
	}

	// Retrieve and assign the secret
	secret, err := pluginutils.GetSecretIr(secrets, krtctx, secretRef.Name, policyCR.GetNamespace())
	if err != nil {
		contextutils.LoggerFrom(ctx).Error(err)
		return nil
	}
	return secret
}

// transformationForSpec translates the transformation spec into and onto the IR policy
func transformationForSpec(spec v1alpha1.RoutePolicySpec, out *routeSpecIr) {
	var err error
	if !useRustformations {
		out.transform, err = toTransformFilterConfig(&spec.Transformation)
		if err != nil {
			out.errors = append(out.errors, err)
		}
		return
	}

	rustformation, toStash, err := torustformFilterConfig(&spec.Transformation)
	if err != nil {
		out.errors = append(out.errors, err)
	}
	out.rustformation = rustformation
	out.rustformationStringToStash = string(toStash)
}
