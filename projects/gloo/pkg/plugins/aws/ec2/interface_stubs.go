package ec2

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

// EC2 upstreams are created by the user, not discovered
func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	// TODO(mitchdraft) - need to implement this as a watch on upstreams so we will update the config as soon as a user changes an upstream, not just when the poll triggers
	return nil, nil, nil
}
