package kubernetes

import (
	"context"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	discoveryAnnotationKey  = "gloo.solo.io/discover"
	discoveryAnnotationTrue = "true"
)

func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {

	if len(watchNamespaces) == 0 {
		watchNamespaces = []string{metav1.NamespaceAll}
	}

	ctx := contextutils.WithLogger(opts.Ctx, "kube-uds")
	logger := contextutils.LoggerFrom(ctx)

	logger.Infow("started", "watchns", watchNamespaces, "writens", writeNamespace)

	watch := p.kubeCoreCache.Subscribe()

	opts = opts.WithDefaults()
	upstreamsChan := make(chan v1.UpstreamList)
	errs := make(chan error)
	discoverUpstreams := func() {
		var serviceList []*kubev1.Service
		for _, ns := range watchNamespaces {

			lister := p.kubeCoreCache.NamespacedServiceLister(ns)
			if lister == nil {
				errs <- errors.Errorf("Kubernetes upstream discovery: Tried to discover upstreams in invalid namespace \"%s\".", ns)
				return
			}

			services, err := lister.List(labels.SelectorFromSet(opts.Selector))
			if err != nil {
				errs <- err
				return
			}
			serviceList = append(serviceList, services...)
		}
		upstreams := p.ConvertServices(ctx, watchNamespaces, serviceList, discOpts, writeNamespace)
		logger.Debugw("discovered services", "num", len(upstreams))
		upstreamsChan <- upstreams
	}

	go func() {
		defer logger.Info("ended")
		defer p.kubeCoreCache.Unsubscribe(watch)
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

func (p *plugin) ConvertServices(ctx context.Context, watchNamespaces []string, services []*kubev1.Service, opts discovery.Opts, writeNamespace string) v1.UpstreamList {
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

		upstreamsToCreate := p.UpstreamConverter.UpstreamsForService(ctx, svc)
		for _, u := range upstreamsToCreate {
			u.GetMetadata().Namespace = writeNamespace
		}

		upstreams = append(upstreams, upstreamsToCreate...)
	}
	return upstreams
}
