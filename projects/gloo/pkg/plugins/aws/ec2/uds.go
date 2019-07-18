package ec2

import (
	"context"

	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"golang.org/x/sync/errgroup"
)

// EC2 upstreams are created by the user, not discovered
// However, if a user changes an upstream, we need to detect that change here and send the updated list in order to
// trigger the associated endpoint updates
func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	allUpstreams := make(chan v1.UpstreamList)
	allErrChan := make(chan error)

	watchNamespaceCount := len(watchNamespaces)

	eg, egCtx := errgroup.WithContext(opts.Ctx)
	for _, namespace := range watchNamespaces {
		// shadow to localize the value for the goroutine
		namespace := namespace
		eg.Go(func() error {
			upstreamsList, errChan, err := p.upstreamClient.Watch(namespace, clients.WatchOpts{Ctx: egCtx})
			if err != nil {
				return errors.Wrapf(err, "unable to start watch in namespace %v", namespace)
			}
			for {
				select {
				// any time an EC2 upstream changes we need to send all EC2 upstreams on the watch channel
				case list := <-upstreamsList:
					upstreams, initialCensusComplete := p.updateDiscoveries(watchNamespaceCount, list, namespace)
					if initialCensusComplete {
						allUpstreams <- upstreams
					}
				case err := <-errChan:
					if err != nil {
						return err
					}
				}
			}
		})
	}
	return allUpstreams, allErrChan, nil
}

func (p *plugin) updateDiscoveries(watchedNamespaceCount int, updatedList v1.UpstreamList, namespace string) (v1.UpstreamList, bool) {
	pluginUpstreams := filterEc2Upstreams(updatedList)
	p.discoveryMutex.Lock()
	defer p.discoveryMutex.Unlock()
	// at least one of the upstreams should have changed, so overwrite the entire namespace's value
	// TODO(mitchdraft) use managed selectors on the watch such that watch only returns EC2 upstreams - so that we don't
	// trigger a resync when non-EC2 upstreams change. "managed" refers to the fact that we need to make sure that the
	// upstream-identifier labels are not wrongly set by the user.
	// TODO(mitchdraft) write the upstream's type to its labels during call to discover.setLabels
	// + document the fact that that label (perhaps "gloo-upstream-type") is managed by gloo
	p.watchedUpstreams[namespace] = pluginUpstreams
	// we only want to emit an upstream list when we have gotten at least one report of the upstreams in each namespace
	// otherwise a startup loop would try to delete the endpoints belonging to upstreams in namespaces that have not yet
	// been read
	namespaceCensusComplete := len(p.watchedUpstreams) == watchedNamespaceCount
	return p.serializeUpstreams(), namespaceCensusComplete
}

// this function should only be called when the lock is held
func (p *plugin) serializeUpstreams() v1.UpstreamList {
	var allUpstreams v1.UpstreamList
	for _, upstreams := range p.watchedUpstreams {
		allUpstreams = append(allUpstreams, upstreams...)
	}
	return allUpstreams
}

func (p *plugin) listAllEc2Upstreams(ctx context.Context, watchNamespaces []string) (v1.UpstreamList, error) {
	var aggregate v1.UpstreamList
	for _, namespace := range watchNamespaces {
		nsList, err := p.upstreamClient.List(namespace, clients.ListOpts{Ctx: ctx})
		if err != nil {
			return nil, errors.Wrapf(err, "unable to list upstreams for namespace %v", namespace)
		}
		ec2List := filterEc2Upstreams(nsList)
		aggregate = append(aggregate, ec2List...)
	}
	return aggregate, nil
}

func filterEc2Upstreams(input v1.UpstreamList) v1.UpstreamList {
	var output v1.UpstreamList
	for _, in := range input {
		if _, ok := in.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2); ok {
			output = append(output, in)
		}
	}
	return output
}
