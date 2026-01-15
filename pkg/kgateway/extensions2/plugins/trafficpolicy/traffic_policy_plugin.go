package trafficpolicy

import (
	"context"
	"time"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	exteniondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	envoy_api_key_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/api_key_auth/v3"
	envoy_basic_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/basic_auth/v3"
	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	compressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_csrf_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	decompressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/decompressor/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_wellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	// TODO(nfuden): remove once rustformations are able to be used in a production environment
	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

const (
	transformationFilterNamePrefix = "transformation"
	rustformationFilterNamePrefix  = "dynamic_modules/simple_mutations"
	localRateLimitFilterNamePrefix = "ratelimit/local"
	localRateLimitStatPrefix       = "http_local_rate_limiter"
	rateLimitFilterNamePrefix      = "ratelimit"
	rbacFilterNamePrefix           = "envoy.filters.http.rbac"
)

var logger = logging.New("plugin/trafficpolicy")

// from envoy code:
// If the field `config` is configured but is empty, we treat the filter is enabled
// explicitly.
// see: https://github.com/envoyproxy/envoy/blob/8ed93ef372f788456b708fc93a7e54e17a013aa7/source/common/router/config_impl.cc#L2552
func EnableFilterPerRoute() *envoyroutev3.FilterConfig {
	return &envoyroutev3.FilterConfig{Config: &anypb.Any{}}
}

func DisableFilterPerRoute() *envoyroutev3.FilterConfig {
	return &envoyroutev3.FilterConfig{Config: &anypb.Any{}, Disabled: true}
}

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
	jwt             *jwtIr
	compression     *compressionIR
	decompression   *decompressionIR
	basicAuth       *basicAuthIR
	urlRewrite      *urlRewriteIR
	apiKeyAuth      *apiKeyAuthIR
	oauth2          *oauthIR
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
	if !d.spec.jwt.Equals(d2.spec.jwt) {
		return false
	}
	if !d.spec.compression.Equals(d2.spec.compression) {
		return false
	}
	if !d.spec.decompression.Equals(d2.spec.decompression) {
		return false
	}
	if !d.spec.basicAuth.Equals(d2.spec.basicAuth) {
		return false
	}
	if !d.spec.urlRewrite.Equals(d2.spec.urlRewrite) {
		return false
	}
	if !d.spec.apiKeyAuth.Equals(d2.spec.apiKeyAuth) {
		return false
	}
	if !d.spec.oauth2.Equals(d2.spec.oauth2) {
		return false
	}
	return true
}

// Validate performs PGV (protobuf-generated validation) validation by delegating
// to each policy sub-IR's Validate() method. This follows the exact same pattern as the Equals() method.
// PGV validation is always performed regardless of route replacement mode.
func (p *TrafficPolicy) Validate() error {
	var validators []func() error
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
	validators = append(validators, p.spec.jwt.Validate)
	validators = append(validators, p.spec.compression.Validate)
	validators = append(validators, p.spec.decompression.Validate)
	validators = append(validators, p.spec.basicAuth.Validate)
	validators = append(validators, p.spec.urlRewrite.Validate)
	validators = append(validators, p.spec.apiKeyAuth.Validate)
	validators = append(validators, p.spec.oauth2.Validate)
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
	listenerTransform        *transformationpb.RouteTransformations
	localRateLimitInChain    map[string]*localratelimitv3.LocalRateLimit
	extAuthPerProvider       ProviderNeededMap
	extProcPerProvider       ProviderNeededMap
	jwtPerProvider           ProviderNeededMap
	rateLimitPerProvider     ProviderNeededMap
	oauth2PerProvider        ProviderNeededMap
	rbacInChain              map[string]*envoyrbacv3.RBAC
	corsInChain              map[string]*corsv3.Cors
	csrfInChain              map[string]*envoy_csrf_v3.CsrfPolicy
	headerMutationInChain    map[string]*header_mutationv3.HeaderMutationPerRoute
	bufferInChain            map[string]*bufferv3.Buffer
	compressorInChain        map[string]*compressorv3.Compressor
	decompressorInChain      map[string]*decompressorv3.Decompressor
	basicAuthInChain         map[string]*envoy_basic_auth_v3.BasicAuth
	apiKeyAuthInChain        map[string]*envoy_api_key_auth_v3.ApiKeyAuth
	// maps secret name to secret in case the same secret is referenced in multiple attachment points (e.g., vhost and route)
	secrets map[string]*envoytlsv3.Secret
}

var _ ir.ProxyTranslationPass = &trafficPolicyPluginGwPass{}

var useRustformations bool

func NewPlugin(ctx context.Context, commoncol *collections.CommonCollections, mergeSettings string, v validator.Validator) sdk.Plugin {
	useRustformations = commoncol.Settings.UseRustFormations // stash the state of the env setup for rustformation usage
	if useRustformations {
		logger.Info("transformation is using Rust Dynamic Module.")
	} else {
		logger.Warn("class transformation using envoy-gloo is being deprecated in v2.2 and will be removed in v2.3")
	}

	cli := kclient.NewFilteredDelayed[*kgateway.TrafficPolicy](
		commoncol.Client,
		wellknown.TrafficPolicyGVR,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	)
	col := krt.WrapClient(cli, commoncol.KrtOpts.ToOptions("TrafficPolicy")...)
	gk := wellknown.TrafficPolicyGVK.GroupKind()

	constructor := NewTrafficPolicyConstructor(ctx, commoncol)

	// TrafficPolicy IR will have TypedConfig -> implement backendroute method to add prompt guard, etc.
	statusCol, policyCol := krt.NewStatusCollection(col, func(krtctx krt.HandlerContext, policyCR *kgateway.TrafficPolicy) (*krtcollections.StatusMarker, *ir.PolicyWrapper) {
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

		var statusMarker *krtcollections.StatusMarker
		for _, ancestor := range policyCR.Status.Ancestors {
			if string(ancestor.ControllerName) == commoncol.ControllerName {
				statusMarker = &krtcollections.StatusMarker{}
				break
			}
		}

		pol := &ir.PolicyWrapper{
			ObjectSource:     objSrc,
			Policy:           policyCR,
			PolicyIR:         policyIR,
			TargetRefs:       pluginsdkutils.TargetRefsToPolicyRefsWithSectionName(policyCR.Spec.TargetRefs, policyCR.Spec.TargetSelectors),
			Errors:           errors,
			PrecedenceWeight: precedenceWeight,
		}
		return statusMarker, pol
	}, commoncol.KrtOpts.ToOptions("TrafficPolicyWrapper")...)

	// processMarkers for policies that have existing status but no current report
	processMarkers := func(kctx krt.HandlerContext, reportMap *reports.ReportMap) {
		objStatus := krt.Fetch(kctx, statusCol)
		for _, status := range objStatus {
			policyKey := reporter.PolicyKey{
				Group:     gk.Group,
				Kind:      gk.Kind,
				Namespace: status.Obj.GetNamespace(),
				Name:      status.Obj.GetName(),
			}

			// Add empty status to clear stale status for policies with no valid targets
			if reportMap.Policies[policyKey] == nil {
				rp := reports.NewReporter(reportMap)
				// create empty policy report entry with no ancestor refs
				rp.Policy(policyKey, 0)
			}
		}
	}

	return sdk.Plugin{
		ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			wellknown.TrafficPolicyGVK.GroupKind(): {
				NewGatewayTranslationPass:       NewGatewayTranslationPass,
				Policies:                        policyCol,
				ProcessPolicyStaleStatusMarkers: processMarkers,
				MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
					return policy.MergePolicies(pols, mergeTrafficPolicies, mergeSettings)
				},
				GetPolicyStatus:   getPolicyStatusFn(cli),
				PatchPolicyStatus: patchPolicyStatusFn(cli),
			},
		},
		ExtraHasSynced: constructor.HasSynced,
	}
}

func NewGatewayTranslationPass(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass {
	return &trafficPolicyPluginGwPass{
		reporter:                 reporter,
		setTransformationInChain: map[string]bool{},
		secrets:                  map[string]*envoytlsv3.Secret{},
	}
}

func (p *TrafficPolicy) Name() string {
	return "trafficpolicies"
}

func (p *trafficPolicyPluginGwPass) ApplyRouteConfigPlugin(
	pCtx *ir.RouteConfigContext,
	out *envoyroutev3.RouteConfiguration,
) {
	policy, ok := pCtx.Policy.(*TrafficPolicy)
	if !ok {
		return
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)
}

func (p *trafficPolicyPluginGwPass) ApplyVhostPlugin(
	pCtx *ir.VirtualHostContext,
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

	p.handlePerRoutePolicies(policy.spec, outputRoute)
	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, policy.spec)

	return nil
}

func (p *trafficPolicyPluginGwPass) ApplyForRouteBackend(
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	rtPolicy, ok := policy.(*TrafficPolicy)
	if !ok {
		return nil
	}

	p.handlePolicies(pCtx.FilterChainName, &pCtx.TypedFilterConfig, rtPolicy.spec)

	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *trafficPolicyPluginGwPass) HttpFilters(_ ir.HttpFiltersContext, fcc ir.FilterChainCommon) ([]filters.StagedHttpFilter, error) {
	stagedFilters := []filters.StagedHttpFilter{}

	// Add global ExtProc disable filter when there are providers
	if len(p.extProcPerProvider.Providers[fcc.FilterChainName]) > 0 {
		// register the filter that sets metadata so that it can have overrides on the route level
		stagedFilters = AddDisableFilterIfNeeded(stagedFilters, extProcGlobalDisableFilterName, extProcGlobalDisableFilterMetadataNamespace)
	}
	// Add ExtProc filters for listener
	for _, provider := range p.extProcPerProvider.Providers[fcc.FilterChainName] {
		extProcFilter := provider.Extension.ExtProc
		if extProcFilter == nil {
			continue
		}

		// add the specific auth filter
		extProcName := extProcFilterName(provider.Name)
		stagedExtProcFilter := filters.MustNewStagedFilterWithWeight(
			extProcName,
			extProcFilter,
			filters.AfterStage(filters.WellKnownFilterStage(filters.AuthZStage)),
			provider.Extension.PrecedenceWeight,
		)

		// handle the case where route level only should be fired
		stagedExtProcFilter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, stagedExtProcFilter)
	}

	// register classic transforms
	if p.setTransformationInChain[fcc.FilterChainName] && !useRustformations {
		// TODO(nfuden): support stages such as early
		transformationCfg := transformationpb.FilterTransformations{}
		if p.listenerTransform != nil {
			convertClassicRouteToListener(&transformationCfg, p.listenerTransform)
		}
		filter := filters.MustNewStagedFilter(transformationFilterNamePrefix,
			&transformationCfg,
			filters.BeforeStage(filters.AcceptedStage),
		)
		filter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, filter)
	}
	if p.setTransformationInChain[fcc.FilterChainName] && useRustformations {
		cfg, _ := utils.MessageToAny(&wrapperspb.StringValue{
			Value: "{}",
		})
		rustCfg := dynamicmodulesv3.DynamicModuleFilter{
			DynamicModuleConfig: &exteniondynamicmodulev3.DynamicModuleConfig{
				Name: "rust_module",
			},
			FilterName:   "http_simple_mutations",
			FilterConfig: cfg,
		}
		if p.listenerTransform != nil {
			// TODO: Add the listener level transform config here?
		}

		rustFilter := filters.MustNewStagedFilter(rustformationFilterNamePrefix,
			&rustCfg,
			filters.BeforeStage(filters.AcceptedStage),
		)
		rustFilter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, rustFilter)
	}

	// Add global ExtAuth disable filter when there are providers
	if len(p.extAuthPerProvider.Providers[fcc.FilterChainName]) > 0 {
		// register the filter that sets metadata so that it can have overrides on the route level
		stagedFilters = AddDisableFilterIfNeeded(stagedFilters, ExtAuthGlobalDisableFilterName, ExtAuthGlobalDisableFilterMetadataNamespace)
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
		stagedExtAuthFilter := filters.MustNewStagedFilterWithWeight(extauthName,
			extAuthFilter,
			filters.DuringStage(filters.AuthNStage),
			provider.Extension.PrecedenceWeight,
		)

		stagedExtAuthFilter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, stagedExtAuthFilter)
	}

	// Add OIDC filters for providers
	for _, provider := range p.oauth2PerProvider.Providers[fcc.FilterChainName] {
		oidcFilter := provider.Extension.OAuth2.cfg
		if oidcFilter == nil {
			continue
		}

		stagedFilter := filters.MustNewStagedFilterWithWeight(
			oauthFilterName(provider.Name),
			oidcFilter,
			// before JWT filter in AuthN stage which so that the ID token can be set by the OAuth2 filter that
			// the JWT filter can extract it from the cookies
			filters.BeforeStage(filters.AuthNStage),
			provider.Extension.PrecedenceWeight,
		)

		stagedFilter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, stagedFilter)
	}

	if len(p.jwtPerProvider.Providers[fcc.FilterChainName]) > 0 {
		stagedFilters = AddDisableFilterIfNeeded(stagedFilters, jwtGlobalDisableFilterName, jwtGlobalDisableFilterMetadataNamespace)
	}
	for _, provider := range p.jwtPerProvider.Providers[fcc.FilterChainName] {
		jwtFilter := provider.Extension.Jwt
		if jwtFilter == nil {
			continue
		}

		// add the specific jwt filter
		jwtName := jwtFilterName(provider.Name)
		stagedJwtFilter := filters.MustNewStagedFilter(
			jwtName,
			jwtFilter,
			filters.DuringStage(filters.AuthNStage),
		)

		stagedJwtFilter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, stagedJwtFilter)
	}

	if f := p.localRateLimitInChain[fcc.FilterChainName]; f != nil {
		filter := filters.MustNewStagedFilter(localRateLimitFilterNamePrefix, f, filters.DuringStage(filters.RateLimitStage))
		filter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, filter)
	}

	// Add global rate limit filters from providers
	for _, provider := range p.rateLimitPerProvider.Providers[fcc.FilterChainName] {
		rateLimitFilter := provider.Extension.RateLimit
		if rateLimitFilter == nil {
			continue
		}

		// add the specific rate limit filter with a unique name
		rateLimitName := getRateLimitFilterName(provider.Name)
		stagedRateLimitFilter := filters.MustNewStagedFilter(rateLimitName,
			rateLimitFilter,
			filters.DuringStage(filters.RateLimitStage),
		)
		stagedRateLimitFilter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, stagedRateLimitFilter)
	}

	// Add Cors filter to enable cors for the listener.
	// Requires the cors policy to be set as typed_per_filter_config.
	if f := p.corsInChain[fcc.FilterChainName]; f != nil {
		filter := filters.MustNewStagedFilter(envoy_wellknown.CORS, f, filters.DuringStage(filters.CorsStage))
		filter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, filter)
	}

	// Add global CSRF http filter
	if f := p.csrfInChain[fcc.FilterChainName]; f != nil {
		filter := filters.MustNewStagedFilter(csrfExtensionFilterName, f, filters.DuringStage(filters.RouteStage))
		filter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, filter)
	}

	// Add header mutation filter.
	if f := p.headerMutationInChain[fcc.FilterChainName]; f != nil {
		filter := filters.MustNewStagedFilter(headerMutationFilterName, f, filters.DuringStage(filters.RouteStage))
		filter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, filter)
	}

	// Add Buffer filter to enable buffer for the listener.
	// Requires the buffer policy to be set as typed_per_filter_config.
	if f := p.bufferInChain[fcc.FilterChainName]; f != nil {
		filter := filters.MustNewStagedFilter(bufferFilterName, f, filters.DuringStage(filters.RouteStage))
		filter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, filter)
	}

	if f := p.rbacInChain[fcc.FilterChainName]; f != nil {
		filter := filters.MustNewStagedFilter(rbacFilterNamePrefix, f, filters.DuringStage(filters.AuthZStage))
		stagedFilters = append(stagedFilters, filter)
	}

	// Add compression and decompression filters after CORS
	stagedFilters = addCompressionFiltersIfNeeded(stagedFilters, p, fcc.FilterChainName)
	// Add Basic Auth filter
	if f := p.basicAuthInChain[fcc.FilterChainName]; f != nil {
		filter := filters.MustNewStagedFilter(basicAuthFilterName, f, filters.DuringStage(filters.AuthNStage))
		filter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, filter)
	}

	// Add API key auth filter to the chain
	if f := p.apiKeyAuthInChain[fcc.FilterChainName]; f != nil {
		filter := filters.MustNewStagedFilter(apiKeyAuthFilterNamePrefix, f, filters.DuringStage(filters.AuthNStage))
		filter.Filter.Disabled = true
		stagedFilters = append(stagedFilters, filter)
	}

	if len(stagedFilters) == 0 {
		return nil, nil
	}

	return stagedFilters, nil
}

func (p *trafficPolicyPluginGwPass) ResourcesToAdd() ir.Resources {
	resources := ir.Resources{}
	for _, secret := range p.secrets {
		resources.Secrets = append(resources.Secrets, secret)
	}
	return resources
}

// handlePolicies handles policies that are meant to be processed with the different
// ProxyTranslationPass Apply* methods
func (p *trafficPolicyPluginGwPass) handlePolicies(
	fcn string,
	typedFilterConfig *ir.TypedFilterConfigMap,
	spec trafficPolicySpecIr,
) {
	if useRustformations {
		p.handleRustTransformation(fcn, typedFilterConfig, spec.rustformation)
	} else {
		p.handleTransformation(fcn, typedFilterConfig, spec.transformation)
	}

	// Apply ExtAuthz configuration if present
	// ExtAuth does not allow for most information such as destination
	// to be set at the route level so we need to smuggle info upwards.
	p.handleExtAuth(fcn, typedFilterConfig, spec.extAuth)
	p.handleExtProc(fcn, typedFilterConfig, spec.extProc)
	p.handleJwt(fcn, typedFilterConfig, spec.jwt)
	p.handleGlobalRateLimit(fcn, typedFilterConfig, spec.globalRateLimit)
	p.handleLocalRateLimit(fcn, typedFilterConfig, spec.localRateLimit)
	p.handleCors(fcn, typedFilterConfig, spec.cors)
	p.handleCsrf(fcn, typedFilterConfig, spec.csrf)
	p.handleHeaderModifiers(fcn, typedFilterConfig, spec.headerModifiers)
	p.handleBuffer(fcn, typedFilterConfig, spec.buffer)
	p.handleRBAC(fcn, typedFilterConfig, spec.rbac)
	p.handleCompression(fcn, typedFilterConfig, spec.compression)
	p.handleDecompression(fcn, typedFilterConfig, spec.decompression)
	p.handleBasicAuth(fcn, typedFilterConfig, spec.basicAuth)
	p.handleAPIKeyAuth(fcn, typedFilterConfig, spec.apiKeyAuth)
	p.handleOauth2(fcn, typedFilterConfig, spec.oauth2)
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

	// Apply URL rewrite configuration
	applyURLRewrite(spec.urlRewrite, out)
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
