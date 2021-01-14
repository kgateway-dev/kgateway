package rbac

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	ExtensionName     = "rbac"
	ErrEnterpriseOnly = "Could not load rbac plugin - this is an Enterprise feature"
)

var (
	_ plugins.Plugin            = NewPlugin()
	_ plugins.RoutePlugin       = NewPlugin()
	_ plugins.VirtualHostPlugin = NewPlugin()
	_ plugins.Upgradable        = new(plugin)
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
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

func (p *plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoy_config_route_v3.VirtualHost) error {
	if in.Options.GetRbac() != nil {
		return eris.New(ErrEnterpriseOnly)
	}

	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	if in.Options.GetRbac() != nil {
		return eris.New(ErrEnterpriseOnly)
	}

	return nil
}
