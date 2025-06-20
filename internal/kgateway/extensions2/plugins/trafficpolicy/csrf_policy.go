package trafficpolicy

import (
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
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

// handleCsrf adds CSRF configuration to routes
func (p *trafficPolicyPluginGwPass) handleCsrf(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, ir *CsrfIR) {
	if typedFilterConfig == nil || ir == nil {
		return
	}
	typedFilterConfig.AddTypedConfig(csrfExtensionFilterName, ir.csrfPolicy)

	// Add a filter to the chain. When having a csrf for a route we need to also have a
	// globally disabled csrf filter in the chain otherwise it will be ignored.
	// If there is also csrf for the listener, it will not override this one.
	if p.csrfInChain == nil {
		p.csrfInChain = make(map[string]*envoy_csrf_v3.CsrfPolicy)
	}
	if _, ok := p.csrfInChain[fcn]; !ok {
		p.csrfInChain[fcn] = csrfFilter()
	}
}

// csrfForSpec translates the CSRF spec into and onto the IR policy
func csrfForSpec(spec v1alpha1.TrafficPolicySpec, out *trafficPolicySpecIr) error {
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
	csrfPolicy.FilterEnabled = &envoy_core_v3.RuntimeFractionalPercent{
		DefaultValue: &envoy_type_v3.FractionalPercent{
			Numerator:   numerator,
			Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
		},
		RuntimeKey: csrfFilterEnabledKey,
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
	if origin == nil {
		return nil
	}

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
