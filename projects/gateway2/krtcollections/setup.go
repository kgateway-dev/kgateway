package krtcollections

import (
	"context"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	extensionsplug "github.com/kgateway-dev/kgateway/projects/gateway2/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/projects/gateway2/ir"
	"github.com/kgateway-dev/kgateway/projects/gateway2/utils/krtutil"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/gvr"
	skubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
) (*GatewayIndex, *RoutesIndex, krt.Collection[ir.Upstream], krt.Collection[ir.EndpointsForUpstream]) {
	registerTypes()

	httpRoutes := krt.WrapClient(kclient.New[*gwv1.HTTPRoute](istioClient), krtopts.ToOptions("HTTPRoute")...)
	tcproutes := krt.WrapClient(kclient.New[*gwv1a2.TCPRoute](istioClient), krtopts.ToOptions("TCPRoute")...)
	kubeRawGateways := krt.WrapClient(kclient.New[*gwv1.Gateway](istioClient), krtopts.ToOptions("KubeGateways")...)

	return initCollectionsWithGateways(isOurGw, kubeRawGateways, httpRoutes, tcproutes, refgrants, extensions, krtopts)
}

func initCollectionsWithGateways(
	isOurGw func(gw *gwv1.Gateway) bool,
	kubeRawGateways krt.Collection[*gwv1.Gateway],
	httpRoutes krt.Collection[*gwv1.HTTPRoute],
	tcproutes krt.Collection[*gwv1a2.TCPRoute],
	refgrants *RefGrantIndex,
	extensions extensionsplug.Plugin,
	krtopts krtutil.KrtOptions,
) (*GatewayIndex, *RoutesIndex, krt.Collection[ir.Upstream], krt.Collection[ir.EndpointsForUpstream]) {

	policies := NewPolicyIndex(krtopts, extensions.ContributesPolicies)

	var backendRefPlugins []extensionsplug.GetBackendForRefPlugin
	for _, ext := range extensions.ContributesPolicies {
		if ext.GetBackendForRef != nil {
			backendRefPlugins = append(backendRefPlugins, ext.GetBackendForRef)
		}
	}

	upstreamIndex := NewUpstreamIndex(krtopts, backendRefPlugins, policies)
	finalUpstreams, endpointIRs := initUpstreams(extensions, upstreamIndex, krtopts)

	kubeGateways := NewGatewayIndex(krtopts, isOurGw, policies, kubeRawGateways)

	routes := NewRoutesIndex(krtopts, httpRoutes, tcproutes, policies, upstreamIndex, refgrants)
	return kubeGateways, routes, finalUpstreams, endpointIRs
}

func initUpstreams(
	extensions extensionsplug.Plugin,
	upstreamIndex *UpstreamIndex,
	krtopts krtutil.KrtOptions,
) (krt.Collection[ir.Upstream], krt.Collection[ir.EndpointsForUpstream]) {

	allEndpoints := []krt.Collection[ir.EndpointsForUpstream]{}
	for k, col := range extensions.ContributesUpstreams {
		if col.Upstreams != nil {
			upstreamIndex.AddUpstreams(k, col.Upstreams)
		}
		if col.Endpoints != nil {
			allEndpoints = append(allEndpoints, col.Endpoints)
		}
	}

	finalUpstreams := krt.JoinCollection(upstreamIndex.Upstreams(), krtopts.ToOptions("FinalUpstreams")...)

	// build Endpoint intermediate representation from kubernetes service and extensions
	// TODO move kube service to be an extension
	endpointIRs := krt.JoinCollection(allEndpoints, krtopts.ToOptions("EndpointIRs")...)
	return finalUpstreams, endpointIRs
}
