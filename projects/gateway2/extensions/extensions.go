package extensions

import (
	"context"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

// K8sGatewayExtensions is responsible for providing implementations for translation utilities
// which have Enterprise variants.
type K8sGatewayExtensions interface {
	// CreatePluginRegistry returns the PluginRegistry
	CreatePluginRegistry(ctx context.Context) registry.PluginRegistry
}

// K8sGatewayExtensionsFactoryParameters contains the parameters required to start Gloo K8s Gateway Extensions (including Translator Plugins)
type K8sGatewayExtensionsFactoryParameters struct {
	Mgr                     controllerruntime.Manager
	AuthConfigClient        v1.AuthConfigClient
	RouteOptionClient       gatewayv1.RouteOptionClient
	VirtualHostOptionClient gatewayv1.VirtualHostOptionClient
	StatusReporter          reporter.StatusReporter
	KickXds                 func(ctx context.Context)
}

// K8sGatewayExtensionsFactory returns an extensions.K8sGatewayExtensions
type K8sGatewayExtensionsFactory func(
	ctx context.Context,
	params K8sGatewayExtensionsFactoryParameters,
) (K8sGatewayExtensions, error)

// NewK8sGatewayExtensions returns the Open Source implementation of K8sGatewayExtensions
func NewK8sGatewayExtensions(
	_ context.Context,
	params K8sGatewayExtensionsFactoryParameters,
) (K8sGatewayExtensions, error) {
	return &k8sGatewayExtensions{
		mgr:                     params.Mgr,
		routeOptionClient:       params.RouteOptionClient,
		virtualHostOptionClient: params.VirtualHostOptionClient,
		statusReporter:          params.StatusReporter,
	}, nil
}

type k8sGatewayExtensions struct {
	mgr                     controllerruntime.Manager
	routeOptionClient       gatewayv1.RouteOptionClient
	virtualHostOptionClient gatewayv1.VirtualHostOptionClient
	statusReporter          reporter.StatusReporter
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
		e.virtualHostOptionClient,
		e.statusReporter,
	)
	return registry.NewPluginRegistry(plugins)
}
