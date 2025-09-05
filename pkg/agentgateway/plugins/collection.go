package plugins

import (
	"context"
	"log/slog"

	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/config/schema/kubeclient"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	infversioned "sigs.k8s.io/gateway-api-inference-extension/client-go/clientset/versioned"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	kgwversioned "github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
)

type AgwCollections struct {
	OurClient kgwversioned.Interface
	Client    istiokube.Client
	// full CRUD client, only needed for status writing currently
	CrudClient client.Client
	KrtOpts    krtutil.KrtOptions

	// Core Kubernetes resources
	Namespaces     krt.Collection[*corev1.Namespace]
	Services       krt.Collection[*corev1.Service]
	Secrets        krt.Collection[*corev1.Secret]
	ConfigMaps     krt.Collection[*corev1.ConfigMap]
	EndpointSlices krt.Collection[*discovery.EndpointSlice]

	// Gateway API resources
	GatewayClasses     krt.Collection[*gwv1.GatewayClass]
	Gateways           krt.Collection[*gwv1.Gateway]
	HTTPRoutes         krt.Collection[*gwv1.HTTPRoute]
	GRPCRoutes         krt.Collection[*gwv1.GRPCRoute]
	TCPRoutes          krt.Collection[*gwv1alpha2.TCPRoute]
	TLSRoutes          krt.Collection[*gwv1alpha2.TLSRoute]
	ReferenceGrants    krt.Collection[*gwv1beta1.ReferenceGrant]
	BackendTLSPolicies krt.Collection[*gwv1alpha3.BackendTLSPolicy]

	// Extended resources
	InferencePools krt.Collection[*inf.InferencePool]

	// irs shared by common colections and agent gateway
	WrappedPods krt.Collection[krtcollections.WrappedPod]
	RefGrants   *krtcollections.RefGrantIndex

	// kgateway resources
	// TODO(npolshak): remove backendindex as part of https://github.com/kgateway-dev/kgateway/issues/12052
	BackendIndex      *krtcollections.BackendIndex
	Backends          krt.Collection[*v1alpha1.Backend]
	TrafficPolicies   krt.Collection[*v1alpha1.TrafficPolicy]
	DirectResponses   krt.Collection[*v1alpha1.DirectResponse]
	GatewayExtensions krt.Collection[*v1alpha1.GatewayExtension]

	// Plugin-initialized collections (set by InitPlugins)
	Routes       *krtcollections.RoutesIndex
	Endpoints    krt.Collection[ir.EndpointsForBackend]
	GatewayIndex *krtcollections.GatewayIndex

	ControllerName string
}

func registerKgwResources(kgwClient kgwversioned.Interface) {
	kubeclient.Register[*v1alpha1.Backend](
		wellknown.BackendGVR,
		wellknown.BackendGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return kgwClient.GatewayV1alpha1().Backends(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return kgwClient.GatewayV1alpha1().Backends(namespace).Watch(context.Background(), o)
		},
	)
	kubeclient.Register[*v1alpha1.DirectResponse](
		wellknown.DirectResponseGVR,
		wellknown.DirectResponseGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return kgwClient.GatewayV1alpha1().DirectResponses(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return kgwClient.GatewayV1alpha1().DirectResponses(namespace).Watch(context.Background(), o)
		},
	)
	kubeclient.Register[*v1alpha1.TrafficPolicy](
		wellknown.TrafficPolicyGVR,
		wellknown.TrafficPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return kgwClient.GatewayV1alpha1().TrafficPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return kgwClient.GatewayV1alpha1().TrafficPolicies(namespace).Watch(context.Background(), o)
		},
	)
}

func registerGatewayAPITypes() {
	// Register Gateway API types with kubeclient system
	kubeclient.Register[*gwv1.GatewayClass](
		gvr.GatewayClass_v1,
		gvk.GatewayClass_v1.Kubernetes(),
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1().GatewayClasses().List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1().GatewayClasses().Watch(context.Background(), o)
		},
	)
	kubeclient.Register[*gwv1.Gateway](
		gvr.KubernetesGateway_v1,
		gvk.KubernetesGateway_v1.Kubernetes(),
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1().Gateways(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1().Gateways(namespace).Watch(context.Background(), o)
		},
	)
	kubeclient.Register[*gwv1.HTTPRoute](
		gvr.HTTPRoute_v1,
		gvk.HTTPRoute_v1.Kubernetes(),
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1().HTTPRoutes(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1().HTTPRoutes(namespace).Watch(context.Background(), o)
		},
	)
	kubeclient.Register[*gwv1.GRPCRoute](
		gvr.GRPCRoute,
		gvk.GRPCRoute.Kubernetes(),
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1().GRPCRoutes(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1().GRPCRoutes(namespace).Watch(context.Background(), o)
		},
	)
	kubeclient.Register[*gwv1beta1.ReferenceGrant](
		gvr.ReferenceGrant,
		gvk.ReferenceGrant.Kubernetes(),
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1beta1().ReferenceGrants(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1beta1().ReferenceGrants(namespace).Watch(context.Background(), o)
		},
	)
	kubeclient.Register[*gwv1alpha2.TCPRoute](
		gvr.TCPRoute,
		gvk.TCPRoute.Kubernetes(),
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).Watch(context.Background(), o)
		},
	)
	kubeclient.Register[*gwv1alpha2.TLSRoute](
		gvr.TLSRoute,
		gvk.TLSRoute.Kubernetes(),
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1alpha2().TLSRoutes(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1alpha2().TLSRoutes(namespace).Watch(context.Background(), o)
		},
	)
}

func registerInferenceExtensionTypes(client istiokube.Client) {
	// Create the inference extension clientset.
	inferencePoolGVR := wellknown.InferencePoolGVK.GroupVersion().WithResource("inferencepools")
	infCli, err := infversioned.NewForConfig(client.RESTConfig())
	if err != nil {
		slog.Error("failed to create inference extension client", "error", err)
	} else {
		kubeclient.Register[*inf.InferencePool](
			inferencePoolGVR,
			wellknown.InferencePoolGVK,
			func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
				return infCli.InferenceV1().InferencePools(namespace).List(context.Background(), o)
			},
			func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
				return infCli.InferenceV1().InferencePools(namespace).Watch(context.Background(), o)
			},
		)
	}
}

func (c *AgwCollections) HasSynced() bool {
	// we check nil as well because some of the inner
	// collections aren't initialized until we call InitPlugins
	return c.Namespaces != nil && c.Namespaces.HasSynced() &&
		c.Services != nil && c.Services.HasSynced() &&
		c.Secrets != nil && c.Secrets.HasSynced() &&
		c.ConfigMaps != nil && c.ConfigMaps.HasSynced() &&
		c.GatewayClasses != nil && c.GatewayClasses.HasSynced() &&
		c.Gateways != nil && c.Gateways.HasSynced() &&
		c.HTTPRoutes != nil && c.HTTPRoutes.HasSynced() &&
		c.GRPCRoutes != nil && c.GRPCRoutes.HasSynced() &&
		c.TCPRoutes != nil && c.TCPRoutes.HasSynced() &&
		c.TLSRoutes != nil && c.TLSRoutes.HasSynced() &&
		c.ReferenceGrants != nil && c.ReferenceGrants.HasSynced() &&
		c.BackendTLSPolicies != nil && c.BackendTLSPolicies.HasSynced() &&
		c.InferencePools != nil && c.InferencePools.HasSynced() &&
		c.WrappedPods != nil && c.WrappedPods.HasSynced() &&
		c.RefGrants != nil && c.RefGrants.HasSynced() &&
		c.BackendIndex != nil && c.BackendIndex.HasSynced() &&
		c.Backends != nil && c.Backends.HasSynced() &&
		c.TrafficPolicies != nil && c.TrafficPolicies.HasSynced() &&
		c.DirectResponses != nil && c.DirectResponses.HasSynced() &&
		c.GatewayExtensions != nil && c.GatewayExtensions.HasSynced() &&
		c.Routes != nil && c.Routes.HasSynced() &&
		c.Endpoints != nil && c.Endpoints.HasSynced() &&
		c.GatewayIndex != nil && c.GatewayIndex.Gateways.HasSynced()
}

// NewAgwCollections initializes the core krt collections.
// Collections that rely on plugins aren't initialized here,
// and InitPlugins must be called.
func NewAgwCollections(
	commoncol *collections.CommonCollections,
) (*AgwCollections, error) {
	// Register Gateway API and kgateway types with Istio kubeclient system
	registerGatewayAPITypes()
	registerInferenceExtensionTypes(commoncol.Client)
	registerKgwResources(commoncol.OurClient)

	agwCollections := &AgwCollections{
		Client:         commoncol.Client,
		ControllerName: commoncol.ControllerName,

		// Core Kubernetes resources
		Namespaces: krt.NewInformer[*corev1.Namespace](commoncol.Client),
		Secrets: krt.WrapClient[*corev1.Secret](
			kclient.NewFiltered[*corev1.Secret](commoncol.Client, kubetypes.Filter{
				ObjectFilter: commoncol.Client.ObjectFilter(),
			}),
		),
		ConfigMaps: krt.WrapClient[*corev1.ConfigMap](
			kclient.NewFiltered[*corev1.ConfigMap](commoncol.Client, kubetypes.Filter{
				ObjectFilter: commoncol.Client.ObjectFilter(),
			}),
			commoncol.KrtOpts.ToOptions("informer/ConfigMaps")...,
		),
		Services: krt.WrapClient[*corev1.Service](
			kclient.NewFiltered[*corev1.Service](commoncol.Client, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}),
			commoncol.KrtOpts.ToOptions("informer/Services")...),
		EndpointSlices: krt.WrapClient[*discovery.EndpointSlice](
			kclient.NewFiltered[*discovery.EndpointSlice](commoncol.Client, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}),
			commoncol.KrtOpts.ToOptions("informer/EndpointSlices")...),

		// Gateway API resources
		GatewayClasses: krt.WrapClient(kclient.NewFiltered[*gwv1.GatewayClass](commoncol.Client, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/GatewayClasses")...),
		Gateways:       krt.WrapClient(kclient.NewFiltered[*gwv1.Gateway](commoncol.Client, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/Gateways")...),
		HTTPRoutes:     krt.WrapClient(kclient.NewFiltered[*gwv1.HTTPRoute](commoncol.Client, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/HTTPRoutes")...),
		GRPCRoutes:     krt.WrapClient(kclient.NewFiltered[*gwv1.GRPCRoute](commoncol.Client, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/GRPCRoutes")...),

		ReferenceGrants: krt.WrapClient(kclient.NewFiltered[*gwv1beta1.ReferenceGrant](commoncol.Client, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/ReferenceGrants")...),

		// kubernetes gateway alpha apis
		TCPRoutes:          krt.WrapClient(kclient.NewDelayedInformer[*gwv1alpha2.TCPRoute](commoncol.Client, gvr.TCPRoute, kubetypes.StandardInformer, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/TCPRoutes")...),
		TLSRoutes:          krt.WrapClient(kclient.NewDelayedInformer[*gwv1alpha2.TLSRoute](commoncol.Client, gvr.TLSRoute, kubetypes.StandardInformer, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/TLSRoutes")...),
		BackendTLSPolicies: krt.WrapClient(kclient.NewDelayedInformer[*gwv1alpha3.BackendTLSPolicy](commoncol.Client, gvr.BackendTLSPolicy, kubetypes.StandardInformer, kubetypes.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/BackendTLSPolicies")...),

		// inference extensions need to be enabled so control plane has permissions to watch resource. Disable by default
		InferencePools: krt.NewStaticCollection[*inf.InferencePool](nil, nil, commoncol.KrtOpts.ToOptions("disable/inferencepools")...),

		// common collections
		WrappedPods: commoncol.WrappedPods,
		RefGrants:   commoncol.RefGrants,

		// kgateway resources
		DirectResponses:   krt.NewInformer[*v1alpha1.DirectResponse](commoncol.Client),
		TrafficPolicies:   krt.NewInformer[*v1alpha1.TrafficPolicy](commoncol.Client),
		GatewayExtensions: krt.NewInformer[*v1alpha1.GatewayExtension](commoncol.Client),
		Backends:          krt.NewInformer[*v1alpha1.Backend](commoncol.Client),
		BackendIndex:      commoncol.BackendIndex,
	}

	if commoncol.Settings.EnableInferExt {
		// inference extensions cluster watch permissions are controlled by enabling EnableInferExt
		inferencePoolGVR := wellknown.InferencePoolGVK.GroupVersion().WithResource("inferencepools")
		agwCollections.InferencePools = krt.WrapClient(kclient.NewDelayedInformer[*inf.InferencePool](commoncol.Client, inferencePoolGVR, kubetypes.StandardInformer, kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}), commoncol.KrtOpts.ToOptions("informer/InferencePools")...)
	}

	return agwCollections, nil
}

// InitPlugins set up collections that rely on plugins.
// This can't be part of NewAgwCollections because the setup
// of plugins themselves rely on a reference to AgwCollections.
func (c *AgwCollections) InitPlugins(
	ctx context.Context,
	mergedPlugins extensionsplug.Plugin,
	globalSettings settings.Settings,
) {
	gateways, routeIndex, backendIndex, endpointIRs := krtcollections.InitCollections(
		ctx,
		c.ControllerName,
		mergedPlugins,
		c.Client,
		c.OurClient,
		c.RefGrants,
		c.KrtOpts,
		globalSettings,
	)

	// init plugin-extended collections
	c.BackendIndex = backendIndex
	c.Routes = routeIndex
	c.Endpoints = endpointIRs
	c.GatewayIndex = gateways
}
