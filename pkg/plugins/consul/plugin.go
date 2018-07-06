package consul

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/plugins"
)

func init() {
	plugins.Register(&Plugin{})
}

func (p *Plugin) SetupEndpointDiscovery(opts bootstrap.Options) (endpointdiscovery.Interface, error) {
	cfg := opts.ConsulOptions.ToConsulConfig()
	disc, err := NewEndpointController(cfg, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start consul endpoint discovery")
	}
	return disc, err
}

type Plugin struct{}

const (
	// define Upstream type name
	UpstreamTypeConsul = "consul"
)

func (p *Plugin) GetDependencies(_ *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *Plugin) ProcessUpstream(_ *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if in.Type != UpstreamTypeConsul {
		return nil
	}
	// decode does validation for us
	if _, err := DecodeUpstreamSpec(in.Spec); err != nil {
		return errors.Wrap(err, "invalid consul upstream spec")
	}

	// consul upstreams use EDS
	out.Type = envoyapi.Cluster_EDS
	out.EdsClusterConfig = &envoyapi.Cluster_EdsClusterConfig{
		EdsConfig: &envoycore.ConfigSource{
			ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
				Ads: &envoycore.AggregatedConfigSource{},
			},
		},
	}
	return nil
}
