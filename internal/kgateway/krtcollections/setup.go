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
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

func registerTypes() {
	skubeclient.Register[*gwv1.HTTPRoute](
		gvr.HTTPRoute_v1,
		gvk.HTTPRoute_v1.Kubernetes(),
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1().HTTPRoutes(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1().HTTPRoutes(namespace).Watch(context.Background(), o)
		},
	)
	skubeclient.Register[*gwv1a2.TCPRoute](
		gvr.TCPRoute,
		gvk.TCPRoute.Kubernetes(),
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace).Watch(context.Background(), o)
		},
	)
	skubeclient.Register[*gwv1.Gateway](
		gvr.KubernetesGateway_v1,
		gvk.KubernetesGateway_v1.Kubernetes(),
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1().Gateways(namespace).List(context.Background(), o)
		},
		func(c skubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1().Gateways(namespace).Watch(context.Background(), o)
		},
	)
}

func InitCollections(
	ctx context.Context,
	extensions extensionsplug.Plugin,
	istioClient kube.Client,
	isOurGw func(gw *gwv1.Gateway) bool,
	refgrants *RefGrantIndex,
	krtopts krtutil.KrtOptions,
) (*GatewayIndex, *RoutesIndex, *BackendIndex, krt.Collection[ir.EndpointsForBackend]) {
	registerTypes()

	httpRoutes := krt.WrapClient(kclient.New[*gwv1.HTTPRoute](istioClient), krtopts.ToOptions("HTTPRoute")...)
	kubeRawGateways := krt.WrapClient(kclient.New[*gwv1.Gateway](istioClient), krtopts.ToOptions("KubeGateways")...)

	tcpRoutes := krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TCPRoute](istioClient, gvr.TCPRoute, kubetypes.StandardInformer, kclient.Filter{}), krtopts.ToOptions("TCPRoute")...)
	tlsRoutes := krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TLSRoute](istioClient, gvr.TLSRoute, kubetypes.StandardInformer, kclient.Filter{}), krtopts.ToOptions("TLSRoute")...)

	return initCollectionsWithGateways(isOurGw, kubeRawGateways, httpRoutes, tcpRoutes, tlsRoutes, refgrants, extensions, krtopts)
}

func initCollectionsWithGateways(
	isOurGw func(gw *gwv1.Gateway) bool,
	kubeRawGateways krt.Collection[*gwv1.Gateway],
	httpRoutes krt.Collection[*gwv1.HTTPRoute],
	tcproutes krt.Collection[*gwv1a2.TCPRoute],
	tlsRoutes krt.Collection[*gwv1a2.TLSRoute],
	refgrants *RefGrantIndex,
	extensions extensionsplug.Plugin,
	krtopts krtutil.KrtOptions,
) (*GatewayIndex, *RoutesIndex, *BackendIndex, krt.Collection[ir.EndpointsForBackend]) {
	policies := NewPolicyIndex(krtopts, extensions.ContributesPolicies)
	var backendRefPlugins []extensionsplug.GetBackendForRefPlugin
	for _, ext := range extensions.ContributesPolicies {
		if ext.GetBackendForRef != nil {
			backendRefPlugins = append(backendRefPlugins, ext.GetBackendForRef)
		}
	}

	backendIndex := NewBackendIndex(krtopts, backendRefPlugins, policies, refgrants)
	endpointIRs := initBackends(extensions, backendIndex, krtopts)

	kubeGateways := NewGatewayIndex(krtopts, isOurGw, policies, kubeRawGateways)

	routes := NewRoutesIndex(krtopts, httpRoutes, tcproutes, tlsRoutes, policies, backendIndex, refgrants)
	return kubeGateways, routes, backendIndex, endpointIRs
}

func initBackends(
	plugins extensionsplug.Plugin,
	upstreamIndex *BackendIndex,
	krtopts krtutil.KrtOptions,
) krt.Collection[ir.EndpointsForBackend] {
	allEndpoints := []krt.Collection[ir.EndpointsForBackend]{}
	for gk, plugin := range plugins.ContributesBackends {
		if plugin.Backends != nil {
			upstreamIndex.AddBackends(gk, plugin.Backends)
		}
		if plugin.Endpoints != nil {
			allEndpoints = append(allEndpoints, plugin.Endpoints)
		}
	}

	// build Endpoint intermediate representation from kubernetes service and extensions
	// TODO move kube service to be an extension
	endpointIRs := krt.JoinCollection(allEndpoints, krtopts.ToOptions("EndpointIRs")...)
	return endpointIRs
}
