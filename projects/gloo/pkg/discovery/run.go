package discovery

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/utils/errutils"
	"go.uber.org/zap"
)

// run once, watch upstreams
func RunEds(upstreamClient v1.UpstreamClient, disc *EndpointDiscovery, watchNamespaces []string, opts clients.WatchOpts) (chan error, error) {
	errs := make(chan error)

	publsherr := func(err error) {
		select {
		case errs <- err:
		default:
			contextutils.LoggerFrom(opts.Ctx).Desugar().Warn("received error and cannot aggregate it.", zap.Error(err))
		}
	}

	for _, ns := range watchNamespaces {
		if ns == "" && len(watchNamespaces) > 1 {
			return nil, errors.Errorf("the \"\" namespace is used to watch all namespaces. Snapshots can either be tracked for " +
				"specific namespaces or \"\" AllNamespaces, but not both.")
		}
	}

	upstreams, upstreamErrs, err := upstreamClient.Watch(watchNamespace, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "beginning upstream watch")
	}
	ctx := opts.Ctx
	go func() {
		var cancel context.CancelFunc = func() {}
		defer func() { cancel() }()
		for {
			select {
			case err, ok := <-upstreamErrs:
				if !ok {
					return
				}
				publsherr(err)
			case upstreamList, ok := <-upstreams:
				if !ok {
					return
				}
				cancel()
				opts.Ctx, cancel = context.WithCancel(ctx)

				if len(upstreamList) == 0 {
					continue
				}

				edsErrs, err := disc.StartEds(upstreamList, opts)
				if err != nil {
					publsherr(err)
					continue
				}
				go errutils.AggregateErrs(opts.Ctx, errs, edsErrs, "eds.discovery.gloo")

			}
		}
	}()
	return errs, nil
}
