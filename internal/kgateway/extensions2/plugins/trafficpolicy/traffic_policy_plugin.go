package trafficpolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"time"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	exteniondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_csrf_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_wellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	// TODO(nfuden): remove once rustformations are able to be used in a production environment
	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

const (
	transformationFilterNamePrefix              = "transformation"
	extAuthGlobalDisableFilterName              = "global_disable/ext_auth"
	extAuthGlobalDisableFilterMetadataNamespace = "dev.kgateway.disable_ext_auth"
	extAuthGlobalDisableKey                     = "extauth_disable"
	rustformationFilterNamePrefix               = "dynamic_modules/simple_mutations"
	metadataRouteTransformation                 = "transformation/helper"
	extauthFilterNamePrefix                     = "ext_auth"
	localRateLimitFilterNamePrefix              = "ratelimit/local"
	localRateLimitStatPrefix                    = "http_local_rate_limiter"
	rateLimitFilterNamePrefix                   = "ratelimit"
)

var (
	logger = logging.New("plugin/trafficpolicy")

	// from envoy code:
	// If the field `config` is configured but is empty, we treat the filter is enabled
	// explicitly.
	// see: https://github.com/envoyproxy/envoy/blob/8ed93ef372f788456b708fc93a7e54e17a013aa7/source/common/router/config_impl.cc#L2552
	EnableFilterPerRoute = &routev3.FilterConfig{Config: &anypb.Any{}}
)

type TrafficPolicy struct {
	ct   time.Time
	spec trafficPolicySpecIr
}

type trafficPolicySpecIr struct {
	AI        *AIPolicyIR
	ExtProc   *ExtprocIR
	transform *transformationpb.RouteTransformations
	// rustformation is currently a *dynamicmodulesv3.DynamicModuleFilter, but can potentially change at some point
	// in the future so we use proto.Message here
	rustformation              proto.Message
	rustformationStringToStash string
	extAuth                    *extAuthIR
	localRateLimit             *localratelimitv3.LocalRateLimit
	rateLimit                  *GlobalRateLimitIR
	cors                       *CorsIR
	csrf                       *CsrfIR
	autoHostRewrite            *wrapperspb.BoolValue
	buffer                     *BufferIR
}

func (d *TrafficPolicy) CreationTime() time.Time {
	return d.ct
}

func (d *TrafficPolicy) Equals(in any) bool {
	d2, ok := in.(*TrafficPolicy)
	if !ok {
		return false
	}

	if d.ct != d2.ct {
		return false
	}
	if !proto.Equal(d.spec.transform, d2.spec.transform) {
		return false
	}
	if !proto.Equal(d.spec.rustformation, d2.spec.rustformation) {
		return false
	}

	// AI equality checks
	if d.spec.AI != nil && d2.spec.AI != nil {
		if d.spec.AI.AISecret != nil && d2.spec.AI.AISecret != nil && !d.spec.AI.AISecret.Equals(*d2.spec.AI.AISecret) {
			return false
		}
		if (d.spec.AI.AISecret != nil) != (d2.spec.AI.AISecret != nil) {
			return false
		}
		if !proto.Equal(d.spec.AI.Extproc, d2.spec.AI.Extproc) {
			return false
		}
		if !proto.Equal(d.spec.AI.Transformation, d2.spec.AI.Transformation) {
			return false
		}
	} else if d.spec.AI != d2.spec.AI {
		// If one of the AI IR values is nil and the other isn't, not equal
		return false
	}

	if !d.spec.extAuth.Equals(d2.spec.extAuth) {
		return false
	}

	if !d.spec.ExtProc.Equals(d2.spec.ExtProc) {
		return false
	}

	if !proto.Equal(d.spec.localRateLimit, d2.spec.localRateLimit) {
		return false
	}

	if !d.spec.rateLimit.Equals(d2.spec.rateLimit) {
		return false
	}

	if !d.spec.cors.Equals(d2.spec.cors) {
		return false
	}

	if !d.spec.csrf.Equals(d2.spec.csrf) {
		return false
	}

	if !proto.Equal(d.spec.autoHostRewrite, d2.spec.autoHostRewrite) {
		return false
	}

	if !d.spec.buffer.Equals(d2.spec.buffer) {
		return false
	}

	return true
}

type trafficPolicyPluginGwPass struct {
	reporter reports.Reporter
	ir.UnimplementedProxyTranslationPass

	setTransformationInChain map[string]bool // TODO(nfuden): make this multi stage
	// TODO(nfuden): dont abuse httplevel filter in favor of route level
	rustformationStash    map[string]string
	listenerTransform     *transformationpb.RouteTransformations
	localRateLimitInChain map[string]*localratelimitv3.LocalRateLimit
	extAuthPerProvider    ProviderNeededMap
	extProcPerProvider    ProviderNeededMap
	rateLimitPerProvider  ProviderNeededMap
	corsInChain           map[string]*corsv3.Cors
	csrfInChain           map[string]*envoy_csrf_v3.CsrfPolicy
	bufferInChain         map[string]*bufferv3.Buffer
}

var _ ir.ProxyTranslationPass = &trafficPolicyPluginGwPass{}

var useRustformations bool

func registerTypes(ourCli versioned.Interface) {
	skubeclient.Register[*v1alpha1.TrafficPolicy](
		wellknown.TrafficPolicyGVR,
		wellknown.TrafficPolicyGVK,
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().TrafficPolicies(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().TrafficPolicies(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes(commoncol.OurClient)

	useRustformations = commoncol.Settings.UseRustFormations // stash the state of the env setup for rustformation usage

	col := krt.WrapClient(kclient.NewFiltered[*v1alpha1.TrafficPolicy](
		commoncol.Client,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	), commoncol.KrtOpts.ToOptions("TrafficPolicy")...)
	gk := wellknown.TrafficPolicyGVK.GroupKind()

	translator := NewTrafficPolicyBuilder(ctx, commoncol)
	v := validator.New()

	// TrafficPolicy IR will have TypedConfig -> implement backendroute method to add prompt guard, etc.
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.TrafficPolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: policyCR.Namespace,
			Name:      policyCR.Name,
		}

		policyIR, errors := translator.Translate(krtctx, policyCR)
		if err := policyIR.Validate(ctx, v, commoncol.Settings.RouteReplacementMode); err != nil {
			logger.Error("validation failed", "policy", policyCR.Name, "error", err)
			errors = append(errors, err)
		}
		pol := &ir.PolicyWrapper{
			ObjectSource: objSrc,
			Policy:       policyCR,
			PolicyIR:     policyIR,
			TargetRefs:   pluginsdkutils.TargetRefsToPolicyRefsWithSectionName(policyCR.Spec.TargetRefs, policyCR.Spec.TargetSelectors),
			Errors:       errors,
		}
		return pol
	})

	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			wellknown.TrafficPolicyGVK.GroupKind(): {
				// AttachmentPoints: []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				NewGatewayTranslationPass: NewGatewayTranslationPass,
				Policies:                  policyCol,
				MergePolicies:             mergePolicies,
				GetPolicyStatus:           getPolicyStatusFn(commoncol.CrudClient),
				PatchPolicyStatus:         patchPolicyStatusFn(commoncol.CrudClient),
			},
		},
		ExtraHasSynced: translator.HasSynced,
	}
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx, reporter reports.Reporter) ir.ProxyTranslationPass {
	return &trafficPolicyPluginGwPass{
		reporter:                 reporter,
		setTransformationInChain: make(map[string]bool),
	}
}

func (p *TrafficPolicy) Name() string {
	return "trafficpolicies"
}

func (p *trafficPolicyPluginGwPass) ApplyRouteConfigPlugin(
	ctx context.Context,
	pCtx *ir.RouteConfigContext,
	out *routev3.RouteConfiguration,
) {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)
}

func (p *trafficPolicyPluginGwPass) ApplyVhostPlugin(
	ctx context.Context,
	pCtx *ir.VirtualHostContext,
	out *routev3.VirtualHost,
) {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)
}

// called 0 or more times
func (p *trafficPolicyPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *routev3.Route) error {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return nil
	}

	if policy.spec.rustformation != nil {
		// TODO(nfuden): get back to this path once we have valid perroute
		// pCtx.TypedFilterConfig.AddTypedConfig(rustformationFilterNamePrefix, policy.spec.rustformation)

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
							{
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
		pCtx.TypedFilterConfig.AddTypedConfig(metadataRouteTransformation, setmetaTransform)

		p.setTransformationInChain[pCtx.FilterChainName] = true
	}

	if policy.spec.AI != nil {
		var aiBackends []*v1alpha1.Backend
		// check if the backends selected by targetRef are all AI backends before applying the policy
		for _, backend := range pCtx.In.Backends {
			if backend.Backend.BackendObject == nil {
				// could be nil if not found or no ref grant
				continue
			}
			b, ok := backend.Backend.BackendObject.Obj.(*v1alpha1.Backend)
			if !ok {
				// AI policy cannot apply to kubernetes services
				// TODO(npolshak): Report this as a warning on status
				logger.Warn("AI Policy cannot apply to kubernetes services", "backend_name", backend.Backend.BackendObject.GetName())
				continue
			}
			if b.Spec.Type != v1alpha1.BackendTypeAI {
				// AI policy cannot apply to non-AI backends
				// TODO(npolshak): Report this as a warning on status
				logger.Warn("AI Policy cannot apply to non-AI backend", "backend_name", backend.Backend.BackendObject.GetName(), "backend_type", string(b.Spec.Type))
				continue
			}
			aiBackends = append(aiBackends, b)
		}
		if len(aiBackends) > 0 {
			// Apply the AI policy to the all AI backends
			p.processAITrafficPolicy(&pCtx.TypedFilterConfig, policy.spec.AI)
		}
	}

	if policy.spec.autoHostRewrite != nil && policy.spec.autoHostRewrite.GetValue() {
		if ra := outputRoute.GetRoute(); ra != nil {
			ra.HostRewriteSpecifier = &routev3.RouteAction_AutoHostRewrite{
				AutoHostRewrite: policy.spec.autoHostRewrite,
			}
		}
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)

	return nil
}

func (p *trafficPolicyPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	rtPolicy, ok := policy.(*TrafficPolicy)
	if !ok {
		return nil
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, rtPolicy.spec)

	if rtPolicy.spec.AI != nil && (rtPolicy.spec.AI.Transformation != nil || rtPolicy.spec.AI.Extproc != nil) {
		p.processAITrafficPolicy(&pCtx.TypedFilterConfig, rtPolicy.spec.AI)
	}

	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *trafficPolicyPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	filters := []plugins.StagedHttpFilter{}

	// Add Ext_proc filters for listener
	for providerName, provider := range p.extProcPerProvider.Providers[fcc.FilterChainName] {
		extProcFilter := provider.ExtProc
		if extProcFilter == nil {
			continue
		}

		// add the specific auth filter
		extProcName := extProcFilterName(providerName)
		stagedExtProcFilter := plugins.MustNewStagedFilter(extProcName,
			extProcFilter,
			plugins.AfterStage(plugins.WellKnownFilterStage(plugins.AuthZStage)))

		// handle the case where route level only should be fired
		stagedExtProcFilter.Filter.Disabled = true

		filters = append(filters, stagedExtProcFilter)
	}

	// register classic transforms
	if p.setTransformationInChain[fcc.FilterChainName] && !useRustformations {
		// TODO(nfuden): support stages such as early
		transformationCfg := transformationpb.FilterTransformations{}
		if p.listenerTransform != nil {
			convertClassicRouteToListener(&transformationCfg, p.listenerTransform)
		}
		filter := plugins.MustNewStagedFilter(transformationFilterNamePrefix,
			&transformationCfg,
			plugins.BeforeStage(plugins.AcceptedStage))
		filter.Filter.Disabled = true

		filters = append(filters, filter)
	}
	if p.setTransformationInChain[fcc.FilterChainName] && useRustformations {
		// ---------------
		// | END CLASSIC |
		// ---------------
		// TODO(nfuden/yuvalk): how to do route level correctly probably contribute to dynamic module upstream
		// smash together configuration
		filterRouteHashConfig := map[string]string{}
		topLevel, ok := p.rustformationStash[""]

		if topLevel == "" {
			topLevel = "}"
		} else {
			// toplevel is already formatted and at this point its quicker to rip off the { than it is so unmarshal and all}
			topLevel = "," + topLevel[1:]
		}
		if ok {
			delete(p.rustformationStash, "")
		}
		for k, v := range p.rustformationStash {
			filterRouteHashConfig[k] = v
		}

		filterConfig, _ := json.Marshal(filterRouteHashConfig)
		msg, _ := utils.MessageToAny(&wrapperspb.StringValue{
			Value: fmt.Sprintf(`{"route_specific": %s%s`, string(filterConfig), topLevel),
		})
		rustCfg := dynamicmodulesv3.DynamicModuleFilter{
			DynamicModuleConfig: &exteniondynamicmodulev3.DynamicModuleConfig{
				Name: "rust_module",
			},
			FilterName: "http_simple_mutations",

			// currently we use stringvalue but we should look at using the json variant as supported in upstream
			FilterConfig: msg,
		}

		filters = append(filters, plugins.MustNewStagedFilter(rustformationFilterNamePrefix,
			&rustCfg,
			plugins.BeforeStage(plugins.AcceptedStage)))

		// filters = append(filters, plugins.MustNewStagedFilter(setFilterStateFilterName,
		// 	&set_filter_statev3.Config{}, plugins.AfterStage(plugins.FaultStage)))
		filters = append(filters, plugins.MustNewStagedFilter(metadataRouteTransformation,
			&transformationpb.FilterTransformations{},
			plugins.AfterStage(plugins.FaultStage)))
	}

	// register the transformation work once
	if len(p.extAuthPerProvider.Providers[fcc.FilterChainName]) != 0 {
		// register the filter that sets metadata so that it can have overrides on the route level
		filters = AddDisableFilterIfNeeded(filters)
	}

	// Add Ext_authz filter for listener
	for providerName, provider := range p.extAuthPerProvider.Providers[fcc.FilterChainName] {
		extAuthFilter := provider.ExtAuth
		if extAuthFilter == nil {
			continue
		}

		// add the specific auth filter
		extauthName := extAuthFilterName(providerName)
		stagedExtAuthFilter := plugins.MustNewStagedFilter(extauthName,
			extAuthFilter,
			plugins.DuringStage(plugins.AuthZStage))

		stagedExtAuthFilter.Filter.Disabled = true

		filters = append(filters, stagedExtAuthFilter)
	}

	if p.localRateLimitInChain[fcc.FilterChainName] != nil {
		filter := plugins.MustNewStagedFilter(localRateLimitFilterNamePrefix,
			p.localRateLimitInChain[fcc.FilterChainName],
			plugins.BeforeStage(plugins.AcceptedStage))
		filter.Filter.Disabled = true
		filters = append(filters, filter)
	}

	// Add global rate limit filters from providers
	for providerName, provider := range p.rateLimitPerProvider.Providers[fcc.FilterChainName] {
		rateLimitFilter := provider.RateLimit
		if rateLimitFilter == nil {
			continue
		}

		// add the specific rate limit filter with a unique name
		rateLimitName := getRateLimitFilterName(providerName)
		stagedRateLimitFilter := plugins.MustNewStagedFilter(rateLimitName,
			rateLimitFilter,
			plugins.DuringStage(plugins.RateLimitStage))

		filters = append(filters, stagedRateLimitFilter)
	}

	// Add Cors filter to enable cors for the listener.
	// Requires the cors policy to be set as typed_per_filter_config.
	if p.corsInChain[fcc.FilterChainName] != nil {
		filter := plugins.MustNewStagedFilter(envoy_wellknown.CORS,
			p.corsInChain[fcc.FilterChainName],
			plugins.DuringStage(plugins.CorsStage))
		filters = append(filters, filter)
	}

	// Add global CSRF http filter
	if p.csrfInChain[fcc.FilterChainName] != nil {
		filter := plugins.MustNewStagedFilter(csrfExtensionFilterName,
			p.csrfInChain[fcc.FilterChainName],
			plugins.DuringStage(plugins.RouteStage))
		filters = append(filters, filter)
	}

	// Add Buffer filter to enable buffer for the listener.
	// Requires the buffer policy to be set as typed_per_filter_config.
	if p.bufferInChain[fcc.FilterChainName] != nil {
		filter := plugins.MustNewStagedFilter(bufferFilterName,
			p.bufferInChain[fcc.FilterChainName],
			plugins.DuringStage(plugins.RouteStage))
		filter.Filter.Disabled = true
		filters = append(filters, filter)
	}

	if len(filters) == 0 {
		return nil, nil
	}
	return filters, nil
}

func (p *trafficPolicyPluginGwPass) handlePolicies(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, spec trafficPolicySpecIr) {
	p.handleTransformation(fcn, typedFilterConfig, spec.transform)
	// Apply ExtAuthz configuration if present
	// ExtAuth does not allow for most information such as destination
	// to be set at the route level so we need to smuggle info upwards.
	p.handleExtAuth(fcn, typedFilterConfig, spec.extAuth)
	p.handleExtProc(fcn, typedFilterConfig, spec.ExtProc)
	// Apply rate limit configuration if present
	p.handleRateLimit(fcn, typedFilterConfig, spec.rateLimit)
	p.handleLocalRateLimit(fcn, typedFilterConfig, spec.localRateLimit)

	// Apply CORS configuration if present
	p.handleCors(fcn, typedFilterConfig, spec.cors)

	// Apply CSRF configuration if present
	p.handleCsrf(fcn, typedFilterConfig, spec.csrf)

	p.handleBuffer(fcn, typedFilterConfig, spec.buffer)
}

func (p *trafficPolicyPluginGwPass) SupportsPolicyMerge() bool {
	return true
}

// mergePolicies merges the given policy ordered from high to low priority (both hierarchically
// and within the same hierarchy) based on the constraints defined per PolicyAtt.
//
// It iterates policies in reverse order (low to high) to ensure higher priority policies can
// always use an OverridableMerge strategy to override lower priority ones. Iterating policies
// in the given priority order (high to low) requires more complex merging for delegated chains
// because policies anywhere in the chain may enable policy overrides for their children but we
// still need to ensure these children cannot override any policies set by their ancestors that
// are not marked as overridable, i.e., (r1,p1)-delegate->(r2,p2)-delegate->(r3,p3) where
// r=route p=policy needs to ensure p3 does not override p1 (assuming p1 does not enable overrides)
// even if p2 allows overrides. This is easier to guarantee by using an OverridableMerge strategy
// by merging in higher priority policies with different HierarchicalPriority.
func mergePolicies(policies []ir.PolicyAtt) ir.PolicyAtt {
	var out ir.PolicyAtt
	if len(policies) == 0 {
		return out
	}
	_, ok := policies[0].PolicyIr.(*TrafficPolicy)
	// ignore unknown types
	if !ok {
		return out
	}

	// base policy to merge into has an empty PolicyIr so it can always be merged into
	out = ir.PolicyAtt{
		GroupKind:    policies[0].GroupKind,
		PolicyRef:    policies[0].PolicyRef,
		MergeOrigins: map[string]*ir.AttachedPolicyRef{},
		PolicyIr:     &TrafficPolicy{},
	}
	merged := out.PolicyIr.(*TrafficPolicy)

	for i := len(policies) - 1; i >= 0; i-- {
		mergeOpts := policy.MergeOptions{
			Strategy: policy.OverridableMerge,
		}
		// If merging a policy lower in the hierarchy with a policy higher in the hierarchy AND
		// the policy higher in the hierarchy enables policy overrides, use an AugmentedMerge strategy
		// to preserve existing fields set by lower levels.
		// NOTE: the HierarchicalPriority check is necessary to prevent enabling override behavior among
		// policies in the same hierarchy, e.g., ExtensionRef vs TargetRef policy attached to the same route, as
		// DelegationInheritedPolicyPriorityPreferChild strictly applies to parent->child policy inheritance and is not applicable
		// outside delegated policy inheritance.
		if out.HierarchicalPriority < policies[i].HierarchicalPriority && policies[i].DelegationInheritedPolicyPriority == apiannotations.DelegationInheritedPolicyPriorityPreferChild {
			mergeOpts.Strategy = policy.AugmentedMerge
		}

		p2 := policies[i].PolicyIr.(*TrafficPolicy)
		p2Ref := policies[i].PolicyRef

		mergeOrigins := MergeTrafficPolicies(merged, p2, p2Ref, mergeOpts)
		maps.Copy(out.MergeOrigins, mergeOrigins)
		out.HierarchicalPriority = policies[i].HierarchicalPriority
		out.Errors = append(out.Errors, policies[i].Errors...)
	}

	return out
}

// MergeTrafficPolicies merges two TrafficPolicy IRs, returning a map that contains information
// about the origin policy reference for each merged field.
func MergeTrafficPolicies(
	p1, p2 *TrafficPolicy,
	p2Ref *ir.AttachedPolicyRef,
	mergeOpts policy.MergeOptions,
) map[string]*ir.AttachedPolicyRef {
	if p1 == nil || p2 == nil {
		return nil
	}
	mergeOrigins := make(map[string]*ir.AttachedPolicyRef)
	if policy.IsMergeable(p1.spec.AI, p2.spec.AI, mergeOpts) {
		p1.spec.AI = p2.spec.AI
		mergeOrigins["ai"] = p2Ref
	}
	if policy.IsMergeable(p1.spec.ExtProc, p2.spec.ExtProc, mergeOpts) {
		p1.spec.ExtProc = p2.spec.ExtProc
		mergeOrigins["extProc"] = p2Ref
	}
	if policy.IsMergeable(p1.spec.transform, p2.spec.transform, mergeOpts) {
		p1.spec.transform = p2.spec.transform
		mergeOrigins["transformation"] = p2Ref
	}
	if policy.IsMergeable(p1.spec.rustformation, p2.spec.rustformation, mergeOpts) {
		p1.spec.rustformation = p2.spec.rustformation
		p1.spec.rustformationStringToStash = p2.spec.rustformationStringToStash
		mergeOrigins["rustformation"] = p2Ref
	}
	if policy.IsMergeable(p1.spec.extAuth, p2.spec.extAuth, mergeOpts) {
		p1.spec.extAuth = p2.spec.extAuth
		mergeOrigins["extAuth"] = p2Ref
	}
	if policy.IsMergeable(p1.spec.localRateLimit, p2.spec.localRateLimit, mergeOpts) {
		p1.spec.localRateLimit = p2.spec.localRateLimit
		mergeOrigins["rateLimit"] = p2Ref
	}
	// Handle global rate limit merging
	if policy.IsMergeable(p1.spec.rateLimit, p2.spec.rateLimit, mergeOpts) {
		p1.spec.rateLimit = p2.spec.rateLimit
		mergeOrigins["rateLimit"] = p2Ref
	}
	// Handle cors merging
	if policy.IsMergeable(p1.spec.cors, p2.spec.cors, mergeOpts) {
		p1.spec.cors = p2.spec.cors
		mergeOrigins["cors"] = p2Ref
	}

	// Handle CSRF policy merging
	if policy.IsMergeable(p1.spec.csrf, p2.spec.csrf, mergeOpts) {
		p1.spec.csrf = p2.spec.csrf
		mergeOrigins["csrf"] = p2Ref
	}
	if policy.IsMergeable(p1.spec.autoHostRewrite, p2.spec.autoHostRewrite, mergeOpts) {
		p1.spec.autoHostRewrite = p2.spec.autoHostRewrite
		mergeOrigins["autoHostRewrite"] = p2Ref
	}

	// Handle buffer policy merging
	if policy.IsMergeable(p1.spec.buffer, p2.spec.buffer, mergeOpts) {
		p1.spec.buffer = p2.spec.buffer
		mergeOrigins["buffer"] = p2Ref
	}

	return mergeOrigins
}
