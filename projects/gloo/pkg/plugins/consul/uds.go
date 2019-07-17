package consul

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

// TODO(marco): consul service discovery currently happens in memory (see HybridUpstreamClient).
//  Re-implementing this discovery (which writes to storage) reusing the ConsulWatcher should be
//  pretty straightforward. We need to decide what we want to do.
func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	return nil, nil, errors.New("not implemented")
}

func (p *plugin) UpdateUpstream(original, desired *v1.Upstream) (bool, error) {
	panic("should never have been called")
}
