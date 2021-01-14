package http_path

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// Compile-time assertion
var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.UpstreamPlugin = new(plugin)
	_ plugins.Upgradable     = new(plugin)
)

const (
	ErrEnterpriseOnly = "Could not load http_path plugin to configure custom paths/endpoint - this is an Enterprise feature"
	ExtensionName     = "http_path"
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

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	for _, host := range in.GetStatic().GetHosts() {
		if host.GetHealthCheckConfig().GetPath() != "" {
			return eris.New(ErrEnterpriseOnly)
		}
	}

	return nil
}
