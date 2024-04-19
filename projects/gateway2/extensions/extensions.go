package extensions

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"

	"github.com/solo-io/gloo/pkg/version"
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

	// GetEnvoyImage returns the envoy image and tag used by the proxy deployment.
	GetEnvoyImage() Image
}

// K8sGatewayExtensionsFactoryParameters contains the parameters required to start Gloo K8s Gateway Extensions (including Translator Plugins)
type K8sGatewayExtensionsFactoryParameters struct {
	Mgr               controllerruntime.Manager
	AuthConfigClient  v1.AuthConfigClient
	RouteOptionClient gatewayv1.RouteOptionClient
	StatusReporter    reporter.StatusReporter
	KickXds           func(ctx context.Context)
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
		params.Mgr,
		params.RouteOptionClient,
		params.StatusReporter,
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

// GetEnvoyImage returns the image repo and tag to use for the envoy container image
// in the proxy deployment.
func (e *k8sGatewayExtensions) GetEnvoyImage() Image {
	return Image{
		Repository: "gloo-envoy-wrapper",
		Tag:        version.Version,
	}
}

// Image contains an image repository (e.g. "gloo-envoy-wrapper") and tag (e.g. "1.17.0").
//
// The Image struct is provided here so that OSS and Enterprise Gloo Gateway can each inject
// their default image repo/tag (e.g. "gloo-envoy-wrapper" for OSS, "gloo-ee-envoy-wrapper" for EE),
// for images that differ between OSS and EE. For now, it's only used for the Envoy wrapper image,
// but could potentially be used for other images in the future.
//
// Users may override the default OSS/EE Envoy image repo/tag (as well as other fields) completely via
// a GatewayParameters CR attached to a Gateway.
type Image struct {
	Repository string
	Tag        string
}
