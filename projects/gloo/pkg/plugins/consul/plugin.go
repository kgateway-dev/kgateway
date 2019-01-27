package consul

import (
	"fmt"
	"net/url"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/hashicorp/consul/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

type plugin struct {
	client *api.Client
}

func (p *plugin) Resolve(u *v1.Upstream) (*url.URL, error) {
	consulSpec, ok := u.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Consul)
	if !ok {
		return nil, nil
	}

	return url.Parse(fmt.Sprintf("tcp://%s.service.consul", consulSpec.Consul.ServiceName))
}

func NewPlugin() plugins.Plugin {
	return &plugin{}
}

func (p *plugin) Init(params plugins.InitParams) error {
	if err := p.tryGetClient(); err != nil {
		return err
	}
	return nil
}

func (p *plugin) tryGetClient() error {
	if p.client != nil {
		return nil
	}
	var err error
	p.client, err = api.NewClient(api.DefaultConfig())
	return err
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	_, ok := in.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Consul)
	if !ok {
		return nil
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
