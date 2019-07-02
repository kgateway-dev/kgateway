package ec2

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

// the user is creating the EC2 upstreams so we don't really have any processing to do
func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	_, ok := in.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
	if !ok {
		return nil
	}

	// configure the cluster to use EDS:ADS and call it a day
	xds.SetEdsOnCluster(out)
	return nil
}

// EC2 upstreams are created by the user, not discovered
func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	return nil, nil, nil
}
