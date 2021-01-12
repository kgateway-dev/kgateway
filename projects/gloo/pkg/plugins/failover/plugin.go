package failover

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// Compile-time assertion
var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.UpstreamPlugin = new(plugin)
	_ plugins.EndpointPlugin = new(plugin)
	_ plugins.Upgradable     = new(plugin)
)

const (
	ErrEnterpriseOnly = "Could not load failover plugin - this is an Enterprise feature"
	ExtensionName     = "failover"
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

func (p *plugin) ProcessEndpoints(params plugins.Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error {
	if in.GetFailover() != nil {
		return eris.New(ErrEnterpriseOnly)
	}
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	if in.GetFailover() != nil {
		return eris.New(ErrEnterpriseOnly)
	}
	return nil
}
