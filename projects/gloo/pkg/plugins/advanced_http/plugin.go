package advanced_http

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
	ErrEnterpriseOnly = "Could not load advanced_http plugin to configure custom paths/methods per endpoint, or complex health check response parsing - this is an Enterprise feature"
	ExtensionName     = "advanced_http"
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
		if host.GetHealthCheckConfig().GetMethod() != "" {
			return eris.New(ErrEnterpriseOnly)
		}
	}

	for _, hc := range in.GetHealthChecks() {
		if hc.GetHttpHealthCheck().GetResponseAssertions() != nil {
			return eris.New(ErrEnterpriseOnly)
		}
	}

	return nil
}
