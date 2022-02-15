package csrf

import (
	envoy_config_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycsrf "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	envoy_type_matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/rotisserie/eris"
	gloo_config_core "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	csrf "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	gloo_type_matcher "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	glootype "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var (
	_ plugins.Plugin                    = new(plugin)
	_ plugins.HttpFilterPlugin          = new(plugin)
	_ plugins.WeightedDestinationPlugin = new(plugin)
	_ plugins.VirtualHostPlugin         = new(plugin)
	_ plugins.RoutePlugin               = new(plugin)
)

const (
	ExtensionName = "csrf"
	FilterName    = "envoy.filters.http.csrf"
)

// filter should be called after routing decision has been made
var pluginStage = plugins.DuringStage(plugins.RouteStage)

type plugin struct {
	filterNeeded bool
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) error {
	p.filterNeeded = !params.Settings.GetGloo().GetRemoveUnusedFilters().GetValue()
	return nil
}

func (p *plugin) HttpFilters(_ plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if !p.filterNeeded {
		return []plugins.StagedHttpFilter{}, nil
	}

	glooCsrfConfig := listener.GetOptions().GetCsrf()
	envoyCsrfConfig, err := translateCsrfConfig(glooCsrfConfig)
	if err != nil {
		return nil, err
	}

	csrfFilter, err := plugins.NewStagedFilterWithConfig(FilterName, envoyCsrfConfig, pluginStage)
	if err != nil {
		return nil, eris.Wrap(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{csrfFilter}, nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route.Route) error {
	csrfPolicy := in.GetOptions().GetCsrf()
	if csrfPolicy == nil {
		return nil
	}

	envoyCsrfConfig, err := translateCsrfConfig(csrfPolicy)
	if err != nil {
		return err
	}
	p.filterNeeded = true

	return pluginutils.SetRoutePerFilterConfig(out, FilterName, envoyCsrfConfig)
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route.VirtualHost,
) error {
	csrfPolicy := in.GetOptions().GetCsrf()
	if csrfPolicy == nil {
		return nil
	}

	envoyCsrfConfig, err := translateCsrfConfig(csrfPolicy)
	if err != nil {
		return err
	}
	p.filterNeeded = true

	return pluginutils.SetVhostPerFilterConfig(out, FilterName, envoyCsrfConfig)
}

func (p *plugin) ProcessWeightedDestination(
	params plugins.RouteParams,
	in *v1.WeightedDestination,
	out *envoy_config_route.WeightedCluster_ClusterWeight,
) error {
	csrfPolicy := in.GetOptions().GetCsrf()
	if csrfPolicy == nil {
		return nil
	}

	envoyCsrfConfig, err := translateCsrfConfig(csrfPolicy)
	if err != nil {
		return err
	}
	p.filterNeeded = true

	return pluginutils.SetWeightedClusterPerFilterConfig(out, FilterName, envoyCsrfConfig)
}

func translateCsrfConfig(csrf *csrf.CsrfPolicy) (*envoycsrf.CsrfPolicy, error) {
	csrfPolicy := &envoycsrf.CsrfPolicy{
		FilterEnabled:     translateFilterEnabled(csrf.GetFilterEnabled()),
		ShadowEnabled:     translateShadowEnabled(csrf.GetShadowEnabled()),
		AdditionalOrigins: translateAdditionalOrigins(csrf.GetAdditionalOrigins()),
	}

	return csrfPolicy, csrfPolicy.Validate()

}

func translateFilterEnabled(glooFilterEnabled *v3.RuntimeFractionalPercent) *envoy_config_core.RuntimeFractionalPercent {
	if glooFilterEnabled == nil {
		return translateRuntimeFractionalPercent(&gloo_config_core.RuntimeFractionalPercent{
			// If we supply a nil DefaultValue here, envoy will replace that with 100%
			DefaultValue: &glootype.FractionalPercent{},
		})
	}
	return translateRuntimeFractionalPercent(glooFilterEnabled)
}

func translateShadowEnabled(glooShadowEnabled *v3.RuntimeFractionalPercent) *envoy_config_core.RuntimeFractionalPercent {
	if glooShadowEnabled == nil {
		return nil
	}
	return translateRuntimeFractionalPercent(glooShadowEnabled)
}

func translateRuntimeFractionalPercent(rfp *v3.RuntimeFractionalPercent) *envoy_config_core.RuntimeFractionalPercent {
	return &envoy_config_core.RuntimeFractionalPercent{
		DefaultValue: &envoytype.FractionalPercent{
			Numerator:   rfp.GetDefaultValue().GetNumerator(),
			Denominator: envoytype.FractionalPercent_DenominatorType(rfp.GetDefaultValue().GetDenominator()),
		},
		RuntimeKey: rfp.GetRuntimeKey(),
	}
}

func translateAdditionalOrigins(glooAdditionalOrigins []*gloo_type_matcher.StringMatcher) []*envoy_type_matcher.StringMatcher {
	var envoyAdditionalOrigins []*envoy_type_matcher.StringMatcher

	for _, ao := range glooAdditionalOrigins {
		switch typed := ao.GetMatchPattern().(type) {
		case *gloo_type_matcher.StringMatcher_Exact:
			envoyAdditionalOrigins = append(envoyAdditionalOrigins, &envoy_type_matcher.StringMatcher{
				MatchPattern: &envoy_type_matcher.StringMatcher_Exact{
					Exact: typed.Exact,
				},
				IgnoreCase: ao.GetIgnoreCase(),
			})
		case *gloo_type_matcher.StringMatcher_Prefix:
			envoyAdditionalOrigins = append(envoyAdditionalOrigins, &envoy_type_matcher.StringMatcher{
				MatchPattern: &envoy_type_matcher.StringMatcher_Prefix{
					Prefix: typed.Prefix,
				},
				IgnoreCase: ao.GetIgnoreCase(),
			})
		case *gloo_type_matcher.StringMatcher_SafeRegex:
			envoyAdditionalOrigins = append(envoyAdditionalOrigins, &envoy_type_matcher.StringMatcher{
				MatchPattern: &envoy_type_matcher.StringMatcher_SafeRegex{
					SafeRegex: &envoy_type_matcher.RegexMatcher{
						EngineType: &envoy_type_matcher.RegexMatcher_GoogleRe2{
							GoogleRe2: &envoy_type_matcher.RegexMatcher_GoogleRE2{},
						},
						Regex: typed.SafeRegex.GetRegex(),
					},
				},
			})
		case *gloo_type_matcher.StringMatcher_Suffix:
			envoyAdditionalOrigins = append(envoyAdditionalOrigins, &envoy_type_matcher.StringMatcher{
				MatchPattern: &envoy_type_matcher.StringMatcher_Suffix{
					Suffix: typed.Suffix,
				},
				IgnoreCase: ao.GetIgnoreCase(),
			})
		}
	}

	return envoyAdditionalOrigins
}
