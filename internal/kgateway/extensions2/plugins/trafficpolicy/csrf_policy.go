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
			csrfPolicy.GetAdditionalOrigins()[i] = &envoy_matcher_v3.StringMatcher{
				MatchPattern: &envoy_matcher_v3.StringMatcher_Exact{
					Exact: origin,
				},
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
