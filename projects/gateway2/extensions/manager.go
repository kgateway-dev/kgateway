package extensions

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

// Manager is responsible for providing implementations for translation utilities
// which have Enterprise variants.
type Manager interface {
	// CreateGatewayQueries returns the GatewayQueries
	CreateGatewayQueries(ctx context.Context) query.GatewayQueries

	// CreatePluginRegistry returns the PluginRegistry
	CreatePluginRegistry(ctx context.Context) registry.PluginRegistry
}

// ManagerFactory returns an extensions.Manager
type ManagerFactory func(manager controllerruntime.Manager) Manager

// NewManager returns the Open Source implementation of Manager
func NewManager(mgr controllerruntime.Manager) Manager {
	return &manager{
		mgr: mgr,
	}
}

type manager struct {
	mgr controllerruntime.Manager
}

// CreateGatewayQueries returns the GatewayQueries
func (m *manager) CreateGatewayQueries(ctx context.Context) query.GatewayQueries {
	return query.NewData(m.mgr.GetClient(), m.mgr.GetScheme())
}

// CreatePluginRegistry returns the PluginRegistry
func (m *manager) CreatePluginRegistry(ctx context.Context) registry.PluginRegistry {
	gatewayQueries := m.CreateGatewayQueries(ctx)
	plugins := registry.BuildPlugins(gatewayQueries)
	return registry.NewPluginRegistry(plugins)
}
