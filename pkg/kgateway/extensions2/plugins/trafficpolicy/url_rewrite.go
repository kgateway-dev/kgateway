package trafficpolicy

import (
	"fmt"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/regexutils"
)

type urlRewriteIR struct {
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

	// Compare regex match
	return proto.Equal(u.regexMatch, otherURLRewrite.regexMatch)
}

// Validate performs validation on the URL rewrite component.
func (u *urlRewriteIR) Validate() error {
	if u == nil {
		return nil
	}
	if u.regexMatch != nil && u.regexMatch.GetPattern() != nil {
		if err := regexutils.CheckRegexString(u.regexMatch.GetPattern().GetRegex()); err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
	}
	return nil
}

// constructURLRewrite constructs the URL rewrite policy IR from the policy specification.
func constructURLRewrite(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.UrlRewrite == nil {
		return
	}

	ir := &urlRewriteIR{}

	if spec.UrlRewrite.PathRegex != nil {
		ir.regexMatch = &envoy_type_matcher_v3.RegexMatchAndSubstitute{
			Pattern: &envoy_type_matcher_v3.RegexMatcher{
				Regex: spec.UrlRewrite.PathRegex.Pattern,
			},
			Substitution: spec.UrlRewrite.PathRegex.Substitution,
		}
	}

	out.urlRewrite = ir
}

// applyURLRewrite applies URL rewrite configuration to the Envoy route.
func applyURLRewrite(urlRewrite *urlRewriteIR, out *envoyroutev3.Route) {
	if urlRewrite == nil || out == nil {
		return
	}

	action := out.GetRoute()
	if action == nil {
		return
	}
	// Apply regex path rewrite
	if urlRewrite.regexMatch != nil {
		// Only apply if not already set
		if action.GetRegexRewrite() == nil && action.GetPrefixRewrite() == "" {
			action.RegexRewrite = urlRewrite.regexMatch
		} else {
			logger.Debug("URL rewrite regex is already set or prefix rewrite is not empty; skipping URL rewrite application")
		}
	}
}
