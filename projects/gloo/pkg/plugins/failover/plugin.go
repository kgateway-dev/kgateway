package failover

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
	_ plugins.Plugin = new(failoverPluginImpl)
	_ plugins.UpstreamPlugin = new(failoverPluginImpl)
	_ plugins.EndpointPlugin = new(failoverPluginImpl)
)

const (
	errEnterpriseOnly = "Could not load failover plugin - this is an Enterprise feature"
	pluginName = "failover"
)

type failoverPluginImpl struct {
	sslConfigTranslator utils.SslConfigTranslator
	endpoints           map[string][]*envoy_config_endpoint_v3.LocalityLbEndpoints
	dnsResolver         consul.DnsResolver
}

func (p *failoverPluginImpl) ProcessEndpoints(params plugins.Params, in *v1.Upstream, out *envoy_config_endpoint_v3.ClusterLoadAssignment) error {
	failoverCfg := in.GetFailover()
	if failoverCfg != nil {
		return eris.New(errEnterpriseOnly)
	}
	return nil
}

func (p *failoverPluginImpl) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	failoverCfg := in.GetFailover()
	if failoverCfg != nil {
		return eris.New(errEnterpriseOnly)
	}
	return nil
}

func (p *failoverPluginImpl) PluginName() string {
	return pluginName
}

func (p *failoverPluginImpl) IsUpgrade() bool {
	return false
}

func (p *failoverPluginImpl) Init(params plugins.InitParams) error {
	return nil
}
