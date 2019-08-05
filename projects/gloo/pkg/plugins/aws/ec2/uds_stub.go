package ec2

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

// EC2 upstreams are created by the user, not discovered
// when upstreams are edited, endpoint discovery will be restarted with the latest version of the updates
// This is just needed to satisfy the DiscoveryPlugin interface
// TODO[eds enhancement] - extract "EDS Plugin" from DiscoveryPlugin interface for plugins such as this that don't do discovery
func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	return nil, nil, nil
}
