package waf

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	errEnterpriseOnly = "Could not load jwt plugin - this is an Enterprise feature"
	ExtensionName = "waf"
)

type Plugin struct {
	listenerEnabled map[*v1.HttpListener]bool
}

var (
	_ plugins.Plugin            = new(Plugin)
	_ plugins.VirtualHostPlugin = new(Plugin)
	_ plugins.RoutePlugin       = new(Plugin)
	_ plugins.HttpFilterPlugin  = new(Plugin)

	// waf should happen before any code is run
	filterStage = plugins.DuringStage(plugins.WafStage)
)

func NewPlugin() *Plugin {
	return &Plugin{
		listenerEnabled: make(map[*v1.HttpListener]bool),
	}
}

func (p *Plugin) PluginName() string {
	return ExtensionName
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

// Process virtual host plugin
func (p *Plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoy_config_route_v3.VirtualHost) error {
	wafConfig := in.Options.GetWaf()
	if wafConfig != nil {
		return eris.New(errEnterpriseOnly)
	}

	return nil
}

// Process route plugin
func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	wafConfig := in.GetOptions().GetWaf()
	if wafConfig != nil {
		return eris.New(errEnterpriseOnly)
	}

	return nil
}

// Http Filter to return the waf filter
func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	waf := listener.GetOptions().GetWaf()
	if waf != nil {
		return nil, eris.New(errEnterpriseOnly)
	}
	return nil, nil
}