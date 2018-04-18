package cloudfoundry

import (
	"context"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/plugins"

	"github.com/solo-io/gloo/pkg/api/types/v1"
)

func init() {
	plugins.Register(&Plugin{}, createEndpointDiscovery)
}

var _ plugins.UpstreamPlugin = &Plugin{}

type Plugin struct {
}

func createEndpointDiscovery(opts bootstrap.Options) (endpointdiscovery.Interface, error) {
	istioclient, err := GetClientFromOptions(opts.CoPilotOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create copilot client")
	}
	resyncDuration := opts.ConfigStorageOptions.SyncFrequency
	disc := NewEndpointDiscovery(context.Background(), istioclient, resyncDuration)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start copilot endpoint discovery")
	}
	return disc, nil
}

func (p *Plugin) GetDependencies(cfg *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *Plugin) ProcessUpstream(params *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if in.Type != UpstreamTypeCF {
		return nil
	}

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
