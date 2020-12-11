package csrf

import (
	"github.com/rotisserie/eris"
	csrf "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	gloo_type_matcher "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"

	envoy_config_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycsrf "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	envoy_type_matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

// filter should be called after routing decision has been made
var pluginStage = plugins.DuringStage(plugins.RouteStage)

const FilterName = "envoy.filters.http.csrf"

func NewPlugin() *plugin {
	return &plugin{}
}

var _ plugins.Plugin = new(plugin)
var _ plugins.HttpFilterPlugin = new(plugin)
var _ plugins.WeightedDestinationPlugin = new(plugin)
var _ plugins.VirtualHostPlugin = new(plugin)
var _ plugins.RoutePlugin = new(plugin)

type plugin struct {
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) HttpFilters(_ plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {

	csrfConfig := listener.GetOptions().GetCsrf()

	if csrfConfig == nil {
		return nil, nil
	}

	csrfFilter, err := plugins.NewStagedFilterWithConfig(FilterName, csrfConfig, pluginStage)
	if err != nil {
		return nil, eris.Wrapf(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{csrfFilter}, nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route.Route) error {
	csrfPolicy := in.Options.GetCsrf()
	if csrfPolicy == nil {
		return nil
	}

	if csrfPolicy.GetAdditionalOrigins() != nil || csrfPolicy.GetFilterEnabled() != nil || csrfPolicy.GetShadowEnabled() != nil {
		config := getCsrfConfig(csrfPolicy)
		return pluginutils.SetRoutePerFilterConfig(out, "envoy.filters.http.csrf", config)
	}

	return nil
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route.VirtualHost,
) error {
	csrfPolicy := in.Options.GetCsrf()
	if csrfPolicy == nil {
		return nil
	}

	if csrfPolicy.GetAdditionalOrigins() != nil || csrfPolicy.GetFilterEnabled() != nil || csrfPolicy.GetShadowEnabled() != nil {
		config := getCsrfConfig(csrfPolicy)
		return pluginutils.SetVhostPerFilterConfig(out, "envoy.filters.http.csrf", config)
	}

	return nil
}

func (p *plugin) ProcessWeightedDestination(
	params plugins.RouteParams,
	in *v1.WeightedDestination,
	out *envoy_config_route.WeightedCluster_ClusterWeight,
) error {
	csrfPolicy := in.Options.GetCsrf()
	if csrfPolicy == nil {
		return nil
	}

	if csrfPolicy.GetAdditionalOrigins() != nil || csrfPolicy.GetFilterEnabled() != nil || csrfPolicy.GetShadowEnabled() != nil {
		config := getCsrfConfig(csrfPolicy)
		return pluginutils.SetWeightedClusterPerFilterConfig(out, "envoy.filters.http.csrf", config)
	}

	return nil
}

func getCsrfConfig(csrf *csrf.CsrfPolicy) *envoycsrf.CsrfPolicy {
	origins := csrf.GetAdditionalOrigins()
	var additionalOrigins []*envoy_type_matcher.StringMatcher
	for _, ao := range origins {
		switch typed := ao.GetMatchPattern().(type) {
		case *gloo_type_matcher.StringMatcher_Exact:
			additionalOrigins = append(additionalOrigins, &envoy_type_matcher.StringMatcher{
				MatchPattern: &envoy_type_matcher.StringMatcher_Exact{
					Exact: typed.Exact,
				},
				IgnoreCase: ao.GetIgnoreCase(),
			})
		case *gloo_type_matcher.StringMatcher_Prefix:
			additionalOrigins = append(additionalOrigins, &envoy_type_matcher.StringMatcher{
				MatchPattern: &envoy_type_matcher.StringMatcher_Prefix{
					Prefix: typed.Prefix,
				},
				IgnoreCase: ao.GetIgnoreCase(),
			})
		case *gloo_type_matcher.StringMatcher_SafeRegex:
			additionalOrigins = append(additionalOrigins, &envoy_type_matcher.StringMatcher{
				MatchPattern: &envoy_type_matcher.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher.RegexMatcher{
						EngineType: &envoy_type_matcher.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher.RegexMatcher_GoogleRE2{},
						},
						Regex: typed.SafeRegex.GetRegex(),
					},
				},
				IgnoreCase: ao.GetIgnoreCase(),
			})
		case *gloo_type_matcher.StringMatcher_Suffix:
			additionalOrigins = append(additionalOrigins, &envoy_type_matcher.StringMatcher{
				MatchPattern: &envoy_type_matcher.StringMatcher_Suffix{
					Suffix: typed.Suffix,
				},
				IgnoreCase: ao.GetIgnoreCase(),
			})
		}

	}

	return &envoycsrf.CsrfPolicy{
		FilterEnabled: &envoy_config_core.RuntimeFractionalPercent{
			DefaultValue: &envoytype.FractionalPercent{
				Numerator:   csrf.GetFilterEnabled().GetDefaultValue().GetNumerator(),
				Denominator: envoytype.FractionalPercent_DenominatorType(csrf.GetFilterEnabled().GetDefaultValue().GetDenominator()),
			},
			RuntimeKey: csrf.GetFilterEnabled().GetRuntimeKey(),
		},
		ShadowEnabled: &envoy_config_core.RuntimeFractionalPercent{
			DefaultValue: &envoytype.FractionalPercent{
				Numerator:   csrf.GetShadowEnabled().GetDefaultValue().GetNumerator(),
				Denominator: envoytype.FractionalPercent_DenominatorType(csrf.GetShadowEnabled().GetDefaultValue().GetDenominator()),
			},
			RuntimeKey: csrf.GetShadowEnabled().GetRuntimeKey(),
		},
		AdditionalOrigins: additionalOrigins,
	}
}
