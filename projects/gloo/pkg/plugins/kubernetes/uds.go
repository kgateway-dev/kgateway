package kubernetes

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	kubev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	discoveryAnnotationKey  = "gloo.solo.io/discover"
	discoveryAnnotationTrue = "true"
)

func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	if p.kubeShareFactory == nil {
		informerF
		p.kubeShareFactory = getInformerFactory(p.kube)
	}
	ctx := contextutils.WithLogger(opts.Ctx, "kube-uds")
	logger := contextutils.LoggerFrom(ctx)

	logger.Infow("started", "watchns", watchNamespaces, "writens", writeNamespace)

	watch := p.kubeShareFactory.Subscribe()

	opts = opts.WithDefaults()
	upstreamsChan := make(chan v1.UpstreamList)
	errs := make(chan error)
	discoverUpstreams := func() {
		services, err := p.kubeShareFactory.ServicesLister().List(labels.SelectorFromSet(opts.Selector))
		if err != nil {
			errs <- err
			return
		}
		pods, err := p.kubeShareFactory.PodsLister().List(labels.SelectorFromSet(opts.Selector))
		if err != nil {
			errs <- err
			return
		}
		upstreams := p.ConvertServices(ctx, watchNamespaces, services, pods, discOpts, writeNamespace)
		logger.Debugw("discovered services", "num", len(upstreams))
		upstreamsChan <- upstreams
	}

	go func() {
		defer logger.Info("ended")
		defer p.kubeShareFactory.Unsubscribe(watch)
		defer close(upstreamsChan)
		defer close(errs)
		// watch should open up with an initial read
		discoverUpstreams()
		for {
			select {
			case _, ok := <-watch:
				if !ok {
					return
				}
				discoverUpstreams()
			case <-ctx.Done():
				return
			}
		}
	}()
	return upstreamsChan, errs, nil
}

func (p *plugin) ConvertServices(ctx context.Context, watchNamespaces []string, services []*kubev1.Service, pods []*kubev1.Pod, opts discovery.Opts, writeNamespace string) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for _, svc := range services {
		if skip(svc, opts) {
			continue
		}

		if !utils.AllNamespaces(watchNamespaces) {
			if !containsString(svc.Namespace, watchNamespaces) {
				continue
			}
		}

		upstreamsToCreate := p.UpstreamConverter.UpstreamsForService(ctx, svc, pods)
		for _, u := range upstreamsToCreate {
			u.Metadata.Namespace = writeNamespace
		}

		upstreams = append(upstreams, upstreamsToCreate...)
	}
	return upstreams
}
