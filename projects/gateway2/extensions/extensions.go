package extensions

import (
	"context"

	gatewaykubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"
	extauthkubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1/kube/apis/enterprise.gloo.solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"

	controllerruntime "sigs.k8s.io/controller-runtime"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// K8sGatewayExtensions is responsible for providing implementations for translation utilities
// which have Enterprise variants.
type K8sGatewayExtensions interface {
	// CreatePluginRegistry exposes the plugins supported by this implementation.
	CreatePluginRegistry(context.Context) registry.PluginRegistry

	// GetTranslator allows an extension to provide custom translation for
	// different gateway classes.
	GetTranslator(context.Context, *apiv1.Gateway, registry.PluginRegistry) translator.K8sGwTranslator

	KRTExtensions() krtcollections.KRTExtensions
}

type CoreCollections struct {
	AugmentedPods               krt.Collection[krtcollections.LocalityPod]
	AuthConfigCollection        krt.Collection[*extauthkubev1.AuthConfig]
	RouteOptionCollection       krt.Collection[*gatewaykubev1.RouteOption]
	VirtualHostOptionCollection krt.Collection[*gatewaykubev1.VirtualHostOption]
}

// K8sGatewayExtensionsFactoryParameters contains the parameters required to start Gloo K8s Gateway Extensions (including Translator Plugins)
type K8sGatewayExtensionsFactoryParameters struct {
	Mgr             controllerruntime.Manager
	IstioClient     istiokube.Client
	CoreCollections CoreCollections
	StatusReporter  reporter.StatusReporter
	KickXds         func(ctx context.Context)
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
	queries := query.NewData(
		params.Mgr.GetClient(),
		params.Mgr.GetScheme(),
		nil,
	)

	return &k8sGatewayExtensions{
		mgr:            params.Mgr,
		collections:    params.CoreCollections,
		statusReporter: params.StatusReporter,
		queries:        queries,
	}, nil
}

type k8sGatewayExtensions struct {
	mgr            controllerruntime.Manager
	collections    CoreCollections
	statusReporter reporter.StatusReporter
	translator     translator.K8sGwTranslator
	pluginRegistry registry.PluginRegistry
	queries        query.GatewayQueries
}

func (e *k8sGatewayExtensions) GetTranslator(_ context.Context, _ *apiv1.Gateway, pluginRegistry registry.PluginRegistry) translator.K8sGwTranslator {
	return translator.NewTranslator(e.queries, pluginRegistry)
}

func (e *k8sGatewayExtensions) CreatePluginRegistry(_ context.Context) registry.PluginRegistry {
	plugins := registry.BuildPlugins(
		e.queries,
		e.mgr.GetClient(),
		e.collections.RouteOptionCollection,
		e.collections.VirtualHostOptionCollection,
		e.statusReporter,
	)
	return registry.NewPluginRegistry(plugins)
}

func (e *k8sGatewayExtensions) KRTExtensions() krtcollections.KRTExtensions {
	return krtcollections.Aggregate()
}
