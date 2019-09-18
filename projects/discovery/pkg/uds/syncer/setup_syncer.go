package syncer

import (
	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/namespace"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

func RunUDS(opts bootstrap.Opts) error {
	watchOpts := opts.WatchOpts.WithDefaults()
	watchOpts.Ctx = contextutils.WithLogger(watchOpts.Ctx, "uds")

	upstreamClient, err := v1.NewUpstreamClient(opts.Upstreams)
	if err != nil {
		return err
	}
	if err := upstreamClient.Register(); err != nil {
		return err
	}

	secretClient, err := v1.NewSecretClient(opts.Secrets)
	if err != nil {
		return err
	}
	if err := secretClient.Register(); err != nil {
		return err
	}

	var nsClient kubernetes.KubeNamespaceClient
	if opts.KubeClient != nil && opts.KubeCoreCache.NamespaceLister() != nil {
		nsClient = namespace.NewNamespaceClient(opts.KubeClient, opts.KubeCoreCache)
	} else {
		// initialize an empty namespace client
		// in the future we can extend the concept of namespaces to
		// its own resource type which users can manage via another storage backend
		nsClient, err = kubernetes.NewKubeNamespaceClient(&factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		})
		if err != nil {
			return err
		}
	}

	emit := make(chan struct{})
	emitter := v1.NewDiscoveryEmitterWithEmit(upstreamClient, nsClient, secretClient, emit)

	// jump start all the watches
	go func() {
		emit <- struct{}{}
	}()

	plugins := registry.Plugins(opts)

	var discoveryPlugins []discovery.DiscoveryPlugin
	for _, plug := range plugins {
		disc, ok := plug.(discovery.DiscoveryPlugin)
		if ok {
			discoveryPlugins = append(discoveryPlugins, disc)
		}
	}
	watchNamespaces := utils.ProcessWatchNamespaces(opts.WatchNamespaces, opts.WriteNamespace)

	errs := make(chan error)

	uds := discovery.NewUpstreamDiscovery(watchNamespaces, opts.WriteNamespace, upstreamClient, discoveryPlugins)
	// TODO(ilackarms) expose discovery options
	udsErrs, err := uds.StartUds(watchOpts, discovery.Opts{})
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(watchOpts.Ctx, errs, udsErrs, "event_loop.uds")

	sync := NewDiscoverySyncer(uds, watchOpts.RefreshRate)
	eventLoop := v1.NewDiscoveryEventLoop(emitter, sync)

	eventLoopErrs, err := eventLoop.Run(opts.WatchNamespaces, watchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(watchOpts.Ctx, errs, eventLoopErrs, "event_loop.uds")

	logger := contextutils.LoggerFrom(watchOpts.Ctx)

	go func() {
		for {
			select {
			case err, ok := <-errs:
				if !ok {
					return
				}
				logger.Errorf("error: %v", err)
			case <-watchOpts.Ctx.Done():
				return
			}
		}
	}()
	return nil
}
