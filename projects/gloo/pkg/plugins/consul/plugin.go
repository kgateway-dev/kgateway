package consul

import (
	"fmt"
	"net/url"

	"github.com/hashicorp/consul/api"
	"github.com/solo-io/go-utils/errors"

	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
)

var _ discovery.DiscoveryPlugin = new(plugin)

type plugin struct {
	client consul.ConsulWatcher
}

func (p *plugin) Resolve(u *v1.Upstream) (*url.URL, error) {
	consulSpec, ok := u.UpstreamType.(*v1.Upstream_Consul)
	if !ok {
		return nil, nil
	}

	spec := consulSpec.Consul

	// default to first datacenter
	var dc string
	if len(spec.DataCenters) > 0 {
		dc = spec.DataCenters[0]
	}

	instances, _, err := p.client.Service(spec.ServiceName, "", &api.QueryOptions{Datacenter: dc, RequireConsistent: true})
	if err != nil {
		return nil, errors.Wrapf(err, "getting service from catalog")
	}

	scheme := "http"
	if u.SslConfig != nil {
		scheme = "https"
	}

	for _, inst := range instances {
		if matchTags(spec.ServiceTags, inst.ServiceTags) {
			return url.Parse(fmt.Sprintf("%v://%v:%v", scheme, inst.ServiceAddress, inst.ServicePort))
		}
	}

	return nil, errors.Errorf("service with name %s and tags %v not found", spec.ServiceName, spec.ServiceTags)
}

func NewPlugin(client consul.ConsulWatcher) *plugin {
	return &plugin{client: client}
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	_, ok := in.UpstreamType.(*v1.Upstream_Consul)
	if !ok {
		return nil
	}

	// consul upstreams use EDS
	xds.SetEdsOnCluster(out)

	return nil
}

func matchTags(t1, t2 []string) bool {
	if len(t1) != len(t2) {
		return false
	}
	for _, tag1 := range t1 {
		var found bool
		for _, tag2 := range t2 {
			if tag1 == tag2 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
