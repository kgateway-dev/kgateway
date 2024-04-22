package status

import (
	"context"
	"strconv"
	"sync"

	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
	"k8s.io/apimachinery/pkg/types"

	"github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	gwplugins "github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/translatorutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

// HandleProxyReports should conform to the OnProxiesTranslatedFn and QueueStatusForProxiesFn signatures
var _ syncer.OnProxiesTranslatedFn = (&statusSyncerFactory{}).HandleProxyReports

// QueueStatusForProxiesFn queues a status sync for a given set of Proxy resources along with the plugins that produced them
var _ proxy_syncer.QueueStatusForProxiesFn = (&statusSyncerFactory{}).QueueStatusForProxies

// GatewayStatusSyncer is responsible for applying status plugins to Gloo Gateway proxies
type GatewayStatusSyncer interface {
	QueueStatusForProxies(
		proxiesToQueue v1.ProxyList,
		pluginRegistry *registry.PluginRegistry,
		totalSyncCount int,
	)
	HandleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports)
}

// a threadsafe factory for initializing a status syncer
// allows for the status syncer to be shared across multiple start funcs
type statusSyncerFactory struct {
	// maps a proxy from a proxy sync action to the plugin registry that produced it
	// proxy -> sync iteration -> plugin registry
	registryPerSync map[int]*registry.PluginRegistry
	resyncsPerProxy map[types.NamespacedName]int

	lock *sync.Mutex
}

func NewStatusSyncerFactory() GatewayStatusSyncer {
	return &statusSyncerFactory{
		registryPerSync: make(map[int]*registry.PluginRegistry),
		resyncsPerProxy: make(map[types.NamespacedName]int),
		lock:            &sync.Mutex{},
	}
}

// QueueStatusForProxies queues the proxies to be synced by the status syncer
func (f *statusSyncerFactory) QueueStatusForProxies(
	proxiesToQueue v1.ProxyList,
	pluginRegistry *registry.PluginRegistry,
	totalSyncCount int,
) {
	f.lock.Lock()
	defer f.lock.Unlock()

	for _, proxy := range proxiesToQueue {
		f.resyncsPerProxy[getProxyNameNamespace(proxy)] = totalSyncCount
	}
	f.registryPerSync[totalSyncCount] = pluginRegistry
}

// HandleProxyReports is a callback that applies status plugins to the proxies that have been queued
func (f *statusSyncerFactory) HandleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports) {
	// ignore until the syncer has been initialized
	f.lock.Lock()
	defer f.lock.Unlock()

	proxiesToReport := make(map[int][]translatorutils.ProxyWithReports)
	for _, proxyWithReport := range filterProxiesByControllerName(proxiesWithReports) {
		var proxySyncCount int
		if proxyWithReport.Proxy.GetMetadata().GetAnnotations() != nil {
			if syncId, ok := proxyWithReport.Proxy.GetMetadata().GetAnnotations()[utils.ProxySyncId]; ok {
				proxySyncCount, _ = strconv.Atoi(syncId)
			}
		}
		proxyKey := getProxyNameNamespace(proxyWithReport.Proxy)

		if f.resyncsPerProxy[proxyKey] > proxySyncCount {
			continue // old one was garbage collectd expect a future resync
		}

		proxiesToReport[proxySyncCount] = append(proxiesToReport[proxySyncCount], proxyWithReport)
		delete(f.resyncsPerProxy, proxyKey)
	}

	for syncCount, proxies := range proxiesToReport {
		if plugins, ok := f.registryPerSync[syncCount]; ok {
			newStatusSyncer(plugins).applyStatusPlugins(ctx, proxies)

			if len(f.resyncsPerProxy) == 0 {
				f.registryPerSync = make(map[int]*registry.PluginRegistry)
			}
		} else {
			// dpanic?
			contextutils.LoggerFrom(ctx).DPanicf("no registry found for proxy sync count %d", syncCount)
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

func getProxyNameNamespace(proxy *v1.Proxy) types.NamespacedName {
	return types.NamespacedName{
		Name:      proxy.GetMetadata().GetName(),
		Namespace: proxy.GetMetadata().GetNamespace(),
	}
}

func getProxySyncCounter(proxy *v1.Proxy) (int, error) {
	proxyAnnotations := proxy.GetMetadata().GetAnnotations()
	if proxyAnnotations == nil {
		return 0, eris.New("proxy annotations are nil")
	}
	if id, ok := proxyAnnotations[utils.ProxySyncId]; !ok {
		return 0, eris.New("proxy sync id not found")
	} else {
		counter, err := strconv.Atoi(id)
		if err != nil {
			return 0, err
		}
		return counter, nil
	}
}
