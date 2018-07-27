package fake

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/plugins"
)

func init() {
	plugins.Register(&Plugin{})
}

func (p *Plugin) SetupEndpointDiscovery(opts bootstrap.Options) (endpointdiscovery.Interface, error) {
	return &FakeEndpointDiscovery, nil
}

type Plugin struct{}

const (
	// define Upstream type name
	UpstreamTypeFake = "fake"
)

func (p *Plugin) Init(options bootstrap.Options) error{
	return nil
}

func (p *Plugin) GetDependencies(_ *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *Plugin) ProcessUpstream(_ *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if in.Type != UpstreamTypeFake {
		return nil
	}

	// just configure the cluster to use EDS:ADS and call it a day
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

type FakeED struct {
	EndpointGroups chan endpointdiscovery.EndpointGroups
	Errors         chan error
}

var FakeEndpointDiscovery = FakeED{
	EndpointGroups: make(chan endpointdiscovery.EndpointGroups, 100),
	Errors:         make(chan error, 100),
}

func (fed *FakeED) Run(stop <-chan struct{}) {

}
func (fed *FakeED) TrackUpstreams(upstreams []*v1.Upstream) {
	grp := endpointdiscovery.EndpointGroups{}
	for _, us := range upstreams {
		if us.Type != UpstreamTypeFake {
			continue
		}

		spec, err := DecodeUpstreamSpec(us.Spec)
		if err != nil {
			fed.Errors <- err
			continue
		}
		for _, host := range spec.Hosts {
			grp[us.Name] = append(grp[us.Name], endpointdiscovery.Endpoint{
				Address: host.Addr,
				Port:    int32(host.Port),
			})
		}
	}

	fed.EndpointGroups <- grp

}
func (fed *FakeED) Endpoints() <-chan endpointdiscovery.EndpointGroups {
	return fed.EndpointGroups
}

func (fed *FakeED) Error() <-chan error {
	return fed.Errors
}
