package runner

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/defaults"
	"github.com/solo-io/gloo/pkg/utils"
	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"
	"github.com/solo-io/gloo/projects/discovery/pkg/uds/syncer"
	syncerutils "github.com/solo-io/gloo/projects/discovery/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/runner"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/namespace"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ bootstrap.Runner = new(udsRunner)

type udsRunner struct {
	extensions *RunExtensions
}

func NewUDSRunner() *udsRunner {
	return NewUDSRunnerWithExtensions(&RunExtensions{})
}

func NewUDSRunnerWithExtensions(extensions *RunExtensions) *udsRunner {
	return &udsRunner{
		extensions: extensions,
	}
}

func (u *udsRunner) Run(ctx context.Context, kubeCache kube.SharedCache, inMemoryCache memory.InMemoryResourceCache, settings *v1.Settings) error {
	ctx = contextutils.WithLogger(ctx, "uds")

	udsEnabled := syncerutils.GetUdsEnabled(settings)
	if !udsEnabled {
		contextutils.LoggerFrom(ctx).Infof("Upstream discovery "+
			"(settings.discovery.udsOptions.enabled) disabled. To enable, modify "+
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

	var nsClient kubernetes.KubeNamespaceClient
	if typedClientset.KubeClient != nil && typedClientset.KubeCoreCache.NamespaceLister() != nil {
		nsClient = namespace.NewNamespaceClient(typedClientset.KubeClient, typedClientset.KubeCoreCache)
	} else {
		// initialize an empty namespace client
		// in the future we can extend the concept of namespaces to
		// its own resource type which users can manage via another storage backend
		nsClient, err = kubernetes.NewKubeNamespaceClient(ctx, &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		})
		if err != nil {
			return err
		}
	}

	emit := make(chan struct{})
	emitter := v1.NewDiscoveryEmitterWithEmit(glooClientset.Upstreams, nsClient, glooClientset.Secrets, emit)

	// jumpstart all the watches
	go func() {
		emit <- struct{}{}
	}()

	dnsAddress := settings.GetConsul().GetDnsAddress()
	if len(dnsAddress) == 0 {
		dnsAddress = consulplugin.DefaultDnsAddress
	}

	dnsPollingInterval := consulplugin.DefaultDnsPollingInterval
	if pollingInterval := settings.GetConsul().GetDnsPollingInterval(); pollingInterval != nil {
		dnsPollingInterval = prototime.DurationFromProto(pollingInterval)
	}
	pluginOpts := registry.PluginOpts{
		SecretClient:  glooClientset.Secrets,
		KubeClient:    typedClientset.KubeClient,
		KubeCoreCache: typedClientset.KubeCoreCache,
		Consul: registry.ConsulPluginOpts{
			ConsulWatcher:      typedClientset.ConsulWatcher,
			DnsServer:          dnsAddress,
			DnsPollingInterval: &dnsPollingInterval,
		},
	}
	plugins := registry.Plugins(pluginOpts)

	var discoveryPlugins []discovery.DiscoveryPlugin
	for _, plug := range plugins {
		disc, ok := plug.(discovery.DiscoveryPlugin)
		if ok {
			discoveryPlugins = append(discoveryPlugins, disc)
		}
	}

	writeNamespace := settings.GetDiscoveryNamespace()
	if writeNamespace == "" {
		writeNamespace = defaults.GlooSystem
	}
	watchNamespaces := utils.ProcessWatchNamespaces(settings.GetWatchNamespaces(), writeNamespace)

	errs := make(chan error)

	statusReporterNamespace := gloostatusutils.GetStatusReporterNamespaceOrDefault(writeNamespace)
	statusClient := gloostatusutils.GetStatusClientForNamespace(statusReporterNamespace)

	uds := discovery.NewUpstreamDiscovery(watchNamespaces, writeNamespace, glooClientset.Upstreams, statusClient, discoveryPlugins)
	// TODO(ilackarms) expose discovery options
	udsErrs, err := uds.StartUds(watchOpts, discovery.Opts{})
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(ctx, errs, udsErrs, "event_loop.uds")

	sync := syncer.NewDiscoverySyncer(uds, watchOpts.RefreshRate)
	eventLoop := v1.NewDiscoveryEventLoop(emitter, sync)

	eventLoopErrs, err := eventLoop.Run(watchNamespaces, watchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(ctx, errs, eventLoopErrs, "event_loop.uds")

	logger := contextutils.LoggerFrom(ctx)

	go func() {
		for {
			select {
			case err, ok := <-errs:
				if !ok {
					return
				}
				logger.Errorf("error: %v", err)
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}
