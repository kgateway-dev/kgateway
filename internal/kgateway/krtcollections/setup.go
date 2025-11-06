package krtcollections

import (
	"context"

	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/util/smallset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	kmetrics "github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
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
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*gwv1.HTTPRoute] {
			return c.GatewayAPI().GatewayV1().HTTPRoutes(namespace)
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
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*gwv1.GRPCRoute] {
			return c.GatewayAPI().GatewayV1().GRPCRoutes(namespace)
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
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*gwv1a2.TCPRoute] {
			return c.GatewayAPI().GatewayV1alpha2().TCPRoutes(namespace)
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
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*gwv1.Gateway] {
			return c.GatewayAPI().GatewayV1().Gateways(namespace)
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
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*gwv1.GatewayClass] {
			return c.GatewayAPI().GatewayV1().GatewayClasses()
		},
	)
}

func InitCollections(
	ctx context.Context,
	controllerNames smallset.Set[string],
	envoyControllerName string,
	plugins sdk.Plugin,
	istioClient kube.Client,
	ourClient versioned.Interface,
	refgrants *RefGrantIndex,
	krtopts krtutil.KrtOptions,
	globalSettings apisettings.Settings,
) (*GatewayIndex, *RoutesIndex, *BackendIndex, krt.Collection[ir.EndpointsForBackend]) {
	registerTypes(ourClient)
	// discovery filter
	filter := kclient.Filter{ObjectFilter: istioClient.ObjectFilter()}

	//nolint:forbidigo // ObjectFilter is not needed for this client as it is cluster scoped
	gatewayClasses := krt.WrapClient(kclient.New[*gwv1.GatewayClass](istioClient), krtopts.ToOptions("KubeGatewayClasses")...)

	namespaces, _ := NewNamespaceCollection(ctx, istioClient, krtopts)

	kubeRawGateways := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.Gateway](istioClient, wellknown.GatewayGVR, filter), krtopts.ToOptions("KubeGateways")...)
	metrics.RegisterEvents(kubeRawGateways, kmetrics.GetResourceMetricEventHandler[*gwv1.Gateway]())

	var kubeRawListenerSets krt.Collection[*gwxv1a1.XListenerSet]
	// ON_EXPERIMENTAL_PROMOTION : Remove this block
	// Ref: https://github.com/kgateway-dev/kgateway/issues/12827
	if globalSettings.EnableExperimentalGatewayAPIFeatures {
		kubeRawListenerSets = krt.WrapClient(kclient.NewDelayedInformer[*gwxv1a1.XListenerSet](istioClient, wellknown.XListenerSetGVR, kubetypes.StandardInformer, filter), krtopts.ToOptions("KubeListenerSets")...)
	} else {
		// If disabled, still build a collection but make it always empty
		kubeRawListenerSets = krt.NewStaticCollection[*gwxv1a1.XListenerSet](nil, nil, krtopts.ToOptions("disable/KubeListenerSets")...)
	}
	metrics.RegisterEvents(kubeRawListenerSets, kmetrics.GetResourceMetricEventHandler[*gwxv1a1.XListenerSet]())

	var policies *PolicyIndex
	if globalSettings.EnableEnvoy {
		policies = NewPolicyIndex(krtopts, plugins.ContributesPolicies, globalSettings)
		for _, plugin := range plugins.ContributesPolicies {
			if plugin.Policies != nil {
				metrics.RegisterEvents(plugin.Policies, kmetrics.GetResourceMetricEventHandler[ir.PolicyWrapper]())
			}
		}
	}

	gateways := NewGatewayIndex(krtopts, controllerNames, envoyControllerName, policies, kubeRawGateways, kubeRawListenerSets, gatewayClasses, namespaces)

	if !globalSettings.EnableEnvoy {
		// For now, the gateway index is used by Agentgateway as well in the deployer
		return gateways, nil, nil, nil
	}

	// create the KRT clients, remember to also register any needed types in the type registration setup.
	httpRoutes := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.HTTPRoute](istioClient, wellknown.HTTPRouteGVR, filter), krtopts.ToOptions("HTTPRoute")...)
	metrics.RegisterEvents(httpRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1.HTTPRoute]())

	tcproutes := krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TCPRoute](istioClient, gvr.TCPRoute, kubetypes.StandardInformer, filter), krtopts.ToOptions("TCPRoute")...)
	metrics.RegisterEvents(tcproutes, kmetrics.GetResourceMetricEventHandler[*gwv1a2.TCPRoute]())

	tlsRoutes := krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TLSRoute](istioClient, gvr.TLSRoute, kubetypes.StandardInformer, filter), krtopts.ToOptions("TLSRoute")...)
	metrics.RegisterEvents(tlsRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1a2.TLSRoute]())

	grpcRoutes := krt.WrapClient(kclient.NewFilteredDelayed[*gwv1.GRPCRoute](istioClient, wellknown.GRPCRouteGVR, filter), krtopts.ToOptions("GRPCRoute")...)
	metrics.RegisterEvents(grpcRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1.GRPCRoute]())

	backendIndex := NewBackendIndex(krtopts, policies, refgrants)
	initBackends(plugins, backendIndex)
	endpointIRs := initEndpoints(plugins, krtopts)

	routes := NewRoutesIndex(krtopts, httpRoutes, grpcRoutes, tcproutes, tlsRoutes, policies, backendIndex, refgrants, globalSettings)
	return gateways, routes, backendIndex, endpointIRs
}

func initBackends(plugins sdk.Plugin, backendIndex *BackendIndex) {
	for gk, plugin := range plugins.ContributesBackends {
		if plugin.Backends != nil {
			backendIndex.AddBackends(gk, plugin.Backends, plugin.AliasKinds...)
		}
	}
}

func initEndpoints(plugins sdk.Plugin, krtopts krtutil.KrtOptions) krt.Collection[ir.EndpointsForBackend] {
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
