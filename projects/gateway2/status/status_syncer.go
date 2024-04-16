package status

import (
	"context"
	"sync"

	"github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	gwplugins "github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/translatorutils"
	"github.com/solo-io/go-utils/contextutils"
)

// HandleProxyReports should conform to the OnProxiesTranslatedFn and QueueStatusForProxiesFn signatures
var _ syncer.OnProxiesTranslatedFn = (&statusSyncerFactory{}).HandleProxyReports

var _ proxy_syncer.QueueStatusForProxiesFn = (&statusSyncerFactory{}).QueueStatusForProxies

type GatewayStatusSyncer interface {
	QueueStatusForProxies(
		proxiesToQueue v1.ProxyList,
		pluginRegistry *registry.PluginRegistry,
	)
	HandleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports)
}

// a threadsafe factory for initializing a status syncer
// allows for the status syncer to be shared across multiple start funcs
type statusSyncerFactory struct {
	registryPerProxy   map[string]*registry.PluginRegistry
	proxiesPerRegistry map[*registry.PluginRegistry]map[string]bool
	lock               *sync.RWMutex
}

func NewStatusSyncerFactory() GatewayStatusSyncer {
	return &statusSyncerFactory{
		registryPerProxy:   make(map[string]*registry.PluginRegistry),
		proxiesPerRegistry: make(map[*registry.PluginRegistry]map[string]bool),
		lock:               &sync.RWMutex{},
	}
}

func (f *statusSyncerFactory) QueueStatusForProxies(
	proxiesToQueue v1.ProxyList,
	pluginRegistry *registry.PluginRegistry,
) {
	f.lock.Lock()
	defer f.lock.Unlock()
	proxies, ok := f.proxiesPerRegistry[pluginRegistry]
	if !ok {
		proxies = make(map[string]bool)
	}
	for _, proxy := range proxiesToQueue {
		proxyId, err := translatorutils.GetProxyId(proxy)
		if err != nil {
			contextutils.LoggerFrom(context.Background()).Warnf("Skipping queueing proxy status sync: %v", err)
			continue
		}
		proxies[proxyId] = true
	}
	f.proxiesPerRegistry[pluginRegistry] = proxies
}

func (f *statusSyncerFactory) HandleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports) {
	// ignore until the syncer has been initialized
	f.lock.RLock()
	defer f.lock.RUnlock()
	for reg, proxiesToSync := range f.proxiesPerRegistry {
		reg := reg
		var filteredProxiesWithReports []translatorutils.ProxyWithReports
		for _, proxyWithReports := range proxiesWithReports {
			proxyId, err := translatorutils.GetProxyId(proxyWithReports.Proxy)
			if err != nil {
				contextutils.LoggerFrom(context.Background()).Warnf("Skipping status sync: %v", err)
				continue
			}
			if _, ok := proxiesToSync[proxyId]; ok {
				filteredProxiesWithReports = append(filteredProxiesWithReports, proxyWithReports)
				delete(proxiesToSync, proxyId)
				break
			}
		}
		newStatusSyncer(reg).applyStatusPlugins(ctx, filteredProxiesWithReports)
		if len(proxiesToSync) == 0 {
			delete(f.proxiesPerRegistry, reg)
		}
	}
}

type statusSyncer struct {
	pluginRegistry *registry.PluginRegistry
}

func newStatusSyncer(
	pluginRegistry *registry.PluginRegistry,
) *statusSyncer {
	return &statusSyncer{
		pluginRegistry: pluginRegistry,
	}
}

func (s *statusSyncer) applyStatusPlugins(
	ctx context.Context,
	proxiesWithReports []translatorutils.ProxyWithReports,
) {
	ctx = contextutils.WithLogger(ctx, "k8sGatewayStatusPlugins")
	logger := contextutils.LoggerFrom(ctx)

	// filter only the proxies that were produced by k8s gws
	proxiesWithReports = filterProxiesByControllerName(proxiesWithReports)

	statusCtx := &gwplugins.StatusContext{
		ProxiesWithReports: proxiesWithReports,
	}
	for _, plugin := range s.pluginRegistry.GetStatusPlugins() {
		err := plugin.ApplyStatusPlugin(ctx, statusCtx)
		if err != nil {
			logger.Errorf("Error applying status plugin: %v", err)
			continue
		}
	}
}

func filterProxiesByControllerName(
	reports []translatorutils.ProxyWithReports,
) []translatorutils.ProxyWithReports {
	var filtered []translatorutils.ProxyWithReports
	for _, proxyWithReports := range reports {
		if proxyWithReports.Proxy.GetMetadata().GetLabels()[utils.ProxyTypeKey] == utils.GatewayApiProxyValue {
			filtered = append(filtered, proxyWithReports)
		}
	}
	return filtered
}
