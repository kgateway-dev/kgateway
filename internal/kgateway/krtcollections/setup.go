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

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
)

// registertypes for common collections

func registerTypes(ourCli versioned.Interface) {
	// if it is a sig gateway api then it can pull directly from the kubeclientgetter
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
}

func InitCollections(
	ctx context.Context,
	controllerName string,
	plugins extensionsplug.Plugin,
	istioClient kube.Client,
	ourClient versioned.Interface,
	refgrants *RefGrantIndex,
	krtopts krtutil.KrtOptions,
) (*GatewayIndex, *RoutesIndex, *BackendIndex, krt.Collection[ir.EndpointsForBackend]) {
	registerTypes(ourClient)

	// create the KRT clients, remember to also register any needed types in the type registration setup.
	httpRoutes := krt.WrapClient(kclient.New[*gwv1.HTTPRoute](istioClient), krtopts.ToOptions("HTTPRoute")...)
	tcproutes := krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TCPRoute](istioClient, gvr.TCPRoute, kubetypes.StandardInformer, kclient.Filter{}), krtopts.ToOptions("TCPRoute")...)
	tlsRoutes := krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TLSRoute](istioClient, gvr.TLSRoute, kubetypes.StandardInformer, kclient.Filter{}), krtopts.ToOptions("TLSRoute")...)
	kubeRawGateways := krt.WrapClient(kclient.New[*gwv1.Gateway](istioClient), krtopts.ToOptions("KubeGateways")...)
	gatewayClasses := krt.WrapClient(kclient.New[*gwv1.GatewayClass](istioClient), krtopts.ToOptions("KubeGatewayClasses")...)

	policies := NewPolicyIndex(krtopts, plugins.ContributesPolicies)
	var backendRefPlugins []extensionsplug.GetBackendForRefPlugin
	for _, ext := range plugins.ContributesPolicies {
		if ext.GetBackendForRef != nil {
			backendRefPlugins = append(backendRefPlugins, ext.GetBackendForRef)
		}
	}
	backendIndex := NewBackendIndex(krtopts, backendRefPlugins, policies, refgrants)
	initBackends(plugins, backendIndex)
	endpointIRs := initEndpoints(plugins, krtopts)
	gateways := NewGatewayIndex(krtopts, controllerName, policies, kubeRawGateways, gatewayClasses)

	routes := NewRoutesIndex(krtopts, httpRoutes, tcproutes, tlsRoutes, policies, backendIndex, refgrants)
	return gateways, routes, backendIndex, endpointIRs
}

func initBackends(plugins extensionsplug.Plugin, backendIndex *BackendIndex) {
	for gk, plugin := range plugins.ContributesBackends {
		if plugin.Backends != nil {
			backendIndex.AddBackends(gk, plugin.Backends)
		}
	}
}

func initEndpoints(plugins extensionsplug.Plugin, krtopts krtutil.KrtOptions) krt.Collection[ir.EndpointsForBackend] {
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
