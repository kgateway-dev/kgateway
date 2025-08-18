package trafficpolicy

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_csrf_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

const (
	csrfExtensionFilterName = "envoy.filters.http.csrf"
	csrfFilterEnabledKey    = "envoy.csrf.filter_enabled"
	csrfShadowEnabledKey    = "envoy.csrf.shadow_enabled"
)

type csrfIR struct {
	policy *envoy_csrf_v3.CsrfPolicy
}

var _ PolicySubIR = &csrfIR{}

func (c *csrfIR) Equals(other PolicySubIR) bool {
	otherCsrf, ok := other.(*csrfIR)
	if !ok {
		return false
	}
	if c == nil && otherCsrf == nil {
		return true
	}
	if c == nil || otherCsrf == nil {
		return false
	}
	return proto.Equal(c.policy, otherCsrf.policy)
}

func (c *csrfIR) Validate() error {
	if c == nil || c.policy == nil {
		return nil
	}
	return c.policy.Validate()
}

// handleCsrf adds CSRF configuration to routes
func (p *trafficPolicyPluginGwPass) handleCsrf(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, ir *csrfIR) {
	if typedFilterConfig == nil || ir == nil {
		return
	}
	typedFilterConfig.AddTypedConfig(csrfExtensionFilterName, ir.policy)

	// Add a filter to the chain. When having a csrf for a route we need to also have a
	// globally disabled csrf filter in the chain otherwise it will be ignored.
	// If there is also csrf for the listener, it will not override this one.
	if p.csrfInChain == nil {
		p.csrfInChain = make(map[string]*envoy_csrf_v3.CsrfPolicy)
	}
	if _, ok := p.csrfInChain[fcn]; !ok {
		p.csrfInChain[fcn] = &envoy_csrf_v3.CsrfPolicy{
			// FilterEnabled is a required value
			FilterEnabled: &envoycorev3.RuntimeFractionalPercent{
				DefaultValue: &envoy_type_v3.FractionalPercent{
					Numerator:   0,
					Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
				},
			},
		}
	}
}

// constructCSRF constructs the CSRF policy IR from the policy specification.
func constructCSRF(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) error {
	if spec.Csrf == nil {
		return nil
	}

	csrfPolicy := &envoy_csrf_v3.CsrfPolicy{}

	// Set filter enabled percentage
	numerator := uint32(0)
	if spec.Csrf.PercentageEnabled != nil {
		numerator = *spec.Csrf.PercentageEnabled
	}

	// FilterEnabled is required by the envoy filter and is set to 0 (off) by default
	csrfPolicy.FilterEnabled = &envoycorev3.RuntimeFractionalPercent{
		DefaultValue: &envoy_type_v3.FractionalPercent{
			Numerator:   numerator,
			Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
		},
		RuntimeKey: csrfFilterEnabledKey,
	}

	// Set shadow enabled percentage if specified
	if spec.Csrf.PercentageShadowed != nil {
		csrfPolicy.ShadowEnabled = &envoycorev3.RuntimeFractionalPercent{
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

	out.csrf = &csrfIR{
		policy: csrfPolicy,
	}
	return nil
}

func toEnvoyStringMatcher(origin v1alpha1.StringMatcher) *envoy_matcher_v3.StringMatcher {
	matcher := &envoy_matcher_v3.StringMatcher{
		IgnoreCase: origin.IgnoreCase,
	}

	switch {
	case origin.Exact != nil:
		matcher.MatchPattern = &envoy_matcher_v3.StringMatcher_Exact{
			Exact: *origin.Exact,
		}
	case origin.Prefix != nil:
		matcher.MatchPattern = &envoy_matcher_v3.StringMatcher_Prefix{
			Prefix: *origin.Prefix,
		}
	case origin.Suffix != nil:
		matcher.MatchPattern = &envoy_matcher_v3.StringMatcher_Suffix{
			Suffix: *origin.Suffix,
		}
	case origin.Contains != nil:
		matcher.MatchPattern = &envoy_matcher_v3.StringMatcher_Contains{
			Contains: *origin.Contains,
		}
	case origin.SafeRegex != nil:
		matcher.MatchPattern = &envoy_matcher_v3.StringMatcher_SafeRegex{
			SafeRegex: &envoy_matcher_v3.RegexMatcher{
				EngineType: &envoy_matcher_v3.RegexMatcher_GoogleRe2{
					GoogleRe2: &envoy_matcher_v3.RegexMatcher_GoogleRE2{},
				},
				Regex: *origin.SafeRegex,
			},
		}
	default:
		// Shouldn't happen because we validate that one and only one matching type is set
		return nil
	}

	return matcher
}
