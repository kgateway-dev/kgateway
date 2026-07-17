package trafficpolicy

import (
	"regexp"
	"strings"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// statPrefixTemplateVar matches a well-formed `{{ var }}` template token,
// capturing the variable name (whitespace inside the braces is ignored). The
// set of valid variable names and the overall value syntax are enforced at
// admission by the CRD schema (see the StatPrefix field's Pattern); this
// matcher is only used to substitute the tokens during translation.
var statPrefixTemplateVar = regexp.MustCompile(`{{\s*([a-zA-Z_]+)\s*}}`)

// Supported template variable names for the route stat prefix.
const (
	statPrefixVarRouteName      = "route_name"
	statPrefixVarRouteNamespace = "route_namespace"
	statPrefixVarRuleName       = "rule_name"
)

type statPrefixIR struct {
	// template is the raw, unresolved stat prefix as authored by the user. It
	// may contain `{{...}}` template tokens that are substituted per-route in
	// ApplyForRoute (route metadata is not known at IR construction time).
	template string
}

var _ PolicySubIR = &statPrefixIR{}

func (s *statPrefixIR) Equals(other PolicySubIR) bool {
	otherStatPrefix, ok := other.(*statPrefixIR)
	if !ok {
		return false
	}
	if s == nil && otherStatPrefix == nil {
		return true
	}
	if s == nil || otherStatPrefix == nil {
		return false
	}
	return s.template == otherStatPrefix.template
}

// Validate is a no-op: the stat prefix syntax (allowed characters, well-formed
// template tokens, and the set of supported variable names) is fully enforced
// at admission by the CRD schema (see the StatPrefix field's Pattern).
func (s *statPrefixIR) Validate() error { return nil }

// constructStatPrefix constructs the stat prefix policy IR from the policy specification.
func constructStatPrefix(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.StatPrefix == nil {
		return
	}
	out.statPrefix = &statPrefixIR{
		template: *spec.StatPrefix,
	}
}

// applyStatPrefix resolves the stat prefix template using the route metadata
// available in pCtx and sets it on the Envoy route. It is set on the Route
// itself (not the RouteAction) so it also applies to redirect and
// direct-response routes.
func applyStatPrefix(sp *statPrefixIR, pCtx *ir.RouteContext, out *envoyroutev3.Route) {
	if sp == nil || out == nil {
		return
	}

	var routeName, routeNamespace string
	if parent := pCtx.In.Parent; parent != nil {
		routeName = parent.Name
		routeNamespace = parent.Namespace
	}
	ruleName := pCtx.In.RuleName

	resolved := statPrefixTemplateVar.ReplaceAllStringFunc(sp.template, func(token string) string {
		match := statPrefixTemplateVar.FindStringSubmatch(token)
		switch match[1] {
		case statPrefixVarRouteName:
			return routeName
		case statPrefixVarRouteNamespace:
			return routeNamespace
		case statPrefixVarRuleName:
			return ruleName
		default:
			// Admission rejects unsupported variables, so this is unreachable;
			// leave the token untouched to avoid silently dropping content.
			return token
		}
	})

	// A template variable (e.g. rule_name for an unnamed rule) can resolve to an
	// empty string. '.' delimits segments in Envoy stat names, so drop any empty
	// '.'-separated segments this leaves behind (leading, trailing, or in the
	// middle) to avoid doubled or trailing dots, which produce malformed stat
	// names and blank levels in dot-hierarchy sinks such as statsd/Graphite.
	out.StatPrefix = joinNonEmptyStatSegments(resolved)
}

// joinNonEmptyStatSegments splits s on the Envoy stat separator ('.') and
// rejoins the non-empty segments, collapsing empty segments anywhere in the
// string.
func joinNonEmptyStatSegments(s string) string {
	segments := strings.Split(s, ".")
	nonEmpty := segments[:0]
	for _, segment := range segments {
		if segment != "" {
			nonEmpty = append(nonEmpty, segment)
		}
	}
	return strings.Join(nonEmpty, ".")
}
