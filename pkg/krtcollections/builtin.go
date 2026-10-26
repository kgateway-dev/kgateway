package krtcollections

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	corsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	envoy_wellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	apiconditions "github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils/sessionpersistence"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

const (
	httpRedirectStatusCodesAllowedMsg = "must be one of 301, 302, 303, 307, 308"
)

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
	timeouts *timeouts
	retry    *envoyroutev3.RetryPolicy
	// sessionPersistence is the per route stateful session config, marshaled once when the rule is built
	// so route translation does not marshal it again for every route. It is shared and must not be modified.
	sessionPersistence *anypb.Any
	// absoluteTimeoutIgnored is set when the sessionPersistence absoluteTimeout has no effect
	// (a Session cookie or a header session), so it is reported on the route status.
	absoluteTimeoutIgnored bool
}

type filterIR struct {
	filterType gwv1.HTTPRouteFilterType
	policy     applyToRoute
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
	// +noKrtEquals
	filter *filterIR
	// +noKrtEquals
	rule ruleIR
	// +noKrtEquals
	hasCors bool
}

func (d *builtinPlugin) CreationTime() time.Time {
	// should this be infinity?
	return time.Time{}
}

func (d *builtinPlugin) Equals(in any) bool {
	// we don't really need equality check here, because this policy is embedded in the httproute,
	// and we have generation based equality checks for that already.
	// +noKrtEquals
	return true
}

type builtinPluginGwPass struct {
	ir.UnimplementedProxyTranslationPass
	reporter            reporter.Reporter
	hasCorsPolicy       map[string]bool
	needStatefulSession map[string]bool
	// sessionPersistenceOverrides collects, per route parent, the messages for route rules whose
	// sessionPersistence is overridden by a BackendConfigPolicy, so a route reports all of them.
	sessionPersistenceOverrides map[sessionPersistenceOverrideKey]sets.Set[string]
	// sessionPersistenceClaimed tracks the output routes whose effective rule sessionPersistence is already
	// decided by a higher priority rule policy (e.g. a child rule's own over an inherited parent rule's).
	sessionPersistenceClaimed map[*envoyroutev3.Route]*anypb.Any
	// absoluteTimeoutIgnored collects, per route parent, the rules whose sessionPersistence
	// absoluteTimeout has no effect, so a route reports all of them in one condition.
	absoluteTimeoutIgnored map[absoluteTimeoutIgnoredKey]sets.Set[string]
}

type sessionPersistenceOverrideKey struct {
	route     types.UID
	routeName string
	// parentRef matches how the reports package keys a route's parent status, so all rules reported
	// under one parent status share one entry here.
	parentRef reports.ParentRefKey
}

type absoluteTimeoutIgnoredKey struct {
	route     types.UID
	parentRef reports.ParentRefKey
}

var _ ir.PolicyIR = &builtinPlugin{}

func (h *RoutesIndex) NewBuiltInIr(
	kctx krt.HandlerContext,
	f gwv1.HTTPRouteFilter,
	fromgk schema.GroupKind,
	fromns string,
	refgrants *RefGrantIndex,
	ups *BackendIndex,
	ruleName *gwv1.SectionName,
	annotations map[string]string,
) (*builtinPlugin, error) {
	var cors *gwv1.HTTPCORSFilter
	if f.Type == gwv1.HTTPRouteFilterCORS {
		cors = f.CORS
	}
	filterIR, err := h.convertfilterIR(kctx, f, fromgk, fromns, refgrants, ups, ruleName, annotations)
	if err != nil {
		return nil, err
	}
	return &builtinPlugin{
		// CORS might be configured but experimental features might be disabled,
		// So set hasCors only if the HTTPCORSFilter has been translated
		hasCors: cors != nil && filterIR != nil,
		filter:  filterIR,
	}, nil
}

func (h *RoutesIndex) NewBuiltInRuleIr(rule gwv1.HTTPRouteRule) *builtinPlugin {
	// If no rule policies are set, return nil so that we don't have a no-op policy
	if rule.Timeouts == nil && rule.Retry == nil && rule.SessionPersistence == nil { //nolint:staticcheck // SA1019: route-level session persistence is still supported
		return nil
	}
	return &builtinPlugin{
		rule: h.buildHTTPRouteRulePolicy(rule),
	}
}

func NewBuiltinPlugin(ctx context.Context) sdk.Plugin {
	return sdk.Plugin{
		ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			ir.VirtualBuiltInGK: {
				NewGatewayTranslationPass: NewGatewayTranslationPass,
			},
		},
	}
}

func (h *RoutesIndex) buildHTTPRouteRulePolicy(rule gwv1.HTTPRouteRule) ruleIR {
	ir := ruleIR{
		timeouts: convertTimeouts(rule.Timeouts),
	}

	ir.retry = convertRetry(rule.Retry, rule.Timeouts)
	// ON_EXPERIMENTAL_PROMOTION : Remove this block
	// Ref: https://github.com/kgateway-dev/kgateway/issues/12825
	//nolint:staticcheck // SA1019: route-level session persistence is still supported
	if rule.SessionPersistence != nil {
		if h.enableExperimentalGatewayAPIFeatures {
			ir.sessionPersistence = convertSessionPersistence(rule.SessionPersistence)
			ir.absoluteTimeoutIgnored = absoluteTimeoutIgnored(rule.SessionPersistence)
		} else {
			logger.Warn("experimental gateway api features are disabled but SessionPersistence is configured. Skipping")
		}
	}
	return ir
}

func (p *builtinPluginGwPass) applyRulePolicy(
	pCtx *ir.RouteContext,
	r ruleIR,
	mergeOpts policy.MergeOptions,
	outputRoute *envoyroutev3.Route,
) {
	// A parent route rule with a delegated backend will not have outputRoute.RouteAction set
	// but the plugin will be invoked on the rule, so treat this as a no-op call
	if outputRoute == nil || outputRoute.GetRoute() == nil {
		return
	}
	r.applyTimeouts(outputRoute.GetRoute(), r.retry != nil, mergeOpts)
	r.applyRetry(outputRoute.GetRoute(), mergeOpts)

	// Session persistence from a BackendConfigPolicy attached to the backend takes precedence over the route rule.
	// For a single backend, the backend's per filter config is shared with the route via pCtx.TypedFilterConfig.
	// For multiple backends, it is set on each weighted cluster, which Envoy prefers over the route.
	backendHasSessionPersistence := pCtx != nil && pCtx.TypedFilterConfig.GetTypedConfig(sessionpersistence.FilterName) != nil
	// Only the rule sessionPersistence that would apply without the BackendConfigPolicy is reported as
	// overridden. Rule policies are applied from highest to lowest priority, so a lower priority one (e.g.
	// inherited from a delegating parent) is only effective if the merge strategy lets it replace the first.
	if r.sessionPersistence != nil && policy.IsSettable(p.sessionPersistenceClaimed[outputRoute], mergeOpts) {
		if p.sessionPersistenceClaimed != nil {
			p.sessionPersistenceClaimed[outputRoute] = r.sessionPersistence
		}
		p.reportSessionPersistenceOverridden(pCtx, outputRoute, backendHasSessionPersistence)
	}
	if r.sessionPersistence != nil && !backendHasSessionPersistence &&
		policy.IsSettable(outputRoute.GetTypedPerFilterConfig()[sessionpersistence.FilterName], mergeOpts) {
		if outputRoute.GetTypedPerFilterConfig() == nil {
			outputRoute.TypedPerFilterConfig = map[string]*anypb.Any{}
		}
		outputRoute.GetTypedPerFilterConfig()[sessionpersistence.FilterName] = r.sessionPersistence
		p.needStatefulSession[pCtx.FilterChainName] = true
		if r.absoluteTimeoutIgnored {
			p.reportAbsoluteTimeoutIgnored(pCtx)
		}
	}
}

const msgAbsoluteTimeoutIgnored = "sessionPersistence.absoluteTimeout only has an effect for Cookie sessions " +
	"with sessionPersistence.cookie.lifetimeType Permanent"

// absoluteTimeoutIgnored reports whether sp sets an absoluteTimeout that has no effect.
// Envoy can only enforce the session lifetime through the cookie Max-Age, which a Session
// cookie (the default lifetime) does not have, so the timeout only applies to Permanent cookies.
func absoluteTimeoutIgnored(sp *gwv1.SessionPersistence) bool {
	if sp == nil || sp.AbsoluteTimeout == nil {
		return false
	}
	if ptr.Deref(sp.Type, gwv1.CookieBasedSessionPersistence) != gwv1.CookieBasedSessionPersistence {
		// Header sessions never use the timeout.
		return true
	}
	lifetime := gwv1.SessionCookieLifetimeType
	if sp.Cookie != nil {
		lifetime = ptr.Deref(sp.Cookie.LifetimeType, gwv1.SessionCookieLifetimeType)
	}
	return lifetime == gwv1.SessionCookieLifetimeType
}

// reportAbsoluteTimeoutIgnored reports, on the route's parent status, a route rule whose sessionPersistence
// absoluteTimeout has no effect. It sets kgateway.dev/Programmed=True with the
// SessionPersistenceAbsoluteTimeoutIgnored reason, unless the condition already reports a problem (False).
func (p *builtinPluginGwPass) reportAbsoluteTimeoutIgnored(pCtx *ir.RouteContext) {
	if p.reporter == nil || pCtx == nil {
		return
	}
	// The sessionPersistence belongs to the rule that owns this policy: the rule itself, or a
	// parent rule delegating to it. Report on that rule's route.
	owner := ruleOwner(&pCtx.In, pCtx.Policy)
	if owner.Parent == nil || owner.Parent.SourceObject == nil {
		return
	}
	rule := owner.RuleName
	if rule == "" {
		rule = strconv.Itoa(owner.RuleIndex)
	}
	src := owner.Parent.SourceObject
	key := absoluteTimeoutIgnoredKey{route: src.GetUID(), parentRef: parentRefKey(owner.ParentRef)}
	rules := p.absoluteTimeoutIgnored[key]
	if rules == nil {
		rules = sets.New[string]()
		p.absoluteTimeoutIgnored[key] = rules
	}
	rules.Insert("rule " + rule)

	parentReport := p.reporter.Route(src).ParentRef(&owner.ParentRef)
	// A dropped or replaced rule reports Programmed=False; never hide it.
	if prr, ok := parentReport.(*reports.ParentRefReport); ok {
		if cond := meta.FindStatusCondition(prr.Conditions, apiconditions.KgatewayConditionProgrammed); cond != nil &&
			cond.Status == metav1.ConditionFalse {
			return
		}
	}
	parentReport.SetCondition(reporter.RouteCondition{
		Type:    gwv1.RouteConditionType(apiconditions.KgatewayConditionProgrammed),
		Status:  metav1.ConditionTrue,
		Reason:  gwv1.RouteConditionReason(apiconditions.KgatewayReasonSessionPersistenceAbsoluteTimeoutIgnored),
		Message: strings.Join(sets.List(rules), ", ") + ": " + msgAbsoluteTimeoutIgnored,
	})
}

func convertTimeouts(timeout *gwv1.HTTPRouteTimeouts) *timeouts {
	if timeout == nil {
		return nil
	}

	var requestTimeout *durationpb.Duration
	if timeout.Request != nil {
		if parsed, err := time.ParseDuration(string(*timeout.Request)); err == nil {
			requestTimeout = durationpb.New(parsed)
		}
	}

	var backendRequestTimeout *durationpb.Duration
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
		// Do not set the timeout if retry is specified and only BackendRequest is set.
		// This will be handled in envoyroutev3.RetryPolicy.PerTryTimeout
		if !hasRetry {
			timeout = timeouts.backendRequestTimeout
		}
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

	in := &kgateway.Retry{
		Attempts: 1,
		RetryOn: []kgateway.RetryOnCondition{
			"cancelled", "connect-failure", "refused-stream", "retriable-headers", "retriable-status-codes", "unavailable", "reset",
		},
		StatusCodes: retry.Codes,
	}
	// An explicit zero disables retries (num_retries: 0).
	if retry.Attempts != nil {
		in.Attempts = int32(*retry.Attempts) //nolint:gosec // G115: retry attempts are small non-negative integers
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

func convertSessionPersistence(sessionPersistence *gwv1.SessionPersistence) *anypb.Any {
	if sessionPersistence == nil {
		return nil
	}
	cfg, err := sessionpersistence.ConfigFromAPI(sessionPersistenceToAPI(sessionPersistence))
	if err != nil {
		logger.Error("failed to create session state", "error", err)
		return nil
	}
	perRoute, err := sessionpersistence.Build(cfg)
	if err != nil {
		logger.Error("failed to create session state", "error", err)
		return nil
	}
	out, err := utils.MessageToAny(perRoute)
	if err != nil {
		logger.Error("error marshalling SessionPersistence", "error", err)
		return nil
	}
	return out
}

// sessionPersistenceToAPI converts the route rule sessionPersistence into the kgateway SessionPersistence
// API, so route rules and BackendConfigPolicy share the same conversion code. Both have the same shape;
// only the route rule default cookie and header names are filled in. An unset lifetimeType is a Session
// cookie, as the CRD defaults it.
func sessionPersistenceToAPI(sp *gwv1.SessionPersistence) *kgateway.SessionPersistence {
	spType := kgateway.SessionPersistenceType(ptr.Deref(sp.Type, gwv1.CookieBasedSessionPersistence))
	out := &kgateway.SessionPersistence{Type: &spType}
	if sp.AbsoluteTimeout != nil {
		if _, err := time.ParseDuration(string(*sp.AbsoluteTimeout)); err != nil {
			// Header sessions never use the timeout, so only warn for cookies.
			if spType == kgateway.CookieBasedSessionPersistence {
				logger.Warn("ignoring invalid sessionPersistence absoluteTimeout", "absolute_timeout", *sp.AbsoluteTimeout, "error", err)
			}
		} else {
			out.AbsoluteTimeout = sp.AbsoluteTimeout
		}
	}

	switch spType {
	case kgateway.CookieBasedSessionPersistence:
		cookie := ptr.Deref(sp.Cookie, gwv1.CookieConfig{})
		out.Cookie = &kgateway.SessionPersistenceCookieConfig{
			Name: new(kgateway.CookieName(ptr.Deref(cookie.Name, "sessionPersistence"))),
			// An unset or empty path defaults to root, to set the cookie for all requests to the hostname.
			// Default browser behavior otherwise is to use the current "directory" of the request.
			// When a request comes in without session cookie for a subpath, it would be limited to that subpath and
			// requests to unrelated subpaths could get routed to a different upstream.
			// Cf. https://httpwg.org/specs/rfc6265.html#sane-path
			Path:         cookie.Path,
			LifetimeType: (*kgateway.CookieLifetimeType)(cookie.LifetimeType),
		}
	case kgateway.HeaderBasedSessionPersistence:
		name := gwv1.HeaderName("x-session-persistence")
		if sp.Header != nil && sp.Header.Name != "" {
			name = sp.Header.Name
		}
		out.Header = &kgateway.SessionPersistenceHeaderConfig{
			Name: name,
		}
	}
	return out
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

func defaultPortForScheme(scheme string) uint32 {
	switch strings.ToLower(scheme) {
	case "http":
		return 80
	case "https":
		return 443
	default:
		return 0
	}
}

func translateHostname(hostname *gwv1.PreciseHostname) string {
	if hostname == nil {
		return ""
	}
	return string(*hostname)
}

func translateStatusCode(
	statusCode *int,
	ruleName *gwv1.SectionName,
	annotations map[string]string,
) (envoyroutev3.RedirectAction_RedirectResponseCode, error) {
	var overrideStatusCode *int
	if value, ok := annotations[apiannotations.HTTPRedirectStatusCode]; ok {
		var err error
		overrideStatusCode, err = parseRedirectStatusCodeAnnotation(value, ruleName)
		if err != nil {
			return envoyroutev3.RedirectAction_FOUND, err
		}
	}

	if statusCode == nil && overrideStatusCode == nil {
		return envoyroutev3.RedirectAction_FOUND, nil
	} else if overrideStatusCode != nil {
		// if overrideStatusCode is set, it takes precedence over statusCode
		statusCode = overrideStatusCode
	}

	switch *statusCode {
	case 301:
		return envoyroutev3.RedirectAction_MOVED_PERMANENTLY, nil
	case 302:
		return envoyroutev3.RedirectAction_FOUND, nil
	case 303:
		return envoyroutev3.RedirectAction_SEE_OTHER, nil
	case 307:
		return envoyroutev3.RedirectAction_TEMPORARY_REDIRECT, nil
	case 308:
		return envoyroutev3.RedirectAction_PERMANENT_REDIRECT, nil
	default:
		return envoyroutev3.RedirectAction_FOUND, fmt.Errorf("invalid redirect status code: %d; %s", *statusCode, httpRedirectStatusCodesAllowedMsg)
	}
}

func parseRedirectStatusCodeAnnotation(
	value string,
	ruleName *gwv1.SectionName,
) (*int, error) {
	if value == "" {
		return nil, errors.New("missing value")
	}

	perRuleVal := strings.SplitSeq(value, ",")
	for v := range perRuleVal {
		code, err := parseRedirectStatusCode(strings.TrimSpace(v), ruleName)
		if err != nil {
			return nil, err
		}
		if code != nil {
			return code, nil
		}
	}
	return nil, nil
}

func parseRedirectStatusCode(
	val string,
	ruleName *gwv1.SectionName,
) (*int, error) {
	ruleVals := strings.Split(val, "=")

	var forRule string
	if len(ruleVals) == 2 {
		forRule = ruleVals[0]
		val = strings.TrimSpace(ruleVals[1])
	}

	if ruleName != nil && forRule != "" && forRule != string(*ruleName) {
		// The annotation does not apply to this rule specified by ruleName
		return nil, nil
	}

	code, err := strconv.Atoi(val)
	if err != nil {
		return nil, fmt.Errorf("invalid redirect status code: %s; %s", val, httpRedirectStatusCodesAllowedMsg)
	}

	return &code, nil
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
	// Note: we intentionally do not use policy.IsSettable() for mirrors,
	// unlike single-value fields (redirect, timeouts, retry), as mirrors are
	// append-only and cumulative to support multiple mirrors from the same HTTPRoute rule.
	if outputRoute == nil || outputRoute.GetRoute() == nil {
		return
	}
	mirror := &envoyroutev3.RouteAction_RequestMirrorPolicy{
		Cluster:         m.Cluster,
		RuntimeFraction: m.RuntimeFraction,
	}
	outputRoute.GetRoute().RequestMirrorPolicies = append(outputRoute.GetRoute().GetRequestMirrorPolicies(), mirror)
}

func convertMirrorIR(
	kctx krt.HandlerContext,
	f *gwv1.HTTPRequestMirrorFilter,
	fromgk schema.GroupKind,
	fromns string,
	refgrants *RefGrantIndex,
	ups *BackendIndex,
) *mirrorIr {
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
				Numerator:   uint32(*f.Percent), //nolint:gosec // G115: percentage values are always non-negative and bounded (0-100)
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
		// use MILLION denominator to maximize precision since arbitrary fractions are allowed.
		return &envoycorev3.RuntimeFractionalPercent{
			DefaultValue: &envoytype.FractionalPercent{
				Numerator:   uint32(ratio * 1000000),
				Denominator: envoytype.FractionalPercent_MILLION,
			},
		}
	}

	// nil means 100%
	return nil
}

func NewGatewayTranslationPass(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass {
	return &builtinPluginGwPass{
		reporter:                    reporter,
		hasCorsPolicy:               make(map[string]bool),
		needStatefulSession:         make(map[string]bool),
		sessionPersistenceOverrides: make(map[sessionPersistenceOverrideKey]sets.Set[string]),
		sessionPersistenceClaimed:   make(map[*envoyroutev3.Route]*anypb.Any),
		absoluteTimeoutIgnored:      make(map[absoluteTimeoutIgnoredKey]sets.Set[string]),
	}
}

// reportSessionPersistenceOverridden reports, on the route's parent status, a route rule whose sessionPersistence
// is overridden by the BackendConfigPolicy of some or all of its backends. It sets kgateway.dev/Programmed=True
// with the SessionPersistenceOverridden reason, unless the condition already reports a problem (False).
func (p *builtinPluginGwPass) reportSessionPersistenceOverridden(
	pCtx *ir.RouteContext,
	outputRoute *envoyroutev3.Route,
	singleBackendOverridden bool,
) {
	if p.reporter == nil || pCtx == nil {
		return
	}
	// The overridden sessionPersistence belongs to the rule that owns this policy: the rule itself, or a
	// parent rule delegating to it. Report on that rule's route.
	owner := ruleOwner(&pCtx.In, pCtx.Policy)
	if owner.Parent == nil || owner.Parent.SourceObject == nil {
		return
	}
	var overridden []string
	if singleBackendOverridden {
		for _, b := range pCtx.In.Backends {
			overridden = append(overridden, backendDisplayName(b.Backend))
		}
	} else {
		// With multiple backends, the backends with a BackendConfigPolicy have the config on their weighted cluster.
		// Return early, without allocating, in the common case where none of them has one.
		clusters := outputRoute.GetRoute().GetWeightedClusters().GetClusters()
		if !slices.ContainsFunc(clusters, func(cw *envoyroutev3.WeightedCluster_ClusterWeight) bool {
			return cw.GetTypedPerFilterConfig()[sessionpersistence.FilterName] != nil
		}) {
			return
		}
		byCluster := make(map[string]ir.BackendRefIR, len(pCtx.In.Backends))
		for _, b := range pCtx.In.Backends {
			byCluster[b.Backend.ClusterName] = b.Backend
		}
		for _, cw := range clusters {
			if cw.GetTypedPerFilterConfig()[sessionpersistence.FilterName] == nil {
				continue
			}
			if b, ok := byCluster[cw.GetName()]; ok {
				overridden = append(overridden, backendDisplayName(b))
			}
		}
	}
	if len(overridden) == 0 {
		return
	}
	slices.Sort(overridden)
	overridden = slices.Compact(overridden)

	rule := owner.RuleName
	if rule == "" {
		rule = strconv.Itoa(owner.RuleIndex)
	}
	src := owner.Parent.SourceObject
	key := sessionPersistenceOverrideKey{
		route:     src.GetUID(),
		routeName: src.GetNamespace() + "/" + src.GetName(),
		parentRef: parentRefKey(owner.ParentRef),
	}
	msgs := p.sessionPersistenceOverrides[key]
	if msgs == nil {
		msgs = sets.New[string]()
		p.sessionPersistenceOverrides[key] = msgs
	}
	msgs.Insert(fmt.Sprintf("rule %s: sessionPersistence is overridden by the BackendConfigPolicy of %s",
		rule, strings.Join(overridden, ", ")))

	parentReport := p.reporter.Route(src).ParentRef(&owner.ParentRef)
	// A dropped or replaced rule reports Programmed=False; never hide it.
	if prr, ok := parentReport.(*reports.ParentRefReport); ok {
		if cond := meta.FindStatusCondition(prr.Conditions, apiconditions.KgatewayConditionProgrammed); cond != nil &&
			cond.Status == metav1.ConditionFalse {
			return
		}
	}
	parentReport.SetCondition(reporter.RouteCondition{
		Type:    gwv1.RouteConditionType(apiconditions.KgatewayConditionProgrammed),
		Status:  metav1.ConditionTrue,
		Reason:  gwv1.RouteConditionReason(apiconditions.KgatewayReasonSessionPersistenceOverridden),
		Message: strings.Join(sets.List(msgs), "; "),
	})
}

// ruleOwner returns the route rule that the builtin rule policy pol belongs to: in itself, or the parent
// rule delegating to it that the policy is inherited from. It returns in when no owner is found.
func ruleOwner(in *ir.HttpRouteRuleMatchIR, pol ir.PolicyIR) *ir.HttpRouteRuleMatchIR {
	for r := in; r != nil; r = r.DelegatingParent {
		for _, att := range r.AttachedPolicies.Policies[ir.VirtualBuiltInGK] {
			if att.PolicyIr == pol {
				return r
			}
		}
	}
	return in
}

// parentRefKey normalizes ref the same way the reports package keys route parent status (by value,
// ignoring the port), so references with equal values but different pointers share a key.
func parentRefKey(ref gwv1.ParentReference) reports.ParentRefKey {
	return reports.ParentRefKey{
		Group: string(ptr.Deref(ref.Group, "")),
		Kind:  string(ptr.Deref(ref.Kind, "")),
		NamespacedName: types.NamespacedName{
			Namespace: string(ptr.Deref(ref.Namespace, "")),
			Name:      string(ref.Name),
		},
		SectionName: string(ptr.Deref(ref.SectionName, "")),
	}
}

func backendDisplayName(b ir.BackendRefIR) string {
	if b.BackendObject == nil {
		return b.ClusterName
	}
	gk := b.BackendObject.GetGroupKind()
	return fmt.Sprintf("%s %s/%s", gk.Kind, b.BackendObject.GetNamespace(), b.BackendObject.GetName())
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
func (p *builtinPluginGwPass) ApplyForRoute(pCtx *ir.RouteContext, outputRoute *envoyroutev3.Route) error {
	pol, ok := pCtx.Policy.(*builtinPlugin)
	if !ok {
		return nil
	}

	mergeOpts := policy.MergeOptions{
		Strategy: policy.GetMergeStrategy(pCtx.InheritedPolicyPriority, false),
	}

	var errs error
	if pol.filter != nil {
		// Only post-process redirects when this policy is allowed to set the
		// route redirect. Lower-priority request-redirect policies still run
		// through ApplyForRoute, but must not recompute PortRedirect for a
		// higher-priority redirect that already won the merge.
		canPostProcessRedirect := outputRoute != nil &&
			pol.filter.filterType == gwv1.HTTPRouteFilterRequestRedirect &&
			policy.IsSettable(outputRoute.GetRedirect(), mergeOpts)
		pol.filter.apply(outputRoute, mergeOpts)
		if canPostProcessRedirect {
			applyRedirectPortPostProcessing(pCtx, pol, outputRoute)
		}
	}

	p.applyRulePolicy(pCtx, pol.rule, mergeOpts, outputRoute)
	if pol.hasCors {
		p.hasCorsPolicy[pCtx.FilterChainName] = true
	}

	return errs
}

func (p *builtinPluginGwPass) ApplyForRouteBackend(
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

func (p *builtinPluginGwPass) HttpFilters(_ ir.HttpFiltersContext, fcc ir.FilterChainCommon) ([]filters.StagedHttpFilter, error) {
	builtinStaged := []filters.StagedHttpFilter{}

	// If there is a cors policy for route rule or backendRef, add the cors http filter to the chain
	if p.hasCorsPolicy[fcc.FilterChainName] {
		stagedFilter, err := filters.NewStagedFilter(envoy_wellknown.CORS, &corsv3.Cors{}, filters.DuringStage(filters.CorsStage))
		if err != nil {
			return nil, err
		}
		stagedFilter.Filter.Disabled = true
		builtinStaged = append(builtinStaged, stagedFilter)
	}

	if p.needStatefulSession[fcc.FilterChainName] {
		builtinStaged = append(builtinStaged, sessionpersistence.DisabledFilter())
	}

	return builtinStaged, nil
}

// convertFilterIR converts the HTTPRouteFilter to the IR.
func (h *RoutesIndex) convertfilterIR(
	kctx krt.HandlerContext,
	f gwv1.HTTPRouteFilter,
	fromgk schema.GroupKind,
	fromns string,
	refgrants *RefGrantIndex,
	ups *BackendIndex,
	ruleName *gwv1.SectionName,
	annotations map[string]string,
) (*filterIR, error) {
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
		rr, err := convertRequestRedirectIR(kctx, f.RequestRedirect, ruleName, annotations)
		if err != nil {
			return nil, err
		}
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
		return nil, nil
	}
	return &filterIR{
		filterType: f.Type,
		policy:     policy,
	}, nil
}

type requestRedirectIr struct {
	// Redir is the base redirect action (hostname, status code, scheme rewrite,
	// path rewrite). PortRedirect is intentionally left unset here and resolved
	// per listener in applyRedirectPortPostProcessing, because the same IR is
	// reused across every listener the HTTPRoute attaches to.
	Redir *envoyroutev3.RedirectAction
	// Scheme and Port preserve the user's raw intent from the
	// HTTPRequestRedirectFilter so that port resolution can distinguish
	// "user omitted this field" from "user picked a default value".
	Scheme *string
	Port   *gwv1.PortNumber
}

func (r *requestRedirectIr) apply(
	outputRoute *envoyroutev3.Route,
	mergeOpts policy.MergeOptions,
) {
	if outputRoute == nil || !policy.IsSettable(outputRoute.GetRedirect(), mergeOpts) {
		return
	}
	// Clone so each Envoy route owns its RedirectAction. The same HttpRouteIR (and thus the
	// same requestRedirectIr) is applied across multiple listeners; applyRedirectPortPostProcessing
	// mutates PortRedirect per listener and must not update shared IR state.
	redir := proto.Clone(r.Redir).(*envoyroutev3.RedirectAction)
	outputRoute.Action = &envoyroutev3.Route_Redirect{
		Redirect: redir,
	}
}

func convertRequestRedirectIR(
	_ krt.HandlerContext,
	f *gwv1.HTTPRequestRedirectFilter,
	ruleName *gwv1.SectionName,
	annotations map[string]string,
) (*requestRedirectIr, error) {
	if f == nil {
		return nil, nil
	}

	statusCode, err := translateStatusCode(f.StatusCode, ruleName, annotations)
	if err != nil {
		return nil, err
	}

	redir := &envoyroutev3.RedirectAction{
		HostRedirect: translateHostname(f.Hostname),
		ResponseCode: statusCode,
		// PortRedirect is intentionally left unset; see applyRedirectPortPostProcessing.
	}
	translateScheme(redir, f.Scheme)
	translatePathRewrite(redir, f.Path)

	return &requestRedirectIr{
		Redir:  redir,
		Scheme: f.Scheme,
		Port:   f.Port,
	}, nil
}

// applyRedirectPortPostProcessing resolves the redirect PortRedirect for the
// current listener per the Gateway API spec:
//
//   - If filter.Port is set, use it.
//   - Else if filter.Scheme is set, use the scheme's well-known port
//     (http=80, https=443).
//   - Else, use the Gateway Listener port.
//
// Per the spec the port is omitted from the Location header when the effective
// port matches the effective scheme's well-known default (http+80, https+443).
// In Envoy, leaving PortRedirect at 0 omits the port from the Location header.
func applyRedirectPortPostProcessing(
	pCtx *ir.RouteContext,
	pol *builtinPlugin,
	outputRoute *envoyroutev3.Route,
) {
	if pol.filter.filterType != gwv1.HTTPRouteFilterRequestRedirect {
		return
	}
	redirectIr, ok := pol.filter.policy.(*requestRedirectIr)
	if !ok {
		return
	}
	redirect := outputRoute.GetRedirect()
	if redirect == nil {
		return
	}

	// Effective scheme: filter.Scheme if set, otherwise inferred from the listener.
	effectiveScheme := strings.ToLower(ptr.Deref(redirectIr.Scheme, ""))
	if effectiveScheme == "" {
		if pCtx.ListenerHasTLS {
			effectiveScheme = "https"
		} else {
			effectiveScheme = "http"
		}
	}

	// Effective port: filter.Port > scheme default > listener port.
	var effectivePort uint32
	switch {
	case redirectIr.Port != nil:
		effectivePort = uint32(*redirectIr.Port) //nolint:gosec // G115: Gateway API PortNumber is int32 in valid port range
	case redirectIr.Scheme != nil:
		effectivePort = defaultPortForScheme(effectiveScheme)
	default:
		effectivePort = pCtx.ListenerPort
	}

	// Omit the port when it matches the scheme's well-known default.
	if effectivePort == defaultPortForScheme(effectiveScheme) {
		redirect.PortRedirect = 0
		return
	}
	redirect.PortRedirect = effectivePort
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
	if u.FullReplace != "" && isPathRewriteSettable(outputRoute, mergeOpts) {
		outputRoute.GetRoute().RegexRewrite = &envoy_type_matcher_v3.RegexMatchAndSubstitute{
			Pattern: &envoy_type_matcher_v3.RegexMatcher{
				Regex: ".*",
			},
			Substitution: u.FullReplace,
		}
	}

	// need to check that both RegexRewrite and PrefixRewrite are settable
	// cannot set both RegexRewrite and PrefixRewrite; Envoy will reject it
	if u.PrefixReplace != "" && isPathRewriteSettable(outputRoute, mergeOpts) {
		path := outputRoute.GetMatch().GetPrefix()
		if path == "" {
			path = outputRoute.GetMatch().GetPath()
		}
		if path == "" {
			path = outputRoute.GetMatch().GetPathSeparatedPrefix()
		}

		if path != "" && u.PrefixReplace == "/" {
			if outputRoute.GetRoute().GetPrefixRewrite() != "" {
				// clear the PrefixRewrite in order to override it with the RegexRewrite
				outputRoute.GetRoute().PrefixRewrite = ""
				//nolint:sloglint // keep the existing log key; renaming it would break log queries
				logger.Debug("overriding PrefixRewrite with RegexRewrite", "path", path, "prefixReplace", u.PrefixReplace)
			}
			outputRoute.GetRoute().RegexRewrite = &envoy_type_matcher_v3.RegexMatchAndSubstitute{
				Pattern: &envoy_type_matcher_v3.RegexMatcher{
					Regex: "^" + path + "\\/*",
				},
				Substitution: "/",
			}
		} else {
			if outputRoute.GetRoute().GetRegexRewrite() != nil {
				// clear the RegexRewrite in order to override it with the PrefixRewrite
				outputRoute.GetRoute().RegexRewrite = nil
				//nolint:sloglint // keep the existing log key; renaming it would break log queries
				logger.Debug("overriding RegexRewrite with PrefixRewrite", "path", path, "prefixReplace", u.PrefixReplace)
			}
			outputRoute.GetRoute().PrefixRewrite = u.PrefixReplace
		}
	}
}

// isPathRewriteSettable returns true if both RegexRewrite and PrefixRewrite are settable
func isPathRewriteSettable(route *envoyroutev3.Route, mergeOpts policy.MergeOptions) bool {
	return policy.IsSettable(route.GetRoute().GetRegexRewrite(), mergeOpts) &&
		policy.IsSettable(route.GetRoute().GetPrefixRewrite(), mergeOpts)
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
