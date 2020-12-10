package csrf

import (
	"github.com/rotisserie/eris"
	csrf "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	v31 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycsrf "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

// filter should be called after routing decision has been made
var pluginStage = plugins.DuringStage(plugins.RouteStage)

const FilterName = "envoy.filters.http.csrf"

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.HttpFilterPlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) HttpFilters(_ plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {

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

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	csrfPolicy := in.Options.GetCsrf()
	if csrfPolicy == nil {
		return nil
	}

	additionalOrigins := csrfPolicy.GetAdditionalOrigins()
	filtersEnabled := csrfPolicy.GetFilterEnabled()
	shadowEnabled := csrfPolicy.GetShadowEnabled()
	if additionalOrigins != nil || filtersEnabled != nil || shadowEnabled != nil  {
		config := getCsrfConfig(csrfPolicy)
		return pluginutils.SetRoutePerFilterConfig(out, "envoy.filters.http.buffer", config)
	}

	return nil
}

func (p *Plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	csrfPolicy := in.Options.GetCsrf()
	if csrfPolicy == nil {
		return nil
	}

	additionalOrigins := csrfPolicy.GetAdditionalOrigins()
	filtersEnabled := csrfPolicy.GetFilterEnabled()
	shadowEnabled := csrfPolicy.GetShadowEnabled()
	if additionalOrigins != nil || filtersEnabled != nil || shadowEnabled != nil  {
		config := getCsrfConfig(csrfPolicy)
		return pluginutils.SetVhostPerFilterConfig(out, "envoy.filters.http.buffer", config)
	}

	return nil
}

func (p *Plugin) ProcessWeightedDestination(
	params plugins.RouteParams,
	in *v1.WeightedDestination,
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
) error {
	csrfPolicy := in.Options.GetCsrf()
	if csrfPolicy == nil {
		return nil
	}

	additionalOrigins := csrfPolicy.GetAdditionalOrigins()
	filtersEnabled := csrfPolicy.GetFilterEnabled()
	shadowEnabled := csrfPolicy.GetShadowEnabled()
	if additionalOrigins != nil || filtersEnabled != nil || shadowEnabled != nil  {
		config := getCsrfConfig(csrfPolicy)
		return pluginutils.SetWeightedClusterPerFilterConfig(out, "envoy.filters.http.buffer", config)
	}

	return nil
}

func getCsrfConfig(csrf *csrf.CsrfPolicy) *envoycsrf.CsrfPolicy {
	origins := csrf.GetAdditionalOrigins()
	var additionalOrigins []*envoy_type_matcher_v3.StringMatcher
	for _, ao := range origins {
		switch typed := ao.GetMatchPattern().(type) {
			case *v31.StringMatcher_Exact:
				additionalOrigins = append(additionalOrigins, &envoy_type_matcher_v3.StringMatcher{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
						Exact: typed.Exact,
					},
					IgnoreCase: ao.GetIgnoreCase(),
				})
			case *v31.StringMatcher_Prefix:
				additionalOrigins = append(additionalOrigins, &envoy_type_matcher_v3.StringMatcher{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
						Prefix: typed.Prefix,
					},
					IgnoreCase: ao.GetIgnoreCase(),
				})
			case *v31.StringMatcher_SafeRegex:
				additionalOrigins = append(additionalOrigins, &envoy_type_matcher_v3.StringMatcher{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_SafeRegex{
						SafeRegex: &envoy_type_matcher_v3.RegexMatcher{
							EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
								GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{},
							},
							Regex: typed.SafeRegex.GetRegex(),
						},
					},
					IgnoreCase: ao.GetIgnoreCase(),
				})
			case *v31.StringMatcher_Suffix:
				additionalOrigins = append(additionalOrigins, &envoy_type_matcher_v3.StringMatcher{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_Suffix{
						Suffix: typed.Suffix,
					},
					IgnoreCase: ao.GetIgnoreCase(),
				})
		}


	}

	return &envoycsrf.CsrfPolicy{
		FilterEnabled: &envoy_config_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoytype.FractionalPercent{
				Numerator: csrf.GetFilterEnabled().GetDefaultValue().GetNumerator(),
				Denominator: envoytype.FractionalPercent_DenominatorType(csrf.GetFilterEnabled().GetDefaultValue().GetDenominator()),
			},
			RuntimeKey: csrf.GetFilterEnabled().GetRuntimeKey(),
		},
		ShadowEnabled: &envoy_config_core_v3.RuntimeFractionalPercent{
			DefaultValue: &envoytype.FractionalPercent{
				Numerator: csrf.GetShadowEnabled().GetDefaultValue().GetNumerator(),
				Denominator: envoytype.FractionalPercent_DenominatorType(csrf.GetShadowEnabled().GetDefaultValue().GetDenominator()),
			},
			RuntimeKey: csrf.GetShadowEnabled().GetRuntimeKey(),
		},
		AdditionalOrigins: additionalOrigins,
	}
}
