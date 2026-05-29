package apiclient

import (
	"context"
	"sync"

	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kubetypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

var registerOnce sync.Once

// RegisterTypes registers all the types used by our API Client.
// Safe to call multiple times; registration is performed only once.
func RegisterTypes() {
	registerOnce.Do(registerTypes)
}

func registerTypes() {
	// kgateway types
	kubeclient.Register(
		wellknown.GatewayParametersGVR,
		wellknown.GatewayParametersGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayKgateway().GatewayParameters(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayKgateway().GatewayParameters(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*kgateway.GatewayParameters] {
			return c.(Client).Kgateway().GatewayKgateway().GatewayParameters(namespace)
		},
	)
	kubeclient.Register(
		wellknown.TLSRouteGVR,
		wellknown.TLSRouteGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1alpha2().TLSRoutes(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1alpha2().TLSRoutes(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*gwv1a2.TLSRoute] {
			return c.GatewayAPI().GatewayV1alpha2().TLSRoutes(namespace)
		},
	)
	kubeclient.Register(
		wellknown.TLSRouteV1Alpha3GVR,
		wellknown.TLSRouteV1Alpha3GVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.GatewayAPI().GatewayV1alpha3().TLSRoutes(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.GatewayAPI().GatewayV1alpha3().TLSRoutes(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*gwv1a3.TLSRoute] {
			return c.GatewayAPI().GatewayV1alpha3().TLSRoutes(namespace)
		},
	)
	kubeclient.Register(
		wellknown.BackendGVR,
		wellknown.BackendGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayKgateway().Backends(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayKgateway().Backends(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*kgateway.Backend] {
			return c.(Client).Kgateway().GatewayKgateway().Backends(namespace)
		},
	)
	kubeclient.Register(
		wellknown.BackendConfigPolicyGVR,
		wellknown.BackendConfigPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayKgateway().BackendConfigPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayKgateway().BackendConfigPolicies(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*kgateway.BackendConfigPolicy] {
			return c.(Client).Kgateway().GatewayKgateway().BackendConfigPolicies(namespace)
		},
	)
	kubeclient.Register(
		wellknown.DirectResponseGVR,
		wellknown.DirectResponseGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayKgateway().DirectResponses(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayKgateway().DirectResponses(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*kgateway.DirectResponse] {
			return c.(Client).Kgateway().GatewayKgateway().DirectResponses(namespace)
		},
	)
	kubeclient.Register(
		wellknown.HTTPListenerPolicyGVR,
		wellknown.HTTPListenerPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayKgateway().HTTPListenerPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayKgateway().HTTPListenerPolicies(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*kgateway.HTTPListenerPolicy] {
			return c.(Client).Kgateway().GatewayKgateway().HTTPListenerPolicies(namespace)
		},
	)
	kubeclient.Register(
		wellknown.ListenerPolicyGVR,
		wellknown.ListenerPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayKgateway().ListenerPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayKgateway().ListenerPolicies(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*kgateway.ListenerPolicy] {
			return c.(Client).Kgateway().GatewayKgateway().ListenerPolicies(namespace)
		},
	)
	kubeclient.Register(
		wellknown.TrafficPolicyGVR,
		wellknown.TrafficPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayKgateway().TrafficPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayKgateway().TrafficPolicies(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*kgateway.TrafficPolicy] {
			return c.(Client).Kgateway().GatewayKgateway().TrafficPolicies(namespace)
		},
	)
	kubeclient.Register(
		wellknown.GatewayExtensionGVR,
		wellknown.GatewayExtensionGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayKgateway().GatewayExtensions(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayKgateway().GatewayExtensions(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*kgateway.GatewayExtension] {
			return c.(Client).Kgateway().GatewayKgateway().GatewayExtensions(namespace)
		},
	)
}
