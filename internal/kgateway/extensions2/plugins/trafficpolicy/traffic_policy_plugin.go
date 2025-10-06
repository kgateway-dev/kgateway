package trafficpolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	exteniondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_csrf_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	envoy_wellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
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
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	sdkfilters "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	pluginsdkir "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

const (
	transformationFilterNamePrefix = "transformation"
	rustformationFilterNamePrefix  = "dynamic_modules/simple_mutations"
	metadataRouteTransformation    = "transformation/helper"
	localRateLimitFilterNamePrefix = "ratelimit/local"
	localRateLimitStatPrefix       = "http_local_rate_limiter"
	rateLimitFilterNamePrefix      = "ratelimit"
	rbacFilterNamePrefix           = "envoy.filters.http.rbac"
)

var (
	logger = logging.New("plugin/trafficpolicy")

	// from envoy code:
	// If the field `config` is configured but is empty, we treat the filter is enabled
	// explicitly.
	// see: https://github.com/envoyproxy/envoy/blob/8ed93ef372f788456b708fc93a7e54e17a013aa7/source/common/router/config_impl.cc#L2552
	EnableFilterPerRoute = &envoyroutev3.FilterConfig{Config: &anypb.Any{}}
)

// PolicySubIR documents the expected interface that all policy sub-IRs should implement.
type PolicySubIR interface {
	// Equals compares this policy with another policy
	Equals(other PolicySubIR) bool

	// Validate performs PGV validation on the policy
	Validate() error

	// TODO: Merge. Just awkward as we won't be using the actual method type.
}

type TrafficPolicy struct {
	ct   time.Time
	spec trafficPolicySpecIr
}

type trafficPolicySpecIr struct {
	ai              *aiPolicyIR
	buffer          *bufferIR
	extProc         *extprocIR
	transformation  *transformationIR
	rustformation   *rustformationIR
	extAuth         *extAuthIR
	localRateLimit  *localRateLimitIR
	globalRateLimit *globalRateLimitIR
	cors            *corsIR
	csrf            *csrfIR
	headerModifiers *headerModifiersIR
	autoHostRewrite *autoHostRewriteIR
	retry           *retryIR
	timeouts        *timeoutsIR
	rbac            *rbacIR
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

	if !d.spec.ai.Equals(d2.spec.ai) {
		return false
	}
	if !d.spec.transformation.Equals(d2.spec.transformation) {
		return false
	}
	if !d.spec.rustformation.Equals(d2.spec.rustformation) {
		return false
	}
	if !d.spec.extAuth.Equals(d2.spec.extAuth) {
		return false
	}
	if !d.spec.extProc.Equals(d2.spec.extProc) {
		return false
	}
	if !d.spec.localRateLimit.Equals(d2.spec.localRateLimit) {
		return false
	}
	if !d.spec.globalRateLimit.Equals(d2.spec.globalRateLimit) {
		return false
	}
	if !d.spec.cors.Equals(d2.spec.cors) {
		return false
	}
	if !d.spec.csrf.Equals(d2.spec.csrf) {
		return false
	}
	if !d.spec.headerModifiers.Equals(d2.spec.headerModifiers) {
		return false
	}
	if !d.spec.autoHostRewrite.Equals(d2.spec.autoHostRewrite) {
		return false
	}
	if !d.spec.buffer.Equals(d2.spec.buffer) {
		return false
	}
	if !d.spec.retry.Equals(d2.spec.retry) {
		return false
	}
	if !d.spec.timeouts.Equals(d2.spec.timeouts) {
		return false
	}
	if !d.spec.rbac.Equals(d2.spec.rbac) {
		return false
	}
	return true
}

// Validate performs PGV (protobuf-generated validation) validation by delegating
// to each policy sub-IR's Validate() method. This follows the exact same pattern as the Equals() method.
// PGV validation is always performed regardless of route replacement mode.
func (p *TrafficPolicy) Validate() error {
	var validators []func() error
	validators = append(validators, p.spec.ai.Validate)
	validators = append(validators, p.spec.transformation.Validate)
	validators = append(validators, p.spec.rustformation.Validate)
	validators = append(validators, p.spec.localRateLimit.Validate)
	validators = append(validators, p.spec.globalRateLimit.Validate)
	validators = append(validators, p.spec.extProc.Validate)
	validators = append(validators, p.spec.extAuth.Validate)
	validators = append(validators, p.spec.csrf.Validate)
	validators = append(validators, p.spec.cors.Validate)
	validators = append(validators, p.spec.headerModifiers.Validate)
	validators = append(validators, p.spec.buffer.Validate)
	validators = append(validators, p.spec.autoHostRewrite.Validate)
	validators = append(validators, p.spec.rbac.Validate)
	for _, validator := range validators {
		if err := validator(); err != nil {
			return err
		}
	}
	return nil
}

type trafficPolicyPluginGwPass struct {
	reporter reporter.Reporter
	ir.UnimplementedProxyTranslationPass

	setTransformationInChain map[string]bool // TODO(nfuden): make this multi stage
	// TODO(nfuden): dont abuse httplevel filter in favor of route level
	rustformationStash    map[string]string
	listenerTransform     *transformationpb.RouteTransformations
	localRateLimitInChain map[string]*localratelimitv3.LocalRateLimit
	extAuthPerProvider    ProviderNeededMap
	extProcPerProvider    ProviderNeededMap
	rateLimitPerProvider  ProviderNeededMap
	rbacInChain           map[string]*envoyrbacv3.RBAC
	corsInChain           map[string]*corsv3.Cors
	csrfInChain           map[string]*envoy_csrf_v3.CsrfPolicy
	headerMutationInChain map[string]*header_mutationv3.HeaderMutationPerRoute
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

func NewPlugin(ctx context.Context, commoncol *collections.CommonCollections, mergeSettings string, v validator.Validator) sdk.Plugin {
	registerTypes(commoncol.OurClient)

	useRustformations = commoncol.Settings.UseRustFormations // stash the state of the env setup for rustformation usage

	col := krt.WrapClient(kclient.NewFiltered[*v1alpha1.TrafficPolicy](
		commoncol.Client,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	), commoncol.KrtOpts.ToOptions("TrafficPolicy")...)
	gk := wellknown.TrafficPolicyGVK.GroupKind()

	constructor := NewTrafficPolicyConstructor(ctx, commoncol)

	// TrafficPolicy IR will have TypedConfig -> implement backendroute method to add prompt guard, etc.
	policyCol := krt.NewCollection(col, func(krtctx krt.HandlerContext, policyCR *v1alpha1.TrafficPolicy) *ir.PolicyWrapper {
		objSrc := ir.ObjectSource{
			Group:     gk.Group,
			Kind:      gk.Kind,
			Namespace: policyCR.Namespace,
			Name:      policyCR.Name,
		}

		policyIR, errors := constructor.ConstructIR(krtctx, policyCR)
		if err := validateWithValidationLevel(ctx, policyIR, v, commoncol.Settings.ValidationMode); err != nil {
			logger.Error("validation failed", "policy", policyCR.Name, "error", err)
			errors = append(errors, err)
		}
		precedenceWeight, err := pluginsdkutils.ParsePrecedenceWeightAnnotation(policyCR.Annotations, apiannotations.PolicyPrecedenceWeight)
		if err != nil {
			errors = append(errors, err)
		}

		pol := &ir.PolicyWrapper{
			ObjectSource:     objSrc,
			Policy:           policyCR,
			PolicyIR:         policyIR,
			TargetRefs:       pluginsdkutils.TargetRefsToPolicyRefsWithSectionName(policyCR.Spec.TargetRefs, policyCR.Spec.TargetSelectors),
			Errors:           errors,
			PrecedenceWeight: precedenceWeight,
		}
		return pol
	})

	return sdk.Plugin{
		ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			wellknown.TrafficPolicyGVK.GroupKind(): {
				NewGatewayTranslationPass: NewGatewayTranslationPass,
				Policies:                  policyCol,
				MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
					return policy.MergePolicies(pols, mergeTrafficPolicies, mergeSettings)
				},
				GetPolicyStatus:   getPolicyStatusFn(commoncol.CrudClient),
				PatchPolicyStatus: patchPolicyStatusFn(commoncol.CrudClient),
			},
		},
		ExtraHasSynced: constructor.HasSynced,
	}
}

func NewGatewayTranslationPass(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass {
	return &trafficPolicyPluginGwPass{
		reporter:                 reporter,
		setTransformationInChain: make(map[string]bool),
	}
}

func (p *TrafficPolicy) Name() string {
	return "trafficpolicies"
}

func (p *trafficPolicyPluginGwPass) ApplyRouteConfigPlugin(
	pCtx *pluginsdkir.RouteConfigContext,
	out *envoyroutev3.RouteConfiguration,
) {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)
}

func (p *trafficPolicyPluginGwPass) ApplyVhostPlugin(
	pCtx *pluginsdkir.VirtualHostContext,
	out *envoyroutev3.VirtualHost,
) {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return
	}

	p.handlePerVHostPolicies(policy.spec, out)
	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)
}

// called 0 or more times
func (p *trafficPolicyPluginGwPass) ApplyForRoute(pCtx *ir.RouteContext, outputRoute *envoyroutev3.Route) error {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return nil
	}

	if policy.spec.rustformation != nil {
		// TODO(nfuden): get back to this path once we have valid perroute
		// pCtx.TypedFilterConfig.AddTypedConfig(rustformationFilterNamePrefix, policy.spec.rustformation)

		// Hack around not having route level.
		// Note this is really really bad and rather fragile due to listener draining behaviors
		routeHash := strconv.Itoa(int(utils.HashProto(outputRoute))) //nolint:gosec // G115: hash value used as string key, truncation is acceptable
		if p.rustformationStash == nil {
			p.rustformationStash = make(map[string]string)
		}
		// encode the configuration that would be route level and stash the serialized version in a map
		p.rustformationStash[routeHash] = string(policy.spec.rustformation.toStash)

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

	if policy.spec.ai != nil {
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
			p.processAITrafficPolicy(&pCtx.TypedFilterConfig, policy.spec.ai)
		}
	}

	p.handlePerRoutePolicies(policy.spec, outputRoute)
	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)

	return nil
}

func (p *trafficPolicyPluginGwPass) ApplyForRouteBackend(
	policy pluginsdkir.PolicyIR,
	pCtx *pluginsdkir.RouteBackendContext,
) error {
	rtPolicy, ok := policy.(*TrafficPolicy)
	if !ok {
		return nil
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, rtPolicy.spec)

	if rtPolicy.spec.ai != nil && (rtPolicy.spec.ai.Transformation != nil || rtPolicy.spec.ai.Extproc != nil) {
		p.processAITrafficPolicy(&pCtx.TypedFilterConfig, rtPolicy.spec.ai)
	}

	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *trafficPolicyPluginGwPass) HttpFilters(fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	filters := []plugins.StagedHttpFilter{}

	// Add global ExtProc disable filter when there are providers
	if len(p.extProcPerProvider.Providers[fcc.FilterChainName]) > 0 {
		// register the filter that sets metadata so that it can have overrides on the route level
		filters = AddDisableFilterIfNeeded(filters, extProcGlobalDisableFilterName, extProcGlobalDisableFilterMetadataNamespace)
	}
	// Add ExtProc filters for listener
	for _, provider := range p.extProcPerProvider.Providers[fcc.FilterChainName] {
		extProcFilter := provider.Extension.ExtProc
		if extProcFilter == nil {
			continue
		}

		// add the specific auth filter
		extProcName := extProcFilterName(provider.Name)
		stagedExtProcFilter := sdkfilters.MustNewStagedFilterWithWeight(extProcName,
			extProcFilter,
			plugins.AfterStage(plugins.WellKnownFilterStage(plugins.AuthZStage)),
			provider.Extension.PrecedenceWeight,
		)

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
		filter := sdkfilters.MustNewStagedFilter(transformationFilterNamePrefix,
			&transformationCfg,
			plugins.BeforeStage(plugins.AcceptedStage),
		)
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

		filters = append(filters, sdkfilters.MustNewStagedFilter(rustformationFilterNamePrefix,
			&rustCfg,
			plugins.BeforeStage(plugins.AcceptedStage),
		))

		// filters = append(filters, sdkfilters.MustNewStagedFilter(setFilterStateFilterName,
		// 	&set_filter_statev3.Config{}, plugins.AfterStage(plugins.FaultStage)))
		filters = append(filters, sdkfilters.MustNewStagedFilter(metadataRouteTransformation,
			&transformationpb.FilterTransformations{},
			plugins.AfterStage(plugins.FaultStage),
		))
	}

	// Add global ExtAuth disable filter when there are providers
	if len(p.extAuthPerProvider.Providers[fcc.FilterChainName]) > 0 {
		// register the filter that sets metadata so that it can have overrides on the route level
		filters = AddDisableFilterIfNeeded(filters, ExtAuthGlobalDisableFilterName, ExtAuthGlobalDisableFilterMetadataNamespace)
	}
	// Add Ext_authz filter for listener
	for _, provider := range p.extAuthPerProvider.Providers[fcc.FilterChainName] {
		extAuthFilter := provider.Extension.ExtAuth
		if extAuthFilter == nil {
			continue
		}

		// add the specific auth filter
		// Note that although this configures the "envoy.filters.http.ext_authz" filter, we still want
		// the ordering to be during the AuthNStage because we are using this filter for authentication
		// purposes
		extauthName := extAuthFilterName(provider.Name)
		stagedExtAuthFilter := sdkfilters.MustNewStagedFilterWithWeight(extauthName,
			extAuthFilter,
			plugins.DuringStage(plugins.AuthNStage),
			provider.Extension.PrecedenceWeight,
		)

		stagedExtAuthFilter.Filter.Disabled = true
		filters = append(filters, stagedExtAuthFilter)
	}

	if f := p.localRateLimitInChain[fcc.FilterChainName]; f != nil {
		filter := sdkfilters.MustNewStagedFilter(localRateLimitFilterNamePrefix, f, plugins.BeforeStage(plugins.AcceptedStage))
		filter.Filter.Disabled = true
		filters = append(filters, filter)
	}

	// Add global rate limit filters from providers
	for _, provider := range p.rateLimitPerProvider.Providers[fcc.FilterChainName] {
		rateLimitFilter := provider.Extension.RateLimit
		if rateLimitFilter == nil {
			continue
		}

		// add the specific rate limit filter with a unique name
		rateLimitName := getRateLimitFilterName(provider.Name)
		stagedRateLimitFilter := sdkfilters.MustNewStagedFilter(rateLimitName,
			rateLimitFilter,
			plugins.DuringStage(plugins.RateLimitStage),
		)
		stagedRateLimitFilter.Filter.Disabled = true
		filters = append(filters, stagedRateLimitFilter)
	}

	// Add Cors filter to enable cors for the listener.
	// Requires the cors policy to be set as typed_per_filter_config.
	if f := p.corsInChain[fcc.FilterChainName]; f != nil {
		filter := sdkfilters.MustNewStagedFilter(envoy_wellknown.CORS, f, plugins.DuringStage(plugins.CorsStage))
		filter.Filter.Disabled = true
		filters = append(filters, filter)
	}

	// Add global CSRF http filter
	if f := p.csrfInChain[fcc.FilterChainName]; f != nil {
		filter := sdkfilters.MustNewStagedFilter(csrfExtensionFilterName, f, plugins.DuringStage(plugins.RouteStage))
		filter.Filter.Disabled = true
		filters = append(filters, filter)
	}

	// Add header mutation filter.
	if f := p.headerMutationInChain[fcc.FilterChainName]; f != nil {
		filter := plugins.MustNewStagedFilter(headerMutationFilterName, f, plugins.DuringStage(plugins.RouteStage))
		filter.Filter.Disabled = true
		filters = append(filters, filter)
	}

	// Add Buffer filter to enable buffer for the listener.
	// Requires the buffer policy to be set as typed_per_filter_config.
	if f := p.bufferInChain[fcc.FilterChainName]; f != nil {
		filter := sdkfilters.MustNewStagedFilter(bufferFilterName, f, plugins.DuringStage(plugins.RouteStage))
		filter.Filter.Disabled = true
		filters = append(filters, filter)
	}

	if f := p.rbacInChain[fcc.FilterChainName]; f != nil {
		filter := plugins.MustNewStagedFilter(rbacFilterNamePrefix, f, plugins.DuringStage(plugins.AuthZStage))
		filters = append(filters, filter)
	}

	if len(filters) == 0 {
		return nil, nil
	}
	return filters, nil
}

// handlePolicies handles policies that are meant to be processed with the different
// ProxyTranslationPass Apply* methods
func (p *trafficPolicyPluginGwPass) handlePolicies(
	fcn string,
	typedFilterConfig *ir.TypedFilterConfigMap,
	spec trafficPolicySpecIr,
) {
	p.handleTransformation(fcn, typedFilterConfig, spec.transformation)
	// Apply ExtAuthz configuration if present
	// ExtAuth does not allow for most information such as destination
	// to be set at the route level so we need to smuggle info upwards.
	p.handleExtAuth(fcn, typedFilterConfig, spec.extAuth)
	p.handleExtProc(fcn, typedFilterConfig, spec.extProc)
	p.handleGlobalRateLimit(fcn, typedFilterConfig, spec.globalRateLimit)
	p.handleLocalRateLimit(fcn, typedFilterConfig, spec.localRateLimit)
	p.handleCors(fcn, typedFilterConfig, spec.cors)
	p.handleCsrf(fcn, typedFilterConfig, spec.csrf)
	p.handleHeaderModifiers(fcn, typedFilterConfig, spec.headerModifiers)
	p.handleBuffer(fcn, typedFilterConfig, spec.buffer)
	p.handleRBAC(fcn, typedFilterConfig, spec.rbac)
}

// handlePerRoutePolicies handles policies that are meant to be processed at the route level
func (p *trafficPolicyPluginGwPass) handlePerRoutePolicies(
	spec trafficPolicySpecIr,
	out *envoyroutev3.Route,
) {
	// A parent route rule with a delegated backend will not have RouteAction set
	if out.GetAction() == nil {
		return
	}

	action := out.GetRoute()

	if spec.autoHostRewrite != nil && spec.autoHostRewrite.enabled != nil && spec.autoHostRewrite.enabled.GetValue() {
		// Only apply TrafficPolicy's AutoHostRewrite if built-in policy's AutoHostRewrite is not already set
		if action.GetHostRewriteSpecifier() == nil {
			action.HostRewriteSpecifier = &envoyroutev3.RouteAction_AutoHostRewrite{
				AutoHostRewrite: spec.autoHostRewrite.enabled,
			}
		}
	}

	if spec.timeouts != nil {
		action.IdleTimeout = spec.timeouts.routeStreamIdleTimeout
		// Only set the route timeout if it is not already set, which implies that it was
		// set by the builtin HTTPRouteTimeouts policy
		if action.GetTimeout() == nil {
			action.Timeout = spec.timeouts.routeTimeout
		}
	}

	// Only set the retry policy if it is not already set, which implies that it was
	// set by the builtin HTTPRouteRetry policy
	if action.GetRetryPolicy() == nil && spec.retry != nil {
		action.RetryPolicy = spec.retry.policy
	}
}

// handlePerVHostPolicies handles policies that are meant to be processed at the vhost level
func (p *trafficPolicyPluginGwPass) handlePerVHostPolicies(
	spec trafficPolicySpecIr,
	out *envoyroutev3.VirtualHost,
) {
	if spec.retry != nil {
		out.RetryPolicy = spec.retry.policy
	}
}

func (p *trafficPolicyPluginGwPass) SupportsPolicyMerge() bool {
	return true
}
