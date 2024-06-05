package header_validation

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var (
	_ plugins.Plugin            = new(plugin)
	_ plugins.HttpFilterPlugin  = new(plugin)
	_ plugins.RoutePlugin       = new(plugin)
	_ plugins.VirtualHostPlugin = new(plugin)
)

const (
	ExtensionName = "header_validation"
	FilterName    = "envoy.http.header_validators.envoy_default"
)

// TODO decide where the appropriate location is for this filter. Since we can
// filter requests fairly early in the request filter chain, putting it
// reasonably early seems like the best place to put this for now.
var pluginStage = plugins.AfterStage(plugins.FaultStage)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) HttpFilters(_ plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	headerValidationSettings := listener.GetOptions().GetHeaderValidationSettings()
	if headerValidationSettings == nil {
		return nil, nil
	}

	headerValidationFilter, err := plugins.NewStagedFilter(ExtensionName, headerValidationSettings, pluginStage)
	if err != nil {
		return nil, eris.Wrapf(err, "generating header validation filter config")
	}

	return []plugins.StagedHttpFilter{headerValidationFilter}, nil
}

func (p *plugin) ProcessRoute(
	params plugins.RouteParams,
	in *v1.Route,
	out *envoy_config_route_v3.Route) error {
	return eris.New("not implemented")
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route_v3.VirtualHost,
) error {
	return eris.New("not implemented")
}
