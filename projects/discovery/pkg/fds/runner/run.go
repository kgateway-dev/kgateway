package runner

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/syncer"
	syncerutils "github.com/solo-io/gloo/projects/discovery/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/runner"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/namespace"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"

	discoveryRegistry "github.com/solo-io/gloo/projects/discovery/pkg/fds/discoveries/registry"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
)

var _ bootstrap.Runner = new(fdsRunner)

type fdsRunner struct {
	extensions *RunExtensions
}

func NewFDSRunner() *fdsRunner {
	return NewFDSRunnerWithExtensions(&RunExtensions{})
}

func NewFDSRunnerWithExtensions(extensions *RunExtensions) *fdsRunner {
	return &fdsRunner{
		extensions: extensions,
	}
}

func (f *fdsRunner) Run(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *v1.Settings) error {
	ctx = contextutils.WithLogger(ctx, "fds")

	fdsMode := syncerutils.GetFdsMode(settings)
	if fdsMode == v1.Settings_DiscoveryOptions_DISABLED {
		contextutils.LoggerFrom(ctx).Infof("Function discovery "+
			"(settings.discovery.fdsMode) disabled. To enable, modify "+
			"gloo.solo.io/Settings - %v", settings.GetMetadata().Ref())
		return syncerutils.ErrorIfDiscoveryServiceUnused(settings)
	}

	refreshRate := time.Minute
	if settings.GetRefreshRate() != nil {
		refreshRate = prototime.DurationFromProto(settings.GetRefreshRate())
	}

	watchOpts := clients.WatchOpts{
		Ctx:         ctx,
		RefreshRate: refreshRate,
		Selector:    syncerutils.GetWatchLabels(settings),
	}

	glooClientset, typedClientset, err := runner.GenerateGlooClientsets(ctx, settings, kubeCache, inMemoryCache)
	if err != nil {
		return err
	}

	var nsClient skkube.KubeNamespaceClient
	if typedClientset.KubeClient != nil && typedClientset.KubeCoreCache.NamespaceLister() != nil {
		nsClient = namespace.NewNamespaceClient(typedClientset.KubeClient, typedClientset.KubeCoreCache)
	} else {
		nsClient = &FakeKubeNamespaceWatcher{}
	}

	cache := v1.NewDiscoveryEmitter(glooClientset.Upstreams, nsClient, glooClientset.Secrets)


	var dnsPollingInterval time.Duration
	if pollingInterval := settings.GetConsul().GetDnsPollingInterval(); pollingInterval != nil {
		dnsPollingInterval = prototime.DurationFromProto(pollingInterval)
	}
	pluginOpts := registry.PluginOpts{
		SecretClient:  glooClientset.Secrets,
		KubeClient:    typedClientset.KubeClient,
		KubeCoreCache: typedClientset.KubeCoreCache,
		Consul: registry.ConsulPluginOpts{
			ConsulWatcher:      typedClientset.ConsulWatcher,
			DnsServer:          settings.GetConsul().GetDnsAddress(),
			DnsPollingInterval: &dnsPollingInterval,
		},
	}
	plugins := registry.Plugins(pluginOpts)
	var resolvers fds.Resolvers
	for _, plug := range plugins {
		resolver, ok := plug.(fds.Resolver)
		if ok {
			resolvers = append(resolvers, resolver)
		}
	}

	// TODO: unhardcode
	functionalPlugins := GetFunctionDiscoveriesWithExtensions(*f.extensions)

	// TODO(yuval-k): max Concurrency here
	updater := fds.NewUpdater(ctx, resolvers, glooClientset.GraphQLApis, glooClientset.Upstreams, 0, functionalPlugins)
	functionDiscovery := fds.NewFunctionDiscovery(updater)

	discoverySyncer := syncer.NewDiscoverySyncer(functionDiscovery, fdsMode)
	eventLoop := v1.NewDiscoveryEventLoop(cache, discoverySyncer)

	errs := make(chan error)

	writeNamespace := settings.GetDiscoveryNamespace()
	if writeNamespace == "" {
		writeNamespace = defaults.GlooSystem
	}
	watchNamespaces := utils.ProcessWatchNamespaces(settings.GetWatchNamespaces(), writeNamespace)

	eventLoopErrs, err := eventLoop.Run(watchNamespaces, watchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(ctx, errs, eventLoopErrs, "event_loop.fds")

	logger := contextutils.LoggerFrom(ctx)

	go func() {
		for {
			select {
			case err := <-errs:
				logger.Errorf("error: %v", err)
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

func GetFunctionDiscoveriesWithExtensions(extensions RunExtensions) []fds.FunctionDiscoveryFactory {
	return GetFunctionDiscoveriesWithExtensionsAndRegistry(discoveryRegistry.Plugins, extensions)
}

func GetFunctionDiscoveriesWithExtensionsAndRegistry(registryDiscFacts func() []fds.FunctionDiscoveryFactory, extensions RunExtensions) []fds.FunctionDiscoveryFactory {
	pluginfuncs := extensions.DiscoveryFactoryFuncs
	discFactories := registryDiscFacts()
	for _, discoveryFactoryExtension := range pluginfuncs {
		pe := discoveryFactoryExtension()
		discFactories = append(discFactories, pe)
	}
	return discFactories
}

// FakeKubeNamespaceWatcher to eliminate the need for this fake client for non kube environments
// TODO: consider using regular solo-kit namespace client instead of KubeNamespace client
type FakeKubeNamespaceWatcher struct{}

func (f *FakeKubeNamespaceWatcher) Watch(opts clients.WatchOpts) (<-chan skkube.KubeNamespaceList, <-chan error, error) {
	return nil, nil, nil
}
func (f *FakeKubeNamespaceWatcher) BaseClient() clients.ResourceClient {
	return nil

}
func (f *FakeKubeNamespaceWatcher) Register() error {
	return nil
}
func (f *FakeKubeNamespaceWatcher) Read(name string, opts clients.ReadOpts) (*skkube.KubeNamespace, error) {
	return nil, nil
}
func (f *FakeKubeNamespaceWatcher) Write(resource *skkube.KubeNamespace, opts clients.WriteOpts) (*skkube.KubeNamespace, error) {
	return nil, nil
}
func (f *FakeKubeNamespaceWatcher) Delete(name string, opts clients.DeleteOpts) error {
	return nil
}
func (f *FakeKubeNamespaceWatcher) List(opts clients.ListOpts) (skkube.KubeNamespaceList, error) {
	return nil, nil
}
