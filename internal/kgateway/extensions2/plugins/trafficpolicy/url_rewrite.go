package trafficpolicy

import (
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

type urlRewriteIR struct {
	hostname   *string
	regexMatch *envoy_type_matcher_v3.RegexMatchAndSubstitute
}

var _ PolicySubIR = &urlRewriteIR{}

func (u *urlRewriteIR) Equals(other PolicySubIR) bool {
	otherURLRewrite, ok := other.(*urlRewriteIR)
	if !ok {
		return false
	}
	if u == nil && otherURLRewrite == nil {
		return true
	}
	if u == nil || otherURLRewrite == nil {
		return false
	}
	// Compare hostname
	if u.hostname == nil && otherURLRewrite.hostname != nil {
		return false
	}
	if u.hostname != nil && otherURLRewrite.hostname == nil {
		return false
	}
	if u.hostname != nil && otherURLRewrite.hostname != nil && *u.hostname != *otherURLRewrite.hostname {
		return false
	}
	// Compare regex match
	return proto.Equal(u.regexMatch, otherURLRewrite.regexMatch)
}

// Validate performs validation on the URL rewrite component.
func (u *urlRewriteIR) Validate() error { return nil }

// constructURLRewrite constructs the URL rewrite policy IR from the policy specification.
func constructURLRewrite(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.UrlRewrite == nil {
		return
	}

	ir := &urlRewriteIR{}

	if spec.UrlRewrite.Hostname != nil {
		ir.hostname = spec.UrlRewrite.Hostname
	}

	if spec.UrlRewrite.Path != nil {
		ir.regexMatch = &envoy_type_matcher_v3.RegexMatchAndSubstitute{
			Pattern: &envoy_type_matcher_v3.RegexMatcher{
				Regex: spec.UrlRewrite.Path.Pattern,
			},
			Substitution: spec.UrlRewrite.Path.Substitution,
		}
	}

	out.urlRewrite = ir
}

// applyURLRewrite applies URL rewrite configuration to the Envoy route.
func applyURLRewrite(urlRewrite *urlRewriteIR, out *envoyroutev3.Route) {
	if urlRewrite == nil || out == nil || out.GetRoute() == nil {
		return
	}

	action := out.GetRoute()

	// Apply hostname rewrite
	if urlRewrite.hostname != nil {
		// Only apply if not already set
		if action.GetHostRewriteSpecifier() == nil {
			action.HostRewriteSpecifier = &envoyroutev3.RouteAction_HostRewriteLiteral{
				HostRewriteLiteral: *urlRewrite.hostname,
			}
		}
	}

	// Apply regex path rewrite
	if urlRewrite.regexMatch != nil {
		// Only apply if not already set
		if action.GetRegexRewrite() == nil && action.GetPrefixRewrite() == "" {
			action.RegexRewrite = urlRewrite.regexMatch
		}
	}
}
