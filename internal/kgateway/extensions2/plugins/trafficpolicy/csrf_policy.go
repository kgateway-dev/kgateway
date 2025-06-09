package trafficpolicy

import (
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_csrf_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

const (
	csrfExtensionFilterName = "envoy.filters.http.csrf"
	csrfFilterEnabledKey    = "envoy.csrf.filter_enabled"
	csrfShadowEnabledKey    = "envoy.csrf.shadow_enabled"
)

type CsrfIR struct {
	csrfPolicy *envoy_csrf_v3.CsrfPolicy
}

func (c *CsrfIR) Equals(other *CsrfIR) bool {
	if c == nil && other == nil {
		return true
	}
	if c == nil || other == nil {
		return false
	}

	return proto.Equal(c.csrfPolicy, other.csrfPolicy)
}

// csrfForSpec translates the CSRF spec into and onto the IR policy
func csrfForSpec(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) error {
	if spec.Csrf == nil {
		return nil
	}

	csrfPolicy := &envoy_csrf_v3.CsrfPolicy{}

	// Set filter enabled percentage
	if spec.Csrf.PercentageEnabled != nil {
		csrfPolicy.FilterEnabled = &envoy_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   *spec.Csrf.PercentageEnabled,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
			RuntimeKey: csrfFilterEnabledKey,
		}
	}

	// Set shadow enabled percentage if specified
	if spec.Csrf.PercentageShadowed != nil {
		csrfPolicy.ShadowEnabled = &envoy_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   *spec.Csrf.PercentageShadowed,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
			RuntimeKey: csrfShadowEnabledKey,
		}
	}

	// Add additional origins if specified
	if len(spec.Csrf.AdditionalOrigins) > 0 {
		csrfPolicy.AdditionalOrigins = make([]*envoy_matcher_v3.StringMatcher, len(spec.Csrf.AdditionalOrigins))
		for i, origin := range spec.Csrf.AdditionalOrigins {
			envoyStringMatcher := toEnvoyStringMatcher(origin)
			if envoyStringMatcher != nil {
				csrfPolicy.GetAdditionalOrigins()[i] = envoyStringMatcher
			}
		}
	}

	out.csrf = &CsrfIR{
		csrfPolicy: csrfPolicy,
	}
	return nil
}

// csrfFilter returns a default csrf filter with the filter enabled percentage set to 0 to be added to the filter
// chain.
func csrfFilter() *envoy_csrf_v3.CsrfPolicy {
	return &envoy_csrf_v3.CsrfPolicy{
		FilterEnabled: &envoy_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   0,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
	}
}

func toEnvoyStringMatcher(origin *v1alpha1.StringMatcher) *envoy_matcher_v3.StringMatcher {
	if origin.Exact != "" {
		return &envoy_matcher_v3.StringMatcher{
			MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
				Exact: origin.Exact,
			},
		}
	}

	if origin.Prefix != "" {
		return &envoy_matcher_v3.StringMatcher{
			MatchPattern: &envoy_matcher_v3.StringMatcher_Prefix{
				Prefix: origin.Prefix,
			},
		}
	}

	if origin.Suffix != "" {
		return &envoy_matcher_v3.StringMatcher{
			MatchPattern: &envoy_matcher_v3.StringMatcher_Suffix{
				Suffix: origin.Suffix,
			},
		}
	}

	if origin.SafeRegex != "" {
		return &envoy_matcher_v3.StringMatcher{
			MatchPattern: &envoy_matcher_v3.StringMatcher_SafeRegex{
				SafeRegex: &envoy_matcher_v3.RegexMatcher{
					EngineType: &envoy_matcher_v3.RegexMatcher_GoogleRe2{
						GoogleRe2: &envoy_matcher_v3.RegexMatcher_GoogleRE2{},
					},
					Regex: origin.SafeRegex,
				},
			},
		}
	}

	// Shouldn't happen because we validate that only one matching type is set
	return nil
}
