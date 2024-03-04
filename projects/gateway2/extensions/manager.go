package extensions

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

// ExtensionManager is responsible for providing implementations for translation utilities
// which have Enterprise variants.
type ExtensionManager interface {
	CreateGatewayQueries(ctx context.Context) query.GatewayQueries
	CreatePluginRegistry(ctx context.Context) registry.PluginRegistry
}

type ExtensionManagerFactory func(manager controllerruntime.Manager) ExtensionManager

// NewExtensionManager returns the Open Source implementation of ExtensionManager
func NewExtensionManager(manager controllerruntime.Manager) ExtensionManager {
	return &extensionManager{
		manager: manager,
	}
}

type extensionManager struct {
	manager controllerruntime.Manager

	queryEngine    query.GatewayQueries
	pluginRegistry registry.PluginRegistry
}

func (e *extensionManager) CreateGatewayQueries(ctx context.Context) query.GatewayQueries {
	if e.queryEngine != nil {
		return e.queryEngine
	}

	e.queryEngine = query.NewData(e.manager.GetClient(), e.manager.GetScheme())
	return e.queryEngine
}

func (e *extensionManager) CreatePluginRegistry(ctx context.Context) registry.PluginRegistry {
	if !e.pluginRegistry.IsNil() {
		return e.pluginRegistry
	}

	gatewayQueries := e.CreateGatewayQueries(ctx)
	plugins := registry.BuildPlugins(gatewayQueries)
	e.pluginRegistry = registry.NewPluginRegistry(plugins)
	return e.pluginRegistry
}
