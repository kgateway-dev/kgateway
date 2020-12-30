package jwt

import (
	envoy_config_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// Compile-time assertion
var (
	_ plugins.Plugin = &plugin{}
	_ plugins.HttpFilterPlugin = &plugin{}
	_ plugins.VirtualHostPlugin = &plugin{}
)

const (
	errEnterpriseOnly = "Could not load jwt plugin - this is an Enterprise feature"
	pluginName = "jwt"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) PluginName() string {
	return pluginName
}

func (p *plugin) IsUpgrade() bool {
	return false
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) HttpFilters(params plugins.Params, l *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	return nil, nil
}

func (p *plugin) ProcessVirtualHost(
	params plugins.VirtualHostParams,
	in *v1.VirtualHost,
	out *envoy_config_route.VirtualHost,
) error {
	jwt := in.GetOptions().GetJwt()
	if jwt != nil {
		return eris.New(errEnterpriseOnly)
	}

	return nil
}