package csrf

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
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

	csrfConfig := listener.GetOptions().GetBuffer()

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
	csrfPolicyPerRoute := in.Options.GetCsrf()
	if csrfPolicyPerRoute == nil {
		return nil
	}

	additionalOrigins := csrfPolicyPerRoute.GetAdditionalOrigins()
	if additionalOrigins != nil {

	}

	filtersEnabled := csrfPolicyPerRoute.GetFilterEnabled()
	shadowEnabled := csrfPolicyPerRoute.GetShadowEnabled()

	return nil
}

func (p *Plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {

	return nil
}

func (p *Plugin) ProcessWeightedDestination(
	params plugins.RouteParams,
	in *v1.WeightedDestination,
	out *envoy_config_route_v3.WeightedCluster_ClusterWeight,
) error {

	return nil
}
