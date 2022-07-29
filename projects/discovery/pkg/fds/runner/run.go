package runner

import (
	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"
	syncerutils "github.com/solo-io/gloo/projects/discovery/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/runner"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/namespace"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

func RunFDS(opts runner.RunOpts) error {
	return RunFDSWithExtensions(opts, StartExtensions{})
}

func RunFDSWithExtensions(opts runner.RunOpts, extensions StartExtensions) error {
	fdsMode := syncerutils.GetFdsMode(opts.Settings)
	if fdsMode == v1.Settings_DiscoveryOptions_DISABLED {
		contextutils.LoggerFrom(opts.WatchOpts.Ctx).Infof("Function discovery "+
			"(settings.discovery.fdsMode) disabled. To enable, modify "+
			"gloo.solo.io/Settings - %v", opts.Settings.GetMetadata().Ref())
		if err := syncerutils.ErrorIfDiscoveryServiceUnused(opts.Settings); err != nil {
			return err
		}
		return nil
	}

	watchOpts := opts.WatchOpts.WithDefaults()
	watchOpts.Ctx = contextutils.WithLogger(watchOpts.Ctx, "fds")

	var nsClient skkube.KubeNamespaceClient
	typedClientset := opts.TypedClientset

	if typedClientset.KubeClient != nil && typedClientset.KubeCoreCache.NamespaceLister() != nil {
		nsClient = namespace.NewNamespaceClient(typedClientset.KubeClient, typedClientset.KubeCoreCache)
	} else {
		nsClient = &FakeKubeNamespaceWatcher{}
	}

	glooClientset := opts.ResourceClientset

	cache := v1.NewDiscoveryEmitter(glooClientset.Upstreams, nsClient, glooClientset.Secrets)

	var resolvers fds.Resolvers
	for _, plug := range registry.Plugins(runner.GetPluginOpts(opts)) {
		resolver, ok := plug.(fds.Resolver)
		if ok {
			resolvers = append(resolvers, resolver)
		}
	}

	// TODO: unhardcode
	functionalPlugins := GetFunctionDiscoveriesWithExtensions(opts, extensions)

	// TODO(yuval-k): max Concurrency here
	updater := fds.NewUpdater(watchOpts.Ctx, resolvers, glooClientset.GraphQLApis, glooClientset.Upstreams, 0, functionalPlugins)
	disc := fds.NewFunctionDiscovery(updater)

	sync := syncer.NewDiscoverySyncer(disc, fdsMode)
	eventLoop := v1.NewDiscoveryEventLoop(cache, sync)

	errs := make(chan error)

	eventLoopErrs, err := eventLoop.Run(opts.WatchNamespaces, watchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(watchOpts.Ctx, errs, eventLoopErrs, "event_loop.fds")

	logger := contextutils.LoggerFrom(watchOpts.Ctx)

	go func() {

		for {
			select {
			case err := <-errs:
				logger.Errorf("error: %v", err)
			case <-watchOpts.Ctx.Done():
				return
			}
		}
	}()
	return nil
}
