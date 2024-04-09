package extensions

import (
	"context"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

// K8sGatewayExtensions is responsible for providing implementations for translation utilities
// which have Enterprise variants.
type K8sGatewayExtensions interface {
	// CreatePluginRegistry returns the PluginRegistry
	CreatePluginRegistry(ctx context.Context) registry.PluginRegistry
}

// K8sGatewayExtensionsFactory returns an extensions.K8sGatewayExtensions
type K8sGatewayExtensionsFactory func(
	mgr controllerruntime.Manager,
	routeOptionClient gatewayv1.RouteOptionClient,
	statusReporter reporter.StatusReporter,
) (K8sGatewayExtensions, error)

// NewK8sGatewayExtensions returns the Open Source implementation of K8sGatewayExtensions
func NewK8sGatewayExtensions(
	mgr controllerruntime.Manager,
	routeOptionClient gatewayv1.RouteOptionClient,
	statusReporter reporter.StatusReporter,
) (K8sGatewayExtensions, error) {
	return &k8sGatewayExtensions{
		mgr,
		routeOptionClient,
		statusReporter,
	}, nil
}

type k8sGatewayExtensions struct {
	mgr               controllerruntime.Manager
	routeOptionClient gatewayv1.RouteOptionClient
	statusReporter    reporter.StatusReporter
}

// CreatePluginRegistry returns the PluginRegistry
func (e *k8sGatewayExtensions) CreatePluginRegistry(_ context.Context) registry.PluginRegistry {
	queries := query.NewData(
		e.mgr.GetClient(),
		e.mgr.GetScheme(),
	)
	plugins := registry.BuildPlugins(
		queries,
		e.mgr.GetClient(),
		e.routeOptionClient,
		e.statusReporter,
	)
	return registry.NewPluginRegistry(plugins)
}
