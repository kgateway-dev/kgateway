package krtcollections

import (
	"context"
	"fmt"
	"strings"
	"time"

	stateful_sessionv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/stateful_session/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	stateful_cookie "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/cookie/v3"
	stateful_header "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/header/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/http/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	envoy_wellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	pluginsdkir "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const statefulSessionFilterName = "envoy.filters.http.stateful_session"

type applyToRoute interface {
	apply(outputRoute *envoy_config_route_v3.Route)
}

type applyToRouteBackend interface {
	applyToBackend(pCtx *ir.RouteBackendContext)
}

type timeouts struct {
	requestTimeout        *durationpb.Duration
	backendRequestTimeout *durationpb.Duration
}

type ruleIr struct {
	retry              *envoy_config_route_v3.RetryPolicy
	timeouts           timeouts
	sessionPersistence *anypb.Any
}

type filterIr struct {
	filterType gwv1.HTTPRouteFilterType

	policy applyToRoute
}

func (f *filterIr) apply(outputRoute *envoy_config_route_v3.Route) {
	if f.policy == nil {
		return
	}
	f.policy.apply(outputRoute)
}

type builtinPlugin struct {
	filter  *filterIr
	rule    ruleIr
	hasCors bool
}

func (d *builtinPlugin) CreationTime() time.Time {
	// should this be infinity?
	return time.Time{}
}

func (d *builtinPlugin) Equals(in any) bool {
	// we don't really need equality check here, because this policy is embedded in the httproute,
	// and we have generation based equality checks for that already.
	return true
}

type builtinPluginGwPass struct {
	ir.UnimplementedProxyTranslationPass
	reporter            reports.Reporter
	hasCorsPolicy       map[string]bool
	needStatefulSession map[string]bool
}

func (p *builtinPluginGwPass) ApplyForBackend(ctx context.Context, pCtx *ir.RouteBackendContext, in ir.HttpBackend, out *envoy_config_route_v3.Route) error {
	// no op
	return nil
}

func (p *builtinPluginGwPass) ApplyHCM(ctx context.Context, pCtx *ir.HcmContext, out *envoyhttp.HttpConnectionManager) error {
	// no-op
	return nil
}

func NewBuiltInIr(kctx krt.HandlerContext, f gwv1.HTTPRouteFilter, fromgk schema.GroupKind, fromns string, refgrants *RefGrantIndex, ups *BackendIndex) ir.PolicyIR {
	var cors *gwv1.HTTPCORSFilter
	if f.Type == gwv1.HTTPRouteFilterCORS {
		cors = f.CORS
	}

	return &builtinPlugin{
		hasCors: cors != nil,
		filter:  convertFilterIr(kctx, f, fromgk, fromns, refgrants, ups),
	}
}

func NewBuiltInRuleIr(rule gwv1.HTTPRouteRule) ir.PolicyIR {
	// If no rule policies are set, return nil so that we don't have a no-op policy
	if rule.Timeouts == nil && rule.Retry == nil && rule.SessionPersistence == nil {
		return nil
	}
	return &builtinPlugin{
		rule: convertRule(rule),
	}
}

func NewBuiltinPlugin(ctx context.Context) extensionsplug.Plugin {
	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			pluginsdkir.VirtualBuiltInGK: {
				// AttachmentPoints: []ir.AttachmentPoints{ir.HttpAttachmentPoint},
				NewGatewayTranslationPass: NewGatewayTranslationPass,
			},
		},
	}
}

func convertRule(rule gwv1.HTTPRouteRule) ruleIr {
	return ruleIr{
		retry:              convertRetry(rule.Retry, rule.Timeouts),
		timeouts:           convertTimeouts(rule.Timeouts),
		sessionPersistence: convertSessionPersistence(rule.SessionPersistence),
	}
}

func (r ruleIr) apply(outputRoute *envoy_config_route_v3.Route) error {
	// A parent route rule with a delegated backend will not have outputRoute.RouteAction set
	// but the plugin will be invoked on the rule, so treat this as a no-op call
	if outputRoute == nil || outputRoute.GetRoute() == nil {
		return nil
	}
	r.applyTimeouts(outputRoute, r.retry != nil)
	r.applyRetry(outputRoute)
	if r.sessionPersistence != nil {
		if outputRoute.GetTypedPerFilterConfig() == nil {
			outputRoute.TypedPerFilterConfig = map[string]*anypb.Any{}
		}
		outputRoute.GetTypedPerFilterConfig()[statefulSessionFilterName] = r.sessionPersistence
	}
	return nil
}

func convertTimeouts(timeout *gwv1.HTTPRouteTimeouts) timeouts {
	if timeout == nil {
		return timeouts{}
	}
	var requestTimeout *durationpb.Duration
	var backendRequestTimeout *durationpb.Duration

	if timeout.Request != nil {
		if parsed, err := time.ParseDuration(string(*timeout.Request)); err == nil {
			requestTimeout = durationpb.New(parsed)
		}
	}

	if timeout.BackendRequest != nil {
		if parsed, err := time.ParseDuration(string(*timeout.BackendRequest)); err == nil {
			backendRequestTimeout = durationpb.New(parsed)
		}
	}

	return timeouts{
		requestTimeout:        requestTimeout,
		backendRequestTimeout: backendRequestTimeout,
	}
}

func (r ruleIr) applyTimeouts(route *envoy_config_route_v3.Route, hasRetry bool) {
	timeouts := r.timeouts
	if timeouts.backendRequestTimeout == nil && timeouts.requestTimeout == nil {
		return
	}

	var timeout *durationpb.Duration
	// Apply the required timeout selection logic
	switch {
	case timeouts.backendRequestTimeout != nil && timeouts.requestTimeout != nil:
		// When both timeouts are set:
		// - Without retry: Use BackendRequest, since it's more specific (shorter)
		// - With retry: Use Request as the overall route timeout since
		//   BackendRequest will be applied to each retry attempt
		if hasRetry {
			timeout = timeouts.requestTimeout
		} else {
			timeout = timeouts.backendRequestTimeout
		}
	case timeouts.backendRequestTimeout != nil:
		// Only BackendRequest is set
		timeout = timeouts.backendRequestTimeout
	case timeouts.requestTimeout != nil:
		// Only Request is set
		timeout = timeouts.requestTimeout
	default:
		return
	}

	route.GetRoute().Timeout = timeout
}

func convertRetry(retry *gwv1.HTTPRouteRetry, timeout *gwv1.HTTPRouteTimeouts) *envoy_config_route_v3.RetryPolicy {
	if retry == nil {
		return nil
	}

	retryPolicy := &envoy_config_route_v3.RetryPolicy{
		NumRetries: &wrapperspb.UInt32Value{Value: 1},
		RetryOn:    "cancelled,connect-failure,refused-stream,retriable-headers,retriable-status-codes,unavailable",
	}

	if retry.Attempts != nil {
		retryPolicy.NumRetries = &wrapperspb.UInt32Value{Value: uint32(*retry.Attempts)}
	}

	if len(retry.Codes) > 0 {
		retryPolicy.RetriableStatusCodes = make([]uint32, len(retry.Codes))
		for i, c := range retry.Codes {
			retryPolicy.GetRetriableStatusCodes()[i] = uint32(c)
		}
	}

	if retry.Backoff != nil {
		backoff, err := time.ParseDuration(string(*retry.Backoff))
		if err != nil {
			// duration fields are cel validated, so this should never happen
			logger.Error("invalid HTTPRoute retry backoff", "backoff", string(*retry.Backoff), "error", err)
		} else {
			retryPolicy.RetryBackOff = &envoy_config_route_v3.RetryPolicy_RetryBackOff{
				BaseInterval: durationpb.New(backoff),
			}
		}
	}

	// If a backend request timeout is set, use it as the per-try timeout.
	// Otherwise, Envoy will by default use the global route timeout
	// Refer to https://gateway-api.sigs.k8s.io/geps/gep-1742/
	if timeout != nil && timeout.BackendRequest != nil {
		timeoutDuration, err := time.ParseDuration(string(*timeout.BackendRequest))
		if err != nil {
			// duration fields are cel validated, so this should never happen
			logger.Error("invalid HTTPRoute backend request timeout", "timeout", string(*timeout.BackendRequest), "error", err)
		} else {
			retryPolicy.PerTryTimeout = durationpb.New(timeoutDuration)
		}
	}

	return retryPolicy
}

func (r ruleIr) applyRetry(route *envoy_config_route_v3.Route) {
	if r.retry == nil {
		return
	}
	route.GetRoute().RetryPolicy = r.retry
}

func convertSessionPersistence(sessionPersistence *gwv1.SessionPersistence) *anypb.Any {
	if sessionPersistence == nil {
		return nil
	}

	// Handle session persistence if specified
	var sessionState proto.Message
	spType := gwv1.CookieBasedSessionPersistence
	if sessionPersistence.Type != nil {
		spType = *sessionPersistence.Type
	}

	switch spType {
	case gwv1.CookieBasedSessionPersistence:
		var ttl *durationpb.Duration
		if sessionPersistence.AbsoluteTimeout != nil {
			if parsed, err := time.ParseDuration(string(*sessionPersistence.AbsoluteTimeout)); err == nil {
				ttl = durationpb.New(parsed)
			}
		}
		cookie := &httpv3.Cookie{
			Name: utils.SanitizeCookieName(ptr.Deref(sessionPersistence.SessionName, "sessionPersistence")),
			Ttl:  ttl,
		}
		// Only set LifetimeType if present in CookieConfig
		if sessionPersistence.CookieConfig != nil &&
			sessionPersistence.CookieConfig.LifetimeType != nil {
			switch *sessionPersistence.CookieConfig.LifetimeType {
			case gwv1.SessionCookieLifetimeType:
				// Session cookies — cookies without a Max-Age or Expires attribute – are deleted when the current session ends
				cookie.Ttl = nil
			case gwv1.PermanentCookieLifetimeType:
				if cookie.GetTtl() == nil {
					cookie.Ttl = durationpb.New(time.Hour * 24 * 365)
				}
			}
		}
		sessionState = &stateful_cookie.CookieBasedSessionState{
			Cookie: cookie,
		}
	case gwv1.HeaderBasedSessionPersistence:
		sessionState = &stateful_header.HeaderBasedSessionState{
			Name: utils.SanitizeHeaderName(ptr.Deref(sessionPersistence.SessionName, "x-session-persistence")),
		}
	}
	sessionStateAny, err := utils.MessageToAny(sessionState)
	if err != nil {
		logger.Error("failed to create session state: %v", "error", err)
		return nil
	}
	statefulSession := &stateful_sessionv3.StatefulSession{
		SessionState: &envoy_config_core_v3.TypedExtensionConfig{
			Name:        "envoy.http.stateful_session." + strings.ToLower(string(spType)),
			TypedConfig: sessionStateAny,
		},
	}
	typedConfig, err := utils.MessageToAny(statefulSession)
	if err != nil {
		logger.Error("failed to create session state: %v", "error", err)
		return nil
	}
	return typedConfig
}

func translatePathRewrite(outputRoute *envoy_config_route_v3.RedirectAction, pathRewrite *gwv1.HTTPPathModifier) {
	if pathRewrite == nil {
		return
	}
	switch pathRewrite.Type {
	case gwv1.FullPathHTTPPathModifier:
		outputRoute.PathRewriteSpecifier = &envoy_config_route_v3.RedirectAction_PathRedirect{
			PathRedirect: ptr.Deref(pathRewrite.ReplaceFullPath, "/"),
		}
	case gwv1.PrefixMatchHTTPPathModifier:
		outputRoute.PathRewriteSpecifier = &envoy_config_route_v3.RedirectAction_PrefixRewrite{
			PrefixRewrite: ptr.Deref(pathRewrite.ReplacePrefixMatch, "/"),
		}
	}
}

func translateScheme(out *envoy_config_route_v3.RedirectAction, scheme *string) {
	if scheme == nil {
		return
	}

	if strings.ToLower(*scheme) == "https" {
		out.SchemeRewriteSpecifier = &envoy_config_route_v3.RedirectAction_HttpsRedirect{HttpsRedirect: true}
	} else {
		out.SchemeRewriteSpecifier = &envoy_config_route_v3.RedirectAction_SchemeRedirect{SchemeRedirect: *scheme}
	}
}

func translatePort(port *gwv1.PortNumber) uint32 {
	if port == nil {
		return 0
	}
	return uint32(*port)
}

func translateHostname(hostname *gwv1.PreciseHostname) string {
	if hostname == nil {
		return ""
	}
	return string(*hostname)
}

func translateStatusCode(i *int) envoy_config_route_v3.RedirectAction_RedirectResponseCode {
	if i == nil {
		return envoy_config_route_v3.RedirectAction_FOUND
	}

	switch *i {
	case 301:
		return envoy_config_route_v3.RedirectAction_MOVED_PERMANENTLY
	case 302:
		return envoy_config_route_v3.RedirectAction_FOUND
	case 303:
		return envoy_config_route_v3.RedirectAction_SEE_OTHER
	case 307:
		return envoy_config_route_v3.RedirectAction_TEMPORARY_REDIRECT
	case 308:
		return envoy_config_route_v3.RedirectAction_PERMANENT_REDIRECT
	default:
		return envoy_config_route_v3.RedirectAction_FOUND
	}
}

// MIRROR IR
// ===========
type mirrorIr struct {
	Cluster         string
	RuntimeFraction *envoy_config_core_v3.RuntimeFractionalPercent
}

func (m *mirrorIr) apply(outputRoute *envoy_config_route_v3.Route) {
	if outputRoute == nil || outputRoute.GetRoute() == nil {
		return
	}
	mirror := &envoy_config_route_v3.RouteAction_RequestMirrorPolicy{
		Cluster:         m.Cluster,
		RuntimeFraction: m.RuntimeFraction,
	}
	outputRoute.GetRoute().RequestMirrorPolicies = append(outputRoute.GetRoute().GetRequestMirrorPolicies(), mirror)
}

func convertMirrorIR(kctx krt.HandlerContext, f *gwv1.HTTPRequestMirrorFilter, fromgk schema.GroupKind, fromns string, refgrants *RefGrantIndex, ups *BackendIndex) *mirrorIr {
	if f == nil {
		return nil
	}
	to := toFromBackendRef(fromns, f.BackendRef)
	if !refgrants.ReferenceAllowed(kctx, fromgk, fromns, to) {
		return nil
	}
	up, err := ups.getBackendFromRef(kctx, fromns, f.BackendRef)
	if err != nil {
		return nil
	}
	fraction := getFractionPercent(*f)
	return &mirrorIr{
		Cluster:         up.ClusterName(),
		RuntimeFraction: fraction,
	}
}

// HEADER MODIFIER IR
// ==================
type headerModifierIr struct {
	Add       []*envoy_config_core_v3.HeaderValueOption
	Remove    []string
	IsRequest bool // true=request, false=response
}

func (h *headerModifierIr) apply(outputRoute *envoy_config_route_v3.Route) {
	if outputRoute == nil {
		return
	}
	if h.IsRequest {
		outputRoute.RequestHeadersToAdd = append(outputRoute.GetRequestHeadersToAdd(), h.Add...)
		outputRoute.RequestHeadersToRemove = append(outputRoute.GetRequestHeadersToRemove(), h.Remove...)
	} else {
		outputRoute.ResponseHeadersToAdd = append(outputRoute.GetResponseHeadersToAdd(), h.Add...)
		outputRoute.ResponseHeadersToRemove = append(outputRoute.GetResponseHeadersToRemove(), h.Remove...)
	}
}

func (h *headerModifierIr) applyToBackend(pCtx *ir.RouteBackendContext) {
	if h.IsRequest {
		pCtx.RequestHeadersToAdd = h.Add
		pCtx.RequestHeadersToRemove = h.Remove
	} else {
		pCtx.ResponseHeadersToAdd = h.Add
		pCtx.ResponseHeadersToRemove = h.Remove
	}
}
func convertHeaderModifierIR(kctx krt.HandlerContext, f *gwv1.HTTPHeaderFilter, isRequest bool) *headerModifierIr {
	if f == nil {
		return nil
	}
	var add []*envoy_config_core_v3.HeaderValueOption
	for _, h := range f.Add {
		add = append(add, &envoy_config_core_v3.HeaderValueOption{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   string(h.Name),
				Value: h.Value,
			},
			AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
		})
	}
	for _, h := range f.Set {
		add = append(add, &envoy_config_core_v3.HeaderValueOption{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   string(h.Name),
				Value: h.Value,
			},
			AppendAction: envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		})
	}
	return &headerModifierIr{
		Add:       add,
		Remove:    f.Remove,
		IsRequest: isRequest,
	}
}

func getFractionPercent(f gwv1.HTTPRequestMirrorFilter) *envoy_config_core_v3.RuntimeFractionalPercent {
	if f.Percent != nil {
		return &envoy_config_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoytype.FractionalPercent{
				Numerator:   uint32(*f.Percent),
				Denominator: envoytype.FractionalPercent_HUNDRED,
			},
		}
	}
	if f.Fraction != nil {
		denom := 100.0
		if f.Fraction.Denominator != nil {
			denom = float64(*f.Fraction.Denominator)
		}
		ratio := float64(f.Fraction.Numerator) / denom
		return &envoy_config_core_v3.RuntimeFractionalPercent{
			DefaultValue: toEnvoyPercentage(ratio),
		}
	}

	// nil means 100%
	return nil
}

func toEnvoyPercentage(percentage float64) *envoytype.FractionalPercent {
	return &envoytype.FractionalPercent{
		Numerator:   uint32(percentage * 10000),
		Denominator: envoytype.FractionalPercent_MILLION,
	}
}

func NewGatewayTranslationPass(ctx context.Context, tctx ir.GwTranslationCtx, reporter reports.Reporter) ir.ProxyTranslationPass {
	return &builtinPluginGwPass{
		reporter:            reporter,
		hasCorsPolicy:       make(map[string]bool),
		needStatefulSession: make(map[string]bool),
	}
}

func (p *builtinPlugin) Name() string {
	return "builtin"
}

// called one or more times per route rule
func (p *builtinPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	policy, ok := pCtx.Policy.(*builtinPlugin)
	if !ok {
		return nil
	}

	var errs error
	if policy.filter != nil {
		policy.filter.apply(outputRoute)
	}

	policy.rule.apply(outputRoute)
	if outputRoute.GetTypedPerFilterConfig()[statefulSessionFilterName] != nil {
		p.needStatefulSession[pCtx.FilterChainName] = true
	}

	if policy.hasCors {
		p.hasCorsPolicy[pCtx.FilterChainName] = true
	}

	return errs
}

func (p *builtinPluginGwPass) ApplyForRouteBackend(
	ctx context.Context,
	policy ir.PolicyIR,
	pCtx *ir.RouteBackendContext,
) error {
	inPolicy, ok := policy.(*builtinPlugin)
	if !ok {
		return nil
	}
	if inPolicy.filter == nil {
		return nil
	}
	if inPolicy.filter.policy == nil {
		return nil
	}

	if inPolicy.hasCors {
		p.hasCorsPolicy[pCtx.FilterChainName] = true
	}
	if backendPolicy, ok := inPolicy.filter.policy.(applyToRouteBackend); ok {
		backendPolicy.applyToBackend(pCtx)
	} else {
		logger.Error("filter policy is not supported on backendRef", "filter_type", inPolicy.filter.filterType)
		// TODO: once we have warnings / non terminal errors we should return it here, so the policy status is updated.
		return nil
	}

	return nil
}

func (p *builtinPluginGwPass) HttpFilters(ctx context.Context, fcc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	builtinStaged := []plugins.StagedHttpFilter{}

	// If there is a cors policy for route rule or backendRef, add the cors http filter to the chain
	if p.hasCorsPolicy[fcc.FilterChainName] {
		stagedFilter, err := plugins.NewStagedFilter(envoy_wellknown.CORS, &corsv3.Cors{}, plugins.DuringStage(plugins.CorsStage))
		if err != nil {
			return nil, err
		}
		builtinStaged = append(builtinStaged, stagedFilter)
	}

	if p.needStatefulSession[fcc.FilterChainName] {
		stagedFilter, err := plugins.NewStagedFilter(statefulSessionFilterName, &stateful_sessionv3.StatefulSession{}, plugins.DuringStage(plugins.AcceptedStage))
		if err != nil {
			return nil, err
		}
		stagedFilter.Filter.Disabled = true
		builtinStaged = append(builtinStaged, stagedFilter)
	}

	return builtinStaged, nil
}

func ToEnvoyCorsPolicy(f *gwv1.HTTPCORSFilter) *corsv3.CorsPolicy {
	if f == nil {
		return nil
	}
	corsPolicy := &corsv3.CorsPolicy{}
	if len(f.AllowOrigins) > 0 {
		origins := make([]*envoy_type_matcher_v3.StringMatcher, len(f.AllowOrigins))
		for i, origin := range f.AllowOrigins {
			origins[i] = &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
					Exact: string(origin),
				},
			}
		}
		corsPolicy.AllowOriginStringMatch = origins
	}
	if len(f.AllowMethods) > 0 {
		methods := make([]string, len(f.AllowMethods))
		for i, method := range f.AllowMethods {
			methods[i] = string(method)
		}
		corsPolicy.AllowMethods = strings.Join(methods, ", ")
	}
	if len(f.AllowHeaders) > 0 {
		headers := make([]string, len(f.AllowHeaders))
		for i, header := range f.AllowHeaders {
			headers[i] = string(header)
		}
		corsPolicy.AllowHeaders = strings.Join(headers, ", ")
	}
	if f.AllowCredentials {
		corsPolicy.AllowCredentials = &wrapperspb.BoolValue{Value: bool(f.AllowCredentials)}
	}
	if len(f.ExposeHeaders) > 0 {
		headers := make([]string, len(f.ExposeHeaders))
		for i, header := range f.ExposeHeaders {
			headers[i] = string(header)
		}
		corsPolicy.ExposeHeaders = strings.Join(headers, ", ")
	}
	if f.MaxAge != 0 {
		corsPolicy.MaxAge = fmt.Sprintf("%d", f.MaxAge)
	}
	return corsPolicy
}

// New helper to create filterIr
func convertFilterIr(kctx krt.HandlerContext, f gwv1.HTTPRouteFilter, fromgk schema.GroupKind, fromns string, refgrants *RefGrantIndex, ups *BackendIndex) *filterIr {
	var policy applyToRoute
	switch f.Type {
	case gwv1.HTTPRouteFilterRequestMirror:
		mir := convertMirrorIR(kctx, f.RequestMirror, fromgk, fromns, refgrants, ups)
		if mir != nil {
			policy = mir
		}
	case gwv1.HTTPRouteFilterRequestHeaderModifier:
		hm := convertHeaderModifierIR(kctx, f.RequestHeaderModifier, true)
		if hm != nil {
			policy = hm
		}
	case gwv1.HTTPRouteFilterResponseHeaderModifier:
		hm := convertHeaderModifierIR(kctx, f.ResponseHeaderModifier, false)
		if hm != nil {
			policy = hm
		}
	case gwv1.HTTPRouteFilterRequestRedirect:
		rr := convertRequestRedirectIR(kctx, f.RequestRedirect)
		if rr != nil {
			policy = rr
		}
	case gwv1.HTTPRouteFilterURLRewrite:
		uw := convertURLRewriteIR(kctx, f.URLRewrite)
		if uw != nil {
			policy = uw
		}
	case gwv1.HTTPRouteFilterCORS:
		ci := convertCORSIR(kctx, f.CORS)
		if ci != nil {
			policy = ci
		}
	}
	if policy == nil {
		return nil
	}
	return &filterIr{
		filterType: f.Type,
		policy:     policy,
	}
}

// REQUEST REDIRECT IR
// ===================
type requestRedirectIr struct {
	Redir *envoy_config_route_v3.RedirectAction
}

func (r *requestRedirectIr) apply(outputRoute *envoy_config_route_v3.Route) {
	if outputRoute == nil {
		return
	}
	outputRoute.Action = &envoy_config_route_v3.Route_Redirect{
		Redirect: r.Redir,
	}
}

func convertRequestRedirectIR(kctx krt.HandlerContext, config *gwv1.HTTPRequestRedirectFilter) *requestRedirectIr {
	if config == nil {
		return nil
	}
	redir := &envoy_config_route_v3.RedirectAction{
		HostRedirect: translateHostname(config.Hostname),
		ResponseCode: translateStatusCode(config.StatusCode),
		PortRedirect: translatePort(config.Port),
	}
	translateScheme(redir, config.Scheme)
	translatePathRewrite(redir, config.Path)
	return &requestRedirectIr{Redir: redir}
}

// URL REWRITE IR
// ==============
type urlRewriteIr struct {
	HostRewrite   *envoy_config_route_v3.RouteAction_HostRewriteLiteral
	FullReplace   string
	PrefixReplace string
}

func (u *urlRewriteIr) apply(outputRoute *envoy_config_route_v3.Route) {
	if outputRoute == nil || outputRoute.GetRoute() == nil {
		return
	}
	if u.HostRewrite != nil {
		outputRoute.GetRoute().HostRewriteSpecifier = u.HostRewrite
	}
	if u.FullReplace != "" {
		outputRoute.GetRoute().RegexRewrite = &envoy_type_matcher_v3.RegexMatchAndSubstitute{
			Pattern: &envoy_type_matcher_v3.RegexMatcher{
				EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{}},
				Regex:      ".*",
			},
			Substitution: u.FullReplace,
		}
	}
	if u.PrefixReplace != "" {
		path := outputRoute.GetMatch().GetPrefix()
		if path == "" {
			path = outputRoute.GetMatch().GetPath()
		}
		if path == "" {
			path = outputRoute.GetMatch().GetPathSeparatedPrefix()
		}
		if path != "" && u.PrefixReplace == "/" {
			outputRoute.GetRoute().RegexRewrite = &envoy_type_matcher_v3.RegexMatchAndSubstitute{
				Pattern: &envoy_type_matcher_v3.RegexMatcher{
					EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{}},
					Regex:      "^" + path + "\\/*",
				},
				Substitution: "/",
			}
		} else {
			outputRoute.GetRoute().PrefixRewrite = u.PrefixReplace
		}
	}
}

func convertURLRewriteIR(kctx krt.HandlerContext, config *gwv1.HTTPURLRewriteFilter) *urlRewriteIr {
	if config == nil {
		return nil
	}
	var hostrewrite *envoy_config_route_v3.RouteAction_HostRewriteLiteral
	if config.Hostname != nil {
		hostrewrite = &envoy_config_route_v3.RouteAction_HostRewriteLiteral{
			HostRewriteLiteral: string(*config.Hostname),
		}
	}
	var prefixReplace string
	var fullReplace string
	if config.Path != nil {
		switch config.Path.Type {
		case gwv1.FullPathHTTPPathModifier:
			fullReplace = ptr.Deref(config.Path.ReplaceFullPath, "/")
		case gwv1.PrefixMatchHTTPPathModifier:
			prefixReplace = ptr.Deref(config.Path.ReplacePrefixMatch, "/")
		}
	}
	return &urlRewriteIr{
		HostRewrite:   hostrewrite,
		FullReplace:   fullReplace,
		PrefixReplace: prefixReplace,
	}
}

// CORS IR
// ========
type corsIr struct {
	Cors *anypb.Any
}

func (c *corsIr) apply(outputRoute *envoy_config_route_v3.Route) {
	if c.Cors == nil {
		return
	}

	if outputRoute.GetTypedPerFilterConfig() == nil {
		outputRoute.TypedPerFilterConfig = make(map[string]*anypb.Any)
	}
	outputRoute.GetTypedPerFilterConfig()[envoy_wellknown.CORS] = c.Cors
}
func (c *corsIr) applyToBackend(pCtx *ir.RouteBackendContext) {
	if c.Cors == nil {
		return
	}
	pCtx.TypedFilterConfig[envoy_wellknown.CORS] = c.Cors
}

func convertCORSIR(_ krt.HandlerContext, f *gwv1.HTTPCORSFilter) *corsIr {
	if f == nil {
		return nil
	}
	corsPolicyAny, err := utils.MessageToAny(ToEnvoyCorsPolicy(f))
	if err != nil {
		// this should never happen.
		logger.Error("failed to convert CORS policy to Any", "error", err)
		return nil
	}
	return &corsIr{Cors: corsPolicyAny}
}
