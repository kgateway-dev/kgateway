package status

import (
	"context"
	"sync"

	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwplugins "github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/translatorutils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type GatewayStatusSyncer interface {
	InitStatusSyncer(
		controllerName string,
		routeOptionClient gatewayv1.RouteOptionClient,
		k8sGwExtensions extensions.K8sGatewayExtensions,
		statusReporter reporter.StatusReporter,
	)
	HandleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports)
}

// HandleProxyReports should conform to the OnProxiesTranslatedFn and InitStatusSyncerFn signatures
var _ syncer.OnProxiesTranslatedFn = (&statusSyncerFactory{}).HandleProxyReports
var _ controller.InitStatusSyncerFn = (&statusSyncerFactory{}).InitStatusSyncer

// a threadsafe factory for initializing a status syncer
// allows for the status syncer to be shared across multiple start funcs
type statusSyncerFactory struct {
	syncer *statusSyncer
	lock   *sync.Mutex
}

func NewStatusSyncerFactory() GatewayStatusSyncer {
	return &statusSyncerFactory{
		lock: &sync.Mutex{},
	}
}

func (f *statusSyncerFactory) HandleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports) {
	// ignore until the syncer has been initialized
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.syncer == nil {
		return
	}
	f.syncer.handleProxyReports(ctx, proxiesWithReports)
}

func (f *statusSyncerFactory) InitStatusSyncer(
	controllerName string,
	routeOptionClient gatewayv1.RouteOptionClient,
	k8sGwExtensions extensions.K8sGatewayExtensions,
	statusReporter reporter.StatusReporter,
) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.syncer == nil {
		f.syncer = newStatusSyncer(
			controllerName,
			routeOptionClient,
			k8sGwExtensions,
			statusReporter,
		)
	} else {
		// dpanic
		contextutils.LoggerFrom(context.Background()).DPanic("status syncer already initialized")
	}
}

type statusSyncer struct {
	controllerName    string
	statusReporter    reporter.StatusReporter
	k8sGwExtensions   extensions.K8sGatewayExtensions
	routeOptionClient gatewayv1.RouteOptionClient
}

func newStatusSyncer(
	controllerName string,
	routeOptionClient gatewayv1.RouteOptionClient,
	k8sGwExtensions extensions.K8sGatewayExtensions,
	statusReporter reporter.StatusReporter,
) *statusSyncer {
	return &statusSyncer{
		controllerName:    controllerName,
		routeOptionClient: routeOptionClient,
		k8sGwExtensions:   k8sGwExtensions,
		statusReporter:    statusReporter,
	}
}

func (s *statusSyncer) handleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports) {
	pluginRegistry := s.k8sGwExtensions.CreatePluginRegistry(ctx)

	s.applyStatusPlugins(ctx, pluginRegistry, proxiesWithReports)
}

func (s *statusSyncer) applyStatusPlugins(
	ctx context.Context,
	pluginRegistry registry.PluginRegistry,
	proxiesWithReports []translatorutils.ProxyWithReports,
) {
	ctx = contextutils.WithLogger(ctx, "k8sGatewayStatusPlugins")
	logger := contextutils.LoggerFrom(ctx)

	// filter only the proxies that were produced by k8s gws
	proxiesWithReports = filterProxiesByControllerName(proxiesWithReports, s.controllerName)

	statusCtx := &gwplugins.StatusContext{
		ProxiesWithReports: proxiesWithReports,
	}
	for _, plugin := range pluginRegistry.GetStatusPlugins() {
		err := plugin.ApplyStatusPlugin(ctx, statusCtx)
		if err != nil {
			logger.Errorf("Error applying status plugin: %v", err)
			continue
		}
	}
}

func filterProxiesByControllerName(
	reports []translatorutils.ProxyWithReports,
	name string,
) []translatorutils.ProxyWithReports {
	return nil
}
