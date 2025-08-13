package krtcollections

import (
	"context"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/config/schema/kubeclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

// registertypes for common collections

func registerTypes(_ versioned.Interface) {
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
	kubeclient.Register[*gwv1a2.TCPRoute](
		gvr.TCPRoute,
		gvk.TCPRoute.Kubernetes(),
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).Watch(context.Background(), o)
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

	// TODO: Update when istio supports ListenerSets
	kubeclient.Register[*gwxv1a1.XListenerSet](
		wellknown.XListenerSetGVR,
		wellknown.XListenerSetGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().ExperimentalV1alpha1().XListenerSets(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().ExperimentalV1alpha1().XListenerSets(namespace).Watch(context.Background(), o)
		},
	)
}

func InitCollections(
	ctx context.Context,
	controllerName string,
	plugins extensionsplug.Plugin,
	istioClient kube.Client,
	ourClient versioned.Interface,
	refgrants *RefGrantIndex,
	krtopts krtinternal.KrtOptions,
	globalSettings settings.Settings,
) (*GatewayIndex, *RoutesIndex, *BackendIndex, krt.Collection[ir.EndpointsForBackend]) {
	registerTypes(ourClient)

	// discovery filter
	filter := kclient.Filter{ObjectFilter: istioClient.ObjectFilter()}

	// create the KRT clients, remember to also register any needed types in the type registration setup.
	httpRoutes := krt.WrapClient(kclient.NewFiltered[*gwv1.HTTPRoute](istioClient, filter), krtopts.ToOptions("HTTPRoute")...)
	metrics.RegisterEvents(httpRoutes, GetResourceMetricEventHandler[*gwv1.HTTPRoute]())

	tcproutes := krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TCPRoute](istioClient, gvr.TCPRoute, kubetypes.StandardInformer, filter), krtopts.ToOptions("TCPRoute")...)
	metrics.RegisterEvents(tcproutes, GetResourceMetricEventHandler[*gwv1a2.TCPRoute]())

	tlsRoutes := krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TLSRoute](istioClient, gvr.TLSRoute, kubetypes.StandardInformer, filter), krtopts.ToOptions("TLSRoute")...)
	metrics.RegisterEvents(tlsRoutes, GetResourceMetricEventHandler[*gwv1a2.TLSRoute]())

	grpcRoutes := krt.WrapClient(kclient.NewFiltered[*gwv1.GRPCRoute](istioClient, filter), krtopts.ToOptions("GRPCRoute")...)
	metrics.RegisterEvents(grpcRoutes, GetResourceMetricEventHandler[*gwv1.GRPCRoute]())

	kubeRawGateways := krt.WrapClient(kclient.NewFiltered[*gwv1.Gateway](istioClient, filter), krtopts.ToOptions("KubeGateways")...)
	metrics.RegisterEvents(kubeRawGateways, GetResourceMetricEventHandler[*gwv1.Gateway]())

	kubeRawListenerSets := krt.WrapClient(kclient.NewDelayedInformer[*gwxv1a1.XListenerSet](istioClient, wellknown.XListenerSetGVR, kubetypes.StandardInformer, filter), krtopts.ToOptions("KubeListenerSets")...)
	metrics.RegisterEvents(kubeRawListenerSets, GetResourceMetricEventHandler[*gwxv1a1.XListenerSet]())

	//nolint:forbidigo // ObjectFilter is not needed for this client as it is cluster scoped
	gatewayClasses := krt.WrapClient(kclient.New[*gwv1.GatewayClass](istioClient), krtopts.ToOptions("KubeGatewayClasses")...)

	namespaces, _ := NewNamespaceCollection(ctx, istioClient, krtopts)

	policies := NewPolicyIndex(krtopts, plugins.ContributesPolicies, globalSettings)
	for _, plugin := range plugins.ContributesPolicies {
		if plugin.Policies != nil {
			metrics.RegisterEvents(plugin.Policies, GetResourceMetricEventHandler[ir.PolicyWrapper]())
		}
	}

	backendIndex := NewBackendIndex(krtopts, policies, refgrants)
	initBackends(plugins, backendIndex)
	endpointIRs := initEndpoints(plugins, krtopts)

	gateways := NewGatewayIndex(krtopts, controllerName, policies, kubeRawGateways, kubeRawListenerSets, gatewayClasses, namespaces)
	routes := NewRoutesIndex(krtopts, httpRoutes, grpcRoutes, tcproutes, tlsRoutes, policies, backendIndex, refgrants, globalSettings)
	return gateways, routes, backendIndex, endpointIRs
}

func initBackends(plugins extensionsplug.Plugin, backendIndex *BackendIndex) {
	for gk, plugin := range plugins.ContributesBackends {
		if plugin.Backends != nil {
			backendIndex.AddBackends(gk, plugin.Backends, plugin.AliasKinds...)
		}
	}
}

func initEndpoints(plugins extensionsplug.Plugin, krtopts krtinternal.KrtOptions) krt.Collection[ir.EndpointsForBackend] {
	allEndpoints := []krt.Collection[ir.EndpointsForBackend]{}
	for _, plugin := range plugins.ContributesBackends {
		if plugin.Endpoints != nil {
			allEndpoints = append(allEndpoints, plugin.Endpoints)
		}
	}
	// build Endpoint intermediate representation from kubernetes service and extensions
	// TODO move kube service to be an extension
	endpointIRs := krt.JoinCollection(allEndpoints, krtopts.ToOptions("EndpointIRs")...)
	return endpointIRs
}
