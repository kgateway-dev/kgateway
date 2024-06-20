package status

import (
	"context"
	"strconv"
	"sync"

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

// QueueStatusForProxiesFn queues a list of proxies to be synced and the plugin registry that produced them for a given sync iteration
var _ proxy_syncer.QueueStatusForProxiesFn = (&statusSyncerFactory{}).QueueStatusForProxies

// GatewayStatusSyncer is responsible for applying status plugins to Gloo Gateway proxies
type GatewayStatusSyncer interface {
	QueueStatusForProxies(
		ctx context.Context,
		proxiesToQueue v1.ProxyList,
		pluginRegistry *registry.PluginRegistry,
		totalSyncCount int,
	)
	HandleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports)
}

// a threadsafe factory for initializing a status syncer
// allows for the status syncer to be shared across multiple start funcs
type statusSyncerFactory struct {
	// maps a proxy sync action to the plugin registry that produced it
	// sync iteration -> plugin registry
	registryPerSync map[int]*registry.PluginRegistry
	// maps a proxy to the sync iteration that produced it
	// only the latest sync iteration is stored and used to apply status plugins
	resyncsPerProxy map[types.NamespacedName]int
	// proxies left to sync
	resyncsPerIteration map[int][]types.NamespacedName

	lock *sync.Mutex
}

func NewStatusSyncerFactory() GatewayStatusSyncer {
	return &statusSyncerFactory{
		registryPerSync:     make(map[int]*registry.PluginRegistry),
		resyncsPerProxy:     make(map[types.NamespacedName]int),
		resyncsPerIteration: make(map[int][]types.NamespacedName),
		lock:                &sync.Mutex{},
	}
}

// QueueStatusForProxies queues the proxies to be synced and plugin registry for the given sync iteration
func (f *statusSyncerFactory) QueueStatusForProxies(
	ctx context.Context,
	proxiesToQueue v1.ProxyList,
	pluginRegistry *registry.PluginRegistry,
	totalSyncCount int,
) {
	f.lock.Lock()
	defer f.lock.Unlock()

	contextutils.LoggerFrom(ctx).Debugf("queueing %v proxies for sync %d", len(proxiesToQueue), totalSyncCount)

	// queue each proxy for a given sync iteration
	for _, proxy := range proxiesToQueue {
		// overwrite the sync count for the proxy with the most recent sync count
		f.resyncsPerProxy[getProxyNameNamespace(proxy)] = totalSyncCount

		// keep track of proxies to check all proxies are handled in debugger
		f.resyncsPerIteration[totalSyncCount] = append(f.resyncsPerIteration[totalSyncCount], getProxyNameNamespace(proxy))
	}
	// the plugin registry that produced the proxies is the same for all proxies in a given sync
	f.registryPerSync[totalSyncCount] = pluginRegistry
}

// HandleProxyReports is a callback that applies status plugins to the proxies that have been queued
func (f *statusSyncerFactory) HandleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports) {
	// ignore until the syncer has been initialized
	f.lock.Lock()
	defer f.lock.Unlock()

	contextutils.LoggerFrom(ctx).Debugf("handling proxy reports for %v proxies", len(proxiesWithReports))

	proxiesToReport := make(map[int][]translatorutils.ProxyWithReports)
	var proxySyncCount int
	for _, proxyWithReport := range filterProxiesByControllerName(proxiesWithReports) {
		// Get the sync iteration that produced the proxy from the proxy metadata
		if proxyWithReport.Proxy.GetMetadata().GetAnnotations() != nil {
			if syncId, ok := proxyWithReport.Proxy.GetMetadata().GetAnnotations()[utils.ProxySyncId]; ok {
				proxySyncCount, _ = strconv.Atoi(syncId)
			}
		}
		proxyKey := getProxyNameNamespace(proxyWithReport.Proxy)

		// if the proxySyncCount saved in the statusSyncer for a given proxy is higher (i.e. newer) than the syncCount
		// on the proxy metadata, then continue because this report iteration is for an older sync which we no longer care about
		if f.resyncsPerProxy[proxyKey] > proxySyncCount {
			// old proxy was garbage collected, expect a future re-sync
			continue
		}

		if f.resyncsPerIteration[proxySyncCount] == nil {
			// re-sync already happened, nothing to do
			continue
		} else {
			updatedList := make([]types.NamespacedName, 0)
			for _, proxyNameNs := range f.resyncsPerIteration[proxySyncCount] {
				if proxyNameNs != proxyKey {
					updatedList = append(updatedList, proxyNameNs)
				}
			}
			f.resyncsPerIteration[proxySyncCount] = updatedList
		}

		proxiesToReport[proxySyncCount] = append(proxiesToReport[proxySyncCount], proxyWithReport)
		// remove the proxy from the queue
		delete(f.resyncsPerProxy, proxyKey)
	}

	for syncCount, proxies := range proxiesToReport {
		if plugins, ok := f.registryPerSync[syncCount]; ok {
			newStatusSyncer(plugins).applyStatusPlugins(ctx, proxies)
		}

		// If there are no more proxies for the sync iteration, delete the sync count
		if len(f.resyncsPerIteration) == 0 {
			delete(f.registryPerSync, syncCount)
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
