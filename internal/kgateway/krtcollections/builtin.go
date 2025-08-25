package krtcollections

import (
	"context"
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
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	envoy_wellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	v1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	pluginsdkir "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

const statefulSessionFilterName = "envoy.filters.http.stateful_session"

type applyToRoute interface {
	// apply may be invoked multiple times on the route, once for each policy.
	// For delegated routes, policies attached to the parent route are inherited
	// and may override the current policy on the output route if MergeOptions allows it,
	// and hence the apply implementation must use policy.IsSettable(field, mergeOpts)
	// to check if the field on the output route can be set before being set.
	// Currently, the apply method is invoked in order of priority from highest(child route policies)
	// to lowest(parent route policies).
	apply(outputRoute *envoyroutev3.Route, mergeOpts policy.MergeOptions)
}

type applyToRouteBackend interface {
	applyToBackend(pCtx *ir.RouteBackendContext)
}

type timeouts struct {
	requestTimeout        *durationpb.Duration
	backendRequestTimeout *durationpb.Duration
}

type ruleIR struct {
	timeouts           *timeouts
	retry              *envoyroutev3.RetryPolicy
	sessionPersistence *stateful_sessionv3.StatefulSessionPerRoute
}

type filterIR struct {
	filterType gwv1.HTTPRouteFilterType

	policy applyToRoute
}

func (f *filterIR) apply(
	outputRoute *envoyroutev3.Route,
	mergeOpts policy.MergeOptions,
) {
	if f.policy == nil {
		return
	}
	f.policy.apply(outputRoute, mergeOpts)
}

type builtinPlugin struct {
	filter  *filterIR
	rule    ruleIR
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

func (p *builtinPluginGwPass) ApplyForBackend(ctx context.Context, pCtx *ir.RouteBackendContext, in ir.HttpBackend, out *envoyroutev3.Route) error {
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
		filter:  convertfilterIR(kctx, f, fromgk, fromns, refgrants, ups),
	}
}

func NewBuiltInRuleIr(rule gwv1.HTTPRouteRule) ir.PolicyIR {
	// If no rule policies are set, return nil so that we don't have a no-op policy
	if rule.Timeouts == nil && rule.Retry == nil && rule.SessionPersistence == nil {
		return nil
	}
	return &builtinPlugin{
		rule: buildHTTPRouteRulePolicy(rule),
	}
}

func NewBuiltinPlugin(ctx context.Context) extensionsplug.Plugin {
	return extensionsplug.Plugin{
		ContributesPolicies: map[schema.GroupKind]extensionsplug.PolicyPlugin{
			pluginsdkir.VirtualBuiltInGK: {
				NewGatewayTranslationPass: NewGatewayTranslationPass,
			},
		},
	}
}

func buildHTTPRouteRulePolicy(rule gwv1.HTTPRouteRule) ruleIR {
	return ruleIR{
		retry:              convertRetry(rule.Retry, rule.Timeouts),
		timeouts:           convertTimeouts(rule.Timeouts),
		sessionPersistence: convertSessionPersistence(rule.SessionPersistence),
	}
}

func (p *builtinPluginGwPass) applyRulePolicy(
	pCtx *ir.RouteContext,
	r ruleIR,
	mergeOpts policy.MergeOptions,
	outputRoute *envoyroutev3.Route,
) error {
	// A parent route rule with a delegated backend will not have outputRoute.RouteAction set
	// but the plugin will be invoked on the rule, so treat this as a no-op call
	if outputRoute == nil || outputRoute.GetRoute() == nil {
		return nil
	}
	r.applyTimeouts(outputRoute.GetRoute(), r.retry != nil, mergeOpts)
	r.applyRetry(outputRoute.GetRoute(), mergeOpts)

	if r.sessionPersistence != nil && policy.IsSettable(outputRoute.GetTypedPerFilterConfig()[statefulSessionFilterName], mergeOpts) {
		if outputRoute.GetTypedPerFilterConfig() == nil {
			outputRoute.TypedPerFilterConfig = map[string]*anypb.Any{}
		}
		anyMsg, err := utils.MessageToAny(r.sessionPersistence)
		if err != nil {
			logger.Error("error marshalling SessionPersistence", "error", err)
			return err
		}
		outputRoute.GetTypedPerFilterConfig()[statefulSessionFilterName] = anyMsg
		p.needStatefulSession[pCtx.FilterChainName] = true
	}
	return nil
}

func convertTimeouts(timeout *gwv1.HTTPRouteTimeouts) *timeouts {
	if timeout == nil {
		return nil
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

	return &timeouts{
		requestTimeout:        requestTimeout,
		backendRequestTimeout: backendRequestTimeout,
	}
}

func (r ruleIR) applyTimeouts(
	action *envoyroutev3.RouteAction,
	hasRetry bool,
	mergeOpts policy.MergeOptions,
) {
	timeouts := r.timeouts
	if timeouts == nil || timeouts.backendRequestTimeout == nil && timeouts.requestTimeout == nil ||
		!policy.IsSettable(action.GetTimeout(), mergeOpts) {
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

	action.Timeout = timeout
}

func convertRetry(
	retry *gwv1.HTTPRouteRetry,
	timeout *gwv1.HTTPRouteTimeouts,
) *envoyroutev3.RetryPolicy {
	if retry == nil {
		return nil
	}

	in := &v1alpha1.Retry{
		Attempts: 1,
		RetryOn: []v1alpha1.RetryOnCondition{
			"cancelled", "connect-failure", "refused-stream", "retriable-headers", "retriable-status-codes", "unavailable",
		},
		StatusCodes: retry.Codes,
	}

	if retry.Attempts != nil {
		in.Attempts = int32(*retry.Attempts)
	}

	if retry.Backoff != nil {
		duration, err := time.ParseDuration(string(*retry.Backoff))
		if err != nil {
			// duration fields are cel validated, so this should never happen
			logger.Error("invalid HTTPRoute retry backoff", "backoff", string(*retry.Backoff), "error", err)
		} else {
			in.BackoffBaseInterval = &metav1.Duration{Duration: duration}
		}
	}

	// If a backend request timeout is set, use it as the per-try timeout.
	// Otherwise, Envoy will by default use the global route timeout
	// Refer to https://gateway-api.sigs.k8s.io/geps/gep-1742/
	if timeout != nil && timeout.BackendRequest != nil {
		duration, err := time.ParseDuration(string(*timeout.BackendRequest))
		if err != nil {
			// duration fields are cel validated, so this should never happen
			logger.Error("invalid HTTPRoute backend request timeout", "timeout", string(*timeout.BackendRequest), "error", err)
		} else {
			in.PerTryTimeout = &metav1.Duration{Duration: duration}
		}
	}

	return policy.BuildRetryPolicy(in)
}

func (r ruleIR) applyRetry(
	action *envoyroutev3.RouteAction,
	mergeOpts policy.MergeOptions,
) {
	if r.retry == nil || !policy.IsSettable(action.GetRetryPolicy(), mergeOpts) {
		return
	}
	action.RetryPolicy = r.retry
}

func convertSessionPersistence(sessionPersistence *gwv1.SessionPersistence) *stateful_sessionv3.StatefulSessionPerRoute {
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
		SessionState: &envoycorev3.TypedExtensionConfig{
			Name:        "envoy.http.stateful_session." + strings.ToLower(string(spType)),
			TypedConfig: sessionStateAny,
		},
	}
	return &stateful_sessionv3.StatefulSessionPerRoute{
		Override: &stateful_sessionv3.StatefulSessionPerRoute_StatefulSession{
			StatefulSession: statefulSession,
		},
	}
}

func translatePathRewrite(outputRoute *envoyroutev3.RedirectAction, pathRewrite *gwv1.HTTPPathModifier) {
	if pathRewrite == nil {
		return
	}
	switch pathRewrite.Type {
	case gwv1.FullPathHTTPPathModifier:
		outputRoute.PathRewriteSpecifier = &envoyroutev3.RedirectAction_PathRedirect{
			PathRedirect: ptr.Deref(pathRewrite.ReplaceFullPath, "/"),
		}
	case gwv1.PrefixMatchHTTPPathModifier:
		outputRoute.PathRewriteSpecifier = &envoyroutev3.RedirectAction_PrefixRewrite{
			PrefixRewrite: ptr.Deref(pathRewrite.ReplacePrefixMatch, "/"),
		}
	}
}

func translateScheme(out *envoyroutev3.RedirectAction, scheme *string) {
	if scheme == nil {
		return
	}

	if strings.ToLower(*scheme) == "https" {
		out.SchemeRewriteSpecifier = &envoyroutev3.RedirectAction_HttpsRedirect{HttpsRedirect: true}
	} else {
		out.SchemeRewriteSpecifier = &envoyroutev3.RedirectAction_SchemeRedirect{SchemeRedirect: *scheme}
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

func translateStatusCode(i *int) envoyroutev3.RedirectAction_RedirectResponseCode {
	if i == nil {
		return envoyroutev3.RedirectAction_FOUND
	}

	switch *i {
	case 301:
		return envoyroutev3.RedirectAction_MOVED_PERMANENTLY
	case 302:
		return envoyroutev3.RedirectAction_FOUND
	case 303:
		return envoyroutev3.RedirectAction_SEE_OTHER
	case 307:
		return envoyroutev3.RedirectAction_TEMPORARY_REDIRECT
	case 308:
		return envoyroutev3.RedirectAction_PERMANENT_REDIRECT
	default:
		return envoyroutev3.RedirectAction_FOUND
	}
}

// MIRROR IR
// ===========
type mirrorIr struct {
	Cluster         string
	RuntimeFraction *envoycorev3.RuntimeFractionalPercent
}

func (m *mirrorIr) apply(
	outputRoute *envoyroutev3.Route,
	mergeOpts policy.MergeOptions,
) {
	if outputRoute == nil || outputRoute.GetRoute() == nil ||
		!policy.IsSettable(outputRoute.GetRoute().GetRequestMirrorPolicies(), mergeOpts) {
		return
	}
	mirror := &envoyroutev3.RouteAction_RequestMirrorPolicy{
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
	Add       []*envoycorev3.HeaderValueOption
	Remove    []string
	IsRequest bool // true=request, false=response
}

func (h *headerModifierIr) apply(
	outputRoute *envoyroutev3.Route,
	_ policy.MergeOptions,
) {
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

func convertHeaderModifierIR(_ krt.HandlerContext, f *gwv1.HTTPHeaderFilter, isRequest bool) *headerModifierIr {
	if f == nil {
		return nil
	}
	var add []*envoycorev3.HeaderValueOption
	for _, h := range f.Add {
		add = append(add, &envoycorev3.HeaderValueOption{
			Header: &envoycorev3.HeaderValue{
				Key:   string(h.Name),
				Value: h.Value,
			},
			AppendAction: envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
		})
	}
	for _, h := range f.Set {
		add = append(add, &envoycorev3.HeaderValueOption{
			Header: &envoycorev3.HeaderValue{
				Key:   string(h.Name),
				Value: h.Value,
			},
			AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		})
	}
	return &headerModifierIr{
		Add:       add,
		Remove:    f.Remove,
		IsRequest: isRequest,
	}
}

func getFractionPercent(f gwv1.HTTPRequestMirrorFilter) *envoycorev3.RuntimeFractionalPercent {
	if f.Percent != nil {
		return &envoycorev3.RuntimeFractionalPercent{
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
		return &envoycorev3.RuntimeFractionalPercent{
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

// ApplyForRoute may be invoked multiple times on the route, once for each policy since
// the builtin plugin does not implement MergePolicies.
// For delegated routes, policies attached to the parent route are inherited
// and may override the current policy on the output route if pCtx.InheritedPolicyPriority allows it
// Currently, ApplyForRoute is invoked per policy in order of priority from highest(child route policies)
// to lowest(parent route policies).
func (p *builtinPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoyroutev3.Route) error {
	pol, ok := pCtx.Policy.(*builtinPlugin)
	if !ok {
		return nil
	}

	mergeOpts := policy.MergeOptions{
		Strategy: policy.GetMergeStrategy(pCtx.InheritedPolicyPriority, false),
	}

	var errs error
	if pol.filter != nil {
		pol.filter.apply(outputRoute, mergeOpts)
	}

	p.applyRulePolicy(pCtx, pol.rule, mergeOpts, outputRoute)
	if pol.hasCors {
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
		stagedFilter.Filter.Disabled = true
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

// New helper to create filterIR
func convertfilterIR(kctx krt.HandlerContext, f gwv1.HTTPRouteFilter, fromgk schema.GroupKind, fromns string, refgrants *RefGrantIndex, ups *BackendIndex) *filterIR {
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
	return &filterIR{
		filterType: f.Type,
		policy:     policy,
	}
}

// REQUEST REDIRECT IR
// ===================
type requestRedirectIr struct {
	Redir *envoyroutev3.RedirectAction
}

func (r *requestRedirectIr) apply(
	outputRoute *envoyroutev3.Route,
	mergeOpts policy.MergeOptions,
) {
	if outputRoute == nil || !policy.IsSettable(outputRoute.GetRedirect(), mergeOpts) {
		return
	}
	outputRoute.Action = &envoyroutev3.Route_Redirect{
		Redirect: r.Redir,
	}
}

func convertRequestRedirectIR(_ krt.HandlerContext, config *gwv1.HTTPRequestRedirectFilter) *requestRedirectIr {
	if config == nil {
		return nil
	}
	redir := &envoyroutev3.RedirectAction{
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
	HostRewrite   *envoyroutev3.RouteAction_HostRewriteLiteral
	FullReplace   string
	PrefixReplace string
}

func (u *urlRewriteIr) apply(
	outputRoute *envoyroutev3.Route,
	mergeOpts policy.MergeOptions,
) {
	if outputRoute == nil || outputRoute.GetRoute() == nil {
		return
	}

	if u.HostRewrite != nil && policy.IsSettable(outputRoute.GetRoute().GetHostRewriteSpecifier(), mergeOpts) {
		outputRoute.GetRoute().HostRewriteSpecifier = u.HostRewrite
	}
	if u.FullReplace != "" && policy.IsSettable(outputRoute.GetRoute().GetRegexRewrite(), mergeOpts) {
		outputRoute.GetRoute().RegexRewrite = &envoy_type_matcher_v3.RegexMatchAndSubstitute{
			Pattern: &envoy_type_matcher_v3.RegexMatcher{
				Regex: ".*",
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
		if path != "" && u.PrefixReplace == "/" && policy.IsSettable(outputRoute.GetRoute().GetRegexRewrite(), mergeOpts) {
			outputRoute.GetRoute().RegexRewrite = &envoy_type_matcher_v3.RegexMatchAndSubstitute{
				Pattern: &envoy_type_matcher_v3.RegexMatcher{
					Regex: "^" + path + "\\/*",
				},
				Substitution: "/",
			}
		} else if policy.IsSettable(outputRoute.GetRoute().GetPrefixRewrite(), mergeOpts) {
			outputRoute.GetRoute().PrefixRewrite = u.PrefixReplace
		}
	}
}

func convertURLRewriteIR(_ krt.HandlerContext, config *gwv1.HTTPURLRewriteFilter) *urlRewriteIr {
	if config == nil {
		return nil
	}
	var hostrewrite *envoyroutev3.RouteAction_HostRewriteLiteral
	if config.Hostname != nil {
		hostrewrite = &envoyroutev3.RouteAction_HostRewriteLiteral{
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

func (c *corsIr) apply(
	outputRoute *envoyroutev3.Route,
	mergeOpts policy.MergeOptions,
) {
	if c.Cors == nil || !policy.IsSettable(outputRoute.GetTypedPerFilterConfig()[envoy_wellknown.CORS], mergeOpts) {
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
	corsPolicyAny, err := utils.MessageToAny(policy.BuildCorsPolicy(f, false))
	if err != nil {
		// this should never happen.
		logger.Error("failed to convert CORS policy to Any", "error", err)
		return nil
	}
	return &corsIr{Cors: corsPolicyAny}
}
