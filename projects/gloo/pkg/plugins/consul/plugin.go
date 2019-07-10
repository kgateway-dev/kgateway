package consul

import (
	"fmt"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"net/url"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
)

var _ discovery.DiscoveryPlugin = new(plugin)

type plugin struct {
	client consul.ConsulClient
}

func (p *plugin) Resolve(u *v1.Upstream) (*url.URL, error) {
	consulSpec, ok := u.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Consul)
	if !ok {
		return nil, nil
	}

	return url.Parse(fmt.Sprintf("tcp://%s.service.consul", consulSpec.Consul.ServiceName))
}

func NewPlugin(client consul.ConsulClient) plugins.Plugin {
	return &plugin{client: client}
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	_, ok := in.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Consul)
	if !ok {
		return nil
	}

	// consul upstreams use EDS
	xds.SetEdsOnCluster(out)

	return nil
}
