package extensions

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

type ExtensionManager interface {
	CreateQueryEngine(ctx context.Context) query.Engine
	CreatePluginRegistry(ctx context.Context) registry.PluginRegistry
}

type ExtensionManagerFactory func(manager controllerruntime.Manager) ExtensionManager

func NewExtensionManager(manager controllerruntime.Manager) ExtensionManager {
	return &extensionManager{
		manager: manager,
	}
}

type extensionManager struct {
	manager controllerruntime.Manager

	queryEngine    query.Engine
	pluginRegistry registry.PluginRegistry
}

func (e *extensionManager) CreateQueryEngine(ctx context.Context) query.Engine {
	if e.queryEngine == nil {
		e.queryEngine = query.NewData(e.manager.GetClient(), e.manager.GetScheme())
	}

	return e.queryEngine
}

func (e *extensionManager) CreatePluginRegistry(ctx context.Context) registry.PluginRegistry {
	if e.pluginRegistry.IsNil() {
		queryEngine := e.CreateQueryEngine(ctx)
		plugins := registry.BuildPlugins(queryEngine)
		e.pluginRegistry = registry.NewPluginRegistry(plugins)
	}

	return e.pluginRegistry
}
