package http_path

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

// Compile-time assertion
var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.UpstreamPlugin = new(plugin)
)

const (
	errEnterpriseOnly = "Could not load http_path plugin - this is an Enterprise feature"
	pluginName        = "http_path"
)

type plugin struct {
	sslConfigTranslator utils.SslConfigTranslator
	endpoints           map[string][]*envoy_config_endpoint_v3.LocalityLbEndpoints
	dnsResolver         consul.DnsResolver
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	for _, host := range in.GetStatic().GetHosts() {
		if host.GetHealthCheckConfig().GetPath() != "" {
			return eris.New(errEnterpriseOnly)
		}
	}

	return nil
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
