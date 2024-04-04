package extensions

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

// K8sGatewayExtensions is responsible for providing implementations for translation utilities
// which have Enterprise variants.
type K8sGatewayExtensions interface {
	// CreatePluginRegistry returns the PluginRegistry
	CreatePluginRegistry(ctx context.Context) registry.PluginRegistry
}

// K8sGatewayExtensionsFactory returns an extensions.K8sGatewayExtensions
type K8sGatewayExtensionsFactory func(mgr controllerruntime.Manager) (K8sGatewayExtensions, error)

// NewK8sGatewayExtensions returns the Open Source implementation of K8sGatewayExtensions
func NewK8sGatewayExtensions(mgr controllerruntime.Manager) (K8sGatewayExtensions, error) {
	return &k8sGatewayExtensions{
		mgr: mgr,
	}, nil
}

type k8sGatewayExtensions struct {
	mgr controllerruntime.Manager
}

// CreatePluginRegistry returns the PluginRegistry
func (e *k8sGatewayExtensions) CreatePluginRegistry(_ context.Context) registry.PluginRegistry {
	queries := query.NewData(
		e.mgr.GetClient(),
		e.mgr.GetScheme(),
	)
	plugins := registry.BuildPlugins(queries, e.mgr.GetClient())
	return registry.NewPluginRegistry(plugins)
}
