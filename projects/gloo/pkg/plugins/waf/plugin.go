package waf

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	errEnterpriseOnly = "Could not load waf plugin - this is an Enterprise feature"
	ExtensionName = "waf"
)

type plugin struct {
	listenerEnabled map[*v1.HttpListener]bool
}

var (
	_ plugins.Plugin            = new(plugin)
	_ plugins.VirtualHostPlugin = new(plugin)
	_ plugins.RoutePlugin       = new(plugin)
	_ plugins.HttpFilterPlugin  = new(plugin)

	// waf should happen before any code is run
	filterStage = plugins.DuringStage(plugins.WafStage)
)

func NewPlugin() *plugin {
	return &plugin{
		listenerEnabled: make(map[*v1.HttpListener]bool),
	}
}

func (p *plugin) PluginName() string {
	return ExtensionName
}

func (p *plugin) IsUpgrade() bool {
	return false
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

// Process virtual host plugin
func (p *plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoy_config_route_v3.VirtualHost) error {
	wafConfig := in.Options.GetWaf()
	if wafConfig != nil {
		return eris.New(errEnterpriseOnly)
	}

	return nil
}

// Process route plugin
func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	wafConfig := in.GetOptions().GetWaf()
	if wafConfig != nil {
		return eris.New(errEnterpriseOnly)
	}

	return nil
}

// Http Filter to return the waf filter
func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	waf := listener.GetOptions().GetWaf()
	if waf != nil {
		return nil, eris.New(errEnterpriseOnly)
	}
	return nil, nil
}