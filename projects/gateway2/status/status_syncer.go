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
	)
	HandleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports)
}

// a threadsafe factory for initializing a status syncer
// allows for the status syncer to be shared across multiple start funcs
type statusSyncerFactory struct {
	// maps a proxy from a proxy sync action to the plugin registry that produced it
	// proxy -> sync iteration -> plugin registry
	registryPerProxy map[types.NamespacedName]map[int]*registry.PluginRegistry
	// maps a plugin registry to the proxies that need to be synced
	// plugin registry -> proxy -> current syncer iteration
	proxiesPerRegistry map[*registry.PluginRegistry]map[types.NamespacedName]int
	lock               *sync.RWMutex
}

func NewStatusSyncerFactory() GatewayStatusSyncer {
	return &statusSyncerFactory{
		registryPerProxy:   make(map[types.NamespacedName]map[int]*registry.PluginRegistry),
		proxiesPerRegistry: make(map[*registry.PluginRegistry]map[types.NamespacedName]int),
		lock:               &sync.RWMutex{},
	}
}

// QueueStatusForProxies queues the proxies to be synced by the status syncer
func (f *statusSyncerFactory) QueueStatusForProxies(
	proxiesToQueue v1.ProxyList,
	pluginRegistry *registry.PluginRegistry,
) {
	f.lock.Lock()
	defer f.lock.Unlock()
	proxies, ok := f.proxiesPerRegistry[pluginRegistry]
	if !ok {
		proxies = make(map[types.NamespacedName]int)
	}
	for _, proxy := range proxiesToQueue {
		proxyName := getProxyNameNamespace(proxy)
		proxyCounter, err := getProxySyncCounter(proxy)
		if err != nil {
			// ignore proxies that do not have a sync id
			continue
		}
		// update proxyCounter only if it is higher than the current one
		if currentCounter, ok := proxies[proxyName]; !ok || proxyCounter > currentCounter {
			proxies[proxyName] = proxyCounter
		}
	}
	f.proxiesPerRegistry[pluginRegistry] = proxies
}

// HandleProxyReports is a callback that applies status plugins to the proxies that have been queued
func (f *statusSyncerFactory) HandleProxyReports(ctx context.Context, proxiesWithReports []translatorutils.ProxyWithReports) {
	// ignore until the syncer has been initialized
	f.lock.RLock()
	defer f.lock.RUnlock()
	for reg, proxiesToSync := range f.proxiesPerRegistry {
		reg := reg
		var filteredProxiesWithReports []translatorutils.ProxyWithReports
		for _, proxyWithReports := range proxiesWithReports {
			proxyName := getProxyNameNamespace(proxyWithReports.Proxy)
			if _, ok := proxiesToSync[proxyName]; ok {
				filteredProxiesWithReports = append(filteredProxiesWithReports, proxyWithReports)
				delete(proxiesToSync, proxyName)
				break
			}
		}
		newStatusSyncer(reg).applyStatusPlugins(ctx, filteredProxiesWithReports)
		if len(proxiesToSync) == 0 {
			delete(f.proxiesPerRegistry, reg)
		}
	}
}

func (s *statusSyncerFactory) getProxiesPerRegistry() map[*registry.PluginRegistry]map[types.NamespacedName]int {
	return s.proxiesPerRegistry
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
