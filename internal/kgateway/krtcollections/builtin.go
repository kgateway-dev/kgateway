package krtcollections

import (
	"context"
	"errors"
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
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
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

type builtinPlugin struct {
	filterMutation func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error
	ruleMutation   func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error
	cors           *gwv1.HTTPCORSFilter
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
		cors:           cors,
		filterMutation: convert(kctx, f, fromgk, fromns, refgrants, ups),
	}
}

func NewBuiltInRuleIr(rule gwv1.HTTPRouteRule) ir.PolicyIR {
	// If no rule policies are set, return nil so that we don't have a no-op policy
	if rule.Timeouts == nil && rule.Retry == nil && rule.SessionPersistence == nil {
		return nil
	}
	return &builtinPlugin{
		ruleMutation: convertRule(rule),
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

func convert(kctx krt.HandlerContext, f gwv1.HTTPRouteFilter, fromgk schema.GroupKind, fromns string, refgrants *RefGrantIndex, ups *BackendIndex) func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
	switch f.Type {
	case gwv1.HTTPRouteFilterRequestMirror:
		return convertMirror(kctx, f.RequestMirror, fromgk, fromns, refgrants, ups)
	case gwv1.HTTPRouteFilterRequestHeaderModifier:
		return convertHeaderModifier(kctx, f.RequestHeaderModifier)
	case gwv1.HTTPRouteFilterResponseHeaderModifier:
		return convertResponseHeaderModifier(kctx, f.ResponseHeaderModifier)
	case gwv1.HTTPRouteFilterRequestRedirect:
		return convertRequestRedirect(kctx, f.RequestRedirect)
	case gwv1.HTTPRouteFilterURLRewrite:
		return convertURLRewrite(kctx, f.URLRewrite)
	case gwv1.HTTPRouteFilterCORS:
		return convertCORS(kctx, f.CORS)
	}
	return nil
}

func formatRuleError(action string, ruleIR ir.HttpRouteRuleMatchIR, err error) error {
	if ruleIR.Name != "" {
		return fmt.Errorf("failed to apply HTTPRoute %s for route %s/%s (rule: %s): %w", action, string(*ruleIR.ParentRef.Namespace), ruleIR.ParentRef.Name, ruleIR.Name, err)
	}
	return fmt.Errorf("failed to apply HTTPRoute %s for route %s/%s: %w", action, string(*ruleIR.ParentRef.Namespace), ruleIR.ParentRef.Name, err)
}

func convertRule(rule gwv1.HTTPRouteRule) func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
	sessionState := convertSessionPersistence(rule.SessionPersistence)
	return func(ruleIR ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
		// A parent route rule with a delegated backend will not have outputRoute.RouteAction set
		// but the plugin will be invoked on the rule, so treat this as a no-op call
		if outputRoute == nil || outputRoute.GetRoute() == nil {
			return nil
		}

		err := applyTimeout(outputRoute, rule.Timeouts, rule.Retry != nil)
		if err != nil {
			return formatRuleError("timeout", ruleIR, err)
		}

		err = applyRetry(outputRoute, rule.Retry, rule.Timeouts)
		if err != nil {
			return formatRuleError("retry", ruleIR, err)
		}
		err = applySessionPersistence(outputRoute, sessionState)
		if err != nil {
			return formatRuleError("session persistence", ruleIR, err)
		}
		return nil
	}
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

func applySessionPersistence(route *envoy_config_route_v3.Route, sessionPersistence *anypb.Any) error {
	if sessionPersistence == nil {
		return nil
	}

	if route.GetTypedPerFilterConfig() == nil {
		route.TypedPerFilterConfig = map[string]*anypb.Any{}
	}
	route.GetTypedPerFilterConfig()[statefulSessionFilterName] = sessionPersistence

	return nil
}

func applyTimeout(route *envoy_config_route_v3.Route, timeout *gwv1.HTTPRouteTimeouts, hasRetry bool) error {
	if timeout == nil {
		return nil
	}

	var timeoutStr string
	// Apply the required timeout selection logic
	switch {
	case timeout.BackendRequest != nil && timeout.Request != nil:
		// When both timeouts are set:
		// - Without retry: Use BackendRequest, since it's more specific (shorter)
		// - With retry: Use Request as the overall route timeout since
		//   BackendRequest will be applied to each retry attempt
		if hasRetry {
			timeoutStr = string(*timeout.Request)
		} else {
			timeoutStr = string(*timeout.BackendRequest)
		}
	case timeout.BackendRequest != nil:
		// Only BackendRequest is set
		timeoutStr = string(*timeout.BackendRequest)
	case timeout.Request != nil:
		// Only Request is set
		timeoutStr = string(*timeout.Request)
	default:
		return nil
	}

	duration, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return fmt.Errorf("invalid HTTPRoute timeout %s: %w", timeoutStr, err)
	}
	route.GetRoute().Timeout = durationpb.New(duration)
	return nil
}

func applyRetry(route *envoy_config_route_v3.Route, retry *gwv1.HTTPRouteRetry, timeout *gwv1.HTTPRouteTimeouts) error {
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
			return fmt.Errorf("invalid HTTPRoute retry backoff %s: %w", *retry.Backoff, err)
		}
		retryPolicy.RetryBackOff = &envoy_config_route_v3.RetryPolicy_RetryBackOff{
			BaseInterval: durationpb.New(backoff),
		}
	}

	// If a backend request timeout is set, use it as the per-try timeout.
	// Otherwise, Envoy will by default use the global route timeout
	// Refer to https://gateway-api.sigs.k8s.io/geps/gep-1742/
	if timeout != nil && timeout.BackendRequest != nil {
		timeoutDuration, err := time.ParseDuration(string(*timeout.BackendRequest))
		if err != nil {
			return fmt.Errorf("invalid HTTPRoute backend request timeout %s: %w", *timeout.BackendRequest, err)
		}
		retryPolicy.PerTryTimeout = durationpb.New(timeoutDuration)
	}

	route.GetRoute().RetryPolicy = retryPolicy
	return nil
}

func convertURLRewrite(kctx krt.HandlerContext, config *gwv1.HTTPURLRewriteFilter) func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
	if config == nil {
		return func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
			return errors.New("missing rewrite filter")
		}
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

	return func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
		if outputRoute.GetRoute() == nil {
			if in.Delegates {
				// if route has children, it's a delegate route, and we don't need to return an error
				// as this might need to apply to children.
				return nil
			}
			return errors.New("missing route action")
		}

		if hostrewrite != nil {
			outputRoute.GetRoute().HostRewriteSpecifier = hostrewrite
		}
		if fullReplace != "" {
			outputRoute.GetRoute().RegexRewrite = &envoy_type_matcher_v3.RegexMatchAndSubstitute{
				Pattern: &envoy_type_matcher_v3.RegexMatcher{
					EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{}},
					Regex:      ".*",
				},
				Substitution: fullReplace,
			}
		}

		if prefixReplace != "" {
			// TODO: not idealy way to get the path from the input route.
			// see if we can plumb the input route into the context
			path := outputRoute.GetMatch().GetPrefix()
			if path == "" {
				path = outputRoute.GetMatch().GetPath()
			}
			if path == "" {
				path = outputRoute.GetMatch().GetPathSeparatedPrefix()
			}
			if path != "" && prefixReplace == "/" {
				outputRoute.GetRoute().RegexRewrite = &envoy_type_matcher_v3.RegexMatchAndSubstitute{
					Pattern: &envoy_type_matcher_v3.RegexMatcher{
						EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{}},
						Regex:      "^" + path + "\\/*",
					},
					Substitution: "/",
				}
			} else {
				outputRoute.GetRoute().PrefixRewrite = prefixReplace
			}
		}
		return nil
	}
}

func convertRequestRedirect(kctx krt.HandlerContext, config *gwv1.HTTPRequestRedirectFilter) func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
	if config == nil {
		return func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
			return errors.New("missing redirect filter")
		}
	}

	redir := &envoy_config_route_v3.RedirectAction{
		HostRedirect: translateHostname(config.Hostname),
		ResponseCode: translateStatusCode(config.StatusCode),
		PortRedirect: translatePort(config.Port),
	}

	// can't return this because proto oneofs are private
	translateScheme(redir, config.Scheme)
	translatePathRewrite(redir, config.Path)

	return func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
		// TODO: check if action is nil and error if not?
		outputRoute.Action = &envoy_config_route_v3.Route_Redirect{
			Redirect: redir,
		}
		return nil
	}
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

func convertHeaderModifier(kctx krt.HandlerContext, f *gwv1.HTTPHeaderFilter) func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
	if f == nil {
		return nil
	}
	var headersToAddd []*envoy_config_core_v3.HeaderValueOption
	// TODO: add validation for header names/values with CheckForbiddenCustomHeaders
	for _, h := range f.Add {
		headersToAddd = append(headersToAddd, &envoy_config_core_v3.HeaderValueOption{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   string(h.Name),
				Value: h.Value,
			},
			AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
		})
	}
	for _, h := range f.Set {
		headersToAddd = append(headersToAddd, &envoy_config_core_v3.HeaderValueOption{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   string(h.Name),
				Value: h.Value,
			},
			AppendAction: envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		})
	}
	toremove := f.Remove

	return func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
		outputRoute.RequestHeadersToAdd = append(outputRoute.GetRequestHeadersToAdd(), headersToAddd...)
		outputRoute.RequestHeadersToRemove = append(outputRoute.GetRequestHeadersToRemove(), toremove...)
		return nil
	}
}

func convertResponseHeaderModifier(kctx krt.HandlerContext, f *gwv1.HTTPHeaderFilter) func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
	if f == nil {
		return nil
	}
	var headersToAddd []*envoy_config_core_v3.HeaderValueOption
	// TODO: add validation for header names/values with CheckForbiddenCustomHeaders
	for _, h := range f.Add {
		headersToAddd = append(headersToAddd, &envoy_config_core_v3.HeaderValueOption{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   string(h.Name),
				Value: h.Value,
			},
			AppendAction: envoy_config_core_v3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
		})
	}
	for _, h := range f.Set {
		headersToAddd = append(headersToAddd, &envoy_config_core_v3.HeaderValueOption{
			Header: &envoy_config_core_v3.HeaderValue{
				Key:   string(h.Name),
				Value: h.Value,
			},
			AppendAction: envoy_config_core_v3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		})
	}
	toremove := f.Remove

	return func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
		outputRoute.ResponseHeadersToAdd = append(outputRoute.GetResponseHeadersToAdd(), headersToAddd...)
		outputRoute.ResponseHeadersToRemove = append(outputRoute.GetResponseHeadersToRemove(), toremove...)
		return nil
	}
}

func convertMirror(kctx krt.HandlerContext, f *gwv1.HTTPRequestMirrorFilter, fromgk schema.GroupKind, fromns string, refgrants *RefGrantIndex, ups *BackendIndex) func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
	if f == nil {
		return nil
	}
	to := toFromBackendRef(fromns, f.BackendRef)
	if !refgrants.ReferenceAllowed(kctx, fromgk, fromns, to) {
		// TODO: report error
		return nil
	}
	up, err := ups.getBackendFromRef(kctx, fromns, f.BackendRef)
	if err != nil {
		// TODO: report error
		return nil
	}
	fraction := getFractionPercent(*f)
	mirror := &envoy_config_route_v3.RouteAction_RequestMirrorPolicy{
		Cluster:         up.ClusterName(),
		RuntimeFraction: fraction,
	}
	return func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
		route := outputRoute.GetRoute()
		if route == nil {
			// TODO: report error
			return nil
		}
		route.RequestMirrorPolicies = append(route.GetRequestMirrorPolicies(), mirror)
		return nil
	}
}

func convertCORS(_ krt.HandlerContext, f *gwv1.HTTPCORSFilter) func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
	if f == nil {
		return nil
	}
	return func(in ir.HttpRouteRuleMatchIR, outputRoute *envoy_config_route_v3.Route) error {
		corsPolicyAny, err := utils.MessageToAny(ToEnvoyCorsPolicy(f))
		if err != nil {
			return fmt.Errorf("failed to convert CORS policy to Any: %w", err)
		}

		if outputRoute.GetTypedPerFilterConfig() == nil {
			outputRoute.TypedPerFilterConfig = make(map[string]*anypb.Any)
		}
		outputRoute.GetTypedPerFilterConfig()[envoy_wellknown.CORS] = corsPolicyAny
		return nil
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

// called 1 time for each listener
func (p *builtinPluginGwPass) ApplyListenerPlugin(ctx context.Context, pCtx *ir.ListenerContext, out *envoy_config_listener_v3.Listener) {
}

func (p *builtinPluginGwPass) ApplyVhostPlugin(ctx context.Context, pCtx *ir.VirtualHostContext, out *envoy_config_route_v3.VirtualHost) {
}

// called one or more times per route rule
func (p *builtinPluginGwPass) ApplyForRoute(ctx context.Context, pCtx *ir.RouteContext, outputRoute *envoy_config_route_v3.Route) error {
	policy, ok := pCtx.Policy.(*builtinPlugin)
	if !ok {
		return nil
	}

	var errs error
	if policy.filterMutation != nil {
		if err := policy.filterMutation(pCtx.In, outputRoute); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	if policy.ruleMutation != nil {
		if err := policy.ruleMutation(pCtx.In, outputRoute); err != nil {
			errs = errors.Join(errs, err)
		}
		if outputRoute.GetTypedPerFilterConfig()[statefulSessionFilterName] != nil {
			p.needStatefulSession[pCtx.FilterChainName] = true
		}
	}

	if policy.cors != nil {
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
	if inPolicy.cors != nil {
		pCtx.TypedFilterConfig[envoy_wellknown.CORS] = ToEnvoyCorsPolicy(inPolicy.cors)
		p.hasCorsPolicy[pCtx.FilterChainName] = true
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

func (p *builtinPluginGwPass) NetworkFilters(ctx context.Context) ([]plugins.StagedNetworkFilter, error) {
	return nil, nil
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *builtinPluginGwPass) ResourcesToAdd(ctx context.Context) ir.Resources {
	return ir.Resources{}
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
