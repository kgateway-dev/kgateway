package rbac

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/rbac"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	ExtensionName = "rbac"
	errEnterpriseOnly = "Could not load jwt plugin - this is an Enterprise feature"
)

var (
	_           plugins.Plugin            = NewPlugin()
	_           plugins.RoutePlugin       = NewPlugin()
	_           plugins.VirtualHostPlugin = NewPlugin()
	filterStage                           = plugins.DuringStage(plugins.AuthZStage)
)

type plugin struct {
	settings *rbac.Settings
}

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
	p.settings = params.Settings.GetRbac()
	return nil
}

func (p *plugin) ProcessVirtualHost(params plugins.VirtualHostParams, in *v1.VirtualHost, out *envoy_config_route_v3.VirtualHost) error {
	rbacConf := in.Options.GetRbac()
	if rbacConf != nil {
		return eris.New(errEnterpriseOnly)
	}

	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	rbacConf := in.Options.GetRbac()
	if rbacConf != nil {
		return eris.New(errEnterpriseOnly)
	}

	return nil
}


