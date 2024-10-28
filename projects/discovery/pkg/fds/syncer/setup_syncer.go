package syncer

import (
	"github.com/solo-io/gloo/pkg/utils/namespaces"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	discoveryRegistry "github.com/solo-io/gloo/projects/discovery/pkg/fds/discoveries/registry"
	syncerutils "github.com/solo-io/gloo/projects/discovery/pkg/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/namespace"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
)

type Extensions struct {
	DiscoveryFactoryFuncs []func(opts bootstrap.Opts) fds.FunctionDiscoveryFactory
}

func NewSetupFunc() setuputils.SetupFunc {
	return setup.NewSetupFuncWithRunAndExtensions(RunFDS, nil, nil)
}

// NewSetupFuncWithExtensions used as extension point for external repo
func NewSetupFuncWithExtensions(extensions Extensions) setuputils.SetupFunc {
	runWithExtensions := func(opts bootstrap.Opts) error {
		return RunFDSWithExtensions(opts, extensions)
	}
	return setup.NewSetupFuncWithRunAndExtensions(runWithExtensions, nil, nil)
}

func RunFDS(opts bootstrap.Opts) error {
	return RunFDSWithExtensions(opts, Extensions{})
}

func RunFDSWithExtensions(opts bootstrap.Opts, extensions Extensions) error {
	fdsMode := syncerutils.GetFdsMode(opts.Settings)
	if fdsMode == v1.Settings_DiscoveryOptions_DISABLED {
		contextutils.LoggerFrom(opts.WatchOpts.Ctx).Infof("Function discovery "+
			"(settings.discovery.fdsMode) disabled. To enable, modify "+
			"gloo.solo.io/Settings - %v", opts.Settings.GetMetadata().Ref())
		if err := syncerutils.ErrorIfDiscoveryServiceUnused(&opts); err != nil {
			return err
		}
		return nil
	}

	watchOpts := opts.WatchOpts.WithDefaults()
	watchOpts.Ctx = contextutils.WithLogger(watchOpts.Ctx, "fds")

	upstreamClient, err := v1.NewUpstreamClient(watchOpts.Ctx, opts.Upstreams)
	if err != nil {
		return err
	}
	if err := upstreamClient.Register(); err != nil {
		return err
	}
	secretClient, err := v1.NewSecretClient(watchOpts.Ctx, opts.Secrets)
	if err != nil {
		return err
	}
	if err := secretClient.Register(); err != nil {
		return err
	}
	graphqlClient, err := v1beta1.NewGraphQLApiClient(watchOpts.Ctx, opts.GraphQLApis)
	if err != nil {
		return err
	}
	if err := graphqlClient.Register(); err != nil {
		return err
	}

	var nsClient skkube.KubeNamespaceClient
	if opts.KubeClient != nil && opts.KubeCoreCache.NamespaceLister() != nil {
		nsClient = namespace.NewNamespaceClient(opts.KubeClient, opts.KubeCoreCache)
	} else {
		nsClient = &namespaces.NoOpKubeNamespaceWatcher{}
	}

	cache := v1.NewDiscoveryEmitter(upstreamClient, nsClient, secretClient)

	var resolvers fds.Resolvers
	for _, plug := range registry.Plugins(registry.FromBootstrap(opts)) {
		resolver, ok := plug.(fds.Resolver)
		if ok {
			resolvers = append(resolvers, resolver)
		}
	}

	// TODO: unhardcode
	functionalPlugins := GetFunctionDiscoveriesWithExtensions(opts, extensions)

	// TODO(yuval-k): max Concurrency here
	updater := fds.NewUpdater(watchOpts.Ctx, resolvers, graphqlClient, upstreamClient, 0, functionalPlugins)
	disc := fds.NewFunctionDiscovery(updater)

	sync := NewDiscoverySyncer(disc, fdsMode)
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

func GetFunctionDiscoveriesWithExtensions(opts bootstrap.Opts, extensions Extensions) []fds.FunctionDiscoveryFactory {
	return GetFunctionDiscoveriesWithExtensionsAndRegistry(opts, discoveryRegistry.Plugins, extensions)
}

func GetFunctionDiscoveriesWithExtensionsAndRegistry(opts bootstrap.Opts, registryDiscFacts func(opts bootstrap.Opts) []fds.FunctionDiscoveryFactory, extensions Extensions) []fds.FunctionDiscoveryFactory {
	pluginfuncs := extensions.DiscoveryFactoryFuncs
	discFactories := registryDiscFacts(opts)
	for _, discoveryFactoryExtension := range pluginfuncs {
		pe := discoveryFactoryExtension(opts)
		if pe != nil {
			discFactories = append(discFactories, pe)
		}
	}
	return discFactories
}
