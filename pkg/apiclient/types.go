package apiclient

import (
	"context"

	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kubetypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// RegisterTypes registers all the types used by our API Client
func RegisterTypes() {
	// kgateway types
	kubeclient.Register(
		wellknown.GatewayParametersGVR,
		wellknown.GatewayParametersGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().GatewayParameters(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().GatewayParameters(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.GatewayParameters] {
			return c.(Client).Kgateway().GatewayV1alpha1().GatewayParameters(namespace)
		},
	)
	kubeclient.Register(
		wellknown.BackendGVR,
		wellknown.BackendGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().Backends(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().Backends(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.Backend] {
			return c.(Client).Kgateway().GatewayV1alpha1().Backends(namespace)
		},
	)
	kubeclient.Register(
		wellknown.BackendConfigPolicyGVR,
		wellknown.BackendConfigPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().BackendConfigPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().BackendConfigPolicies(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.BackendConfigPolicy] {
			return c.(Client).Kgateway().GatewayV1alpha1().BackendConfigPolicies(namespace)
		},
	)
	kubeclient.Register(
		wellknown.DirectResponseGVR,
		wellknown.DirectResponseGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().DirectResponses(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().DirectResponses(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.DirectResponse] {
			return c.(Client).Kgateway().GatewayV1alpha1().DirectResponses(namespace)
		},
	)
	kubeclient.Register(
		wellknown.HTTPListenerPolicyGVR,
		wellknown.HTTPListenerPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().HTTPListenerPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().HTTPListenerPolicies(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.HTTPListenerPolicy] {
			return c.(Client).Kgateway().GatewayV1alpha1().HTTPListenerPolicies(namespace)
		},
	)
	kubeclient.Register(
		wellknown.ListenerPolicyGVR,
		wellknown.ListenerPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().ListenerPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().ListenerPolicies(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.ListenerPolicy] {
			return c.(Client).Kgateway().GatewayV1alpha1().ListenerPolicies(namespace)
		},
	)
	kubeclient.Register(
		wellknown.TrafficPolicyGVR,
		wellknown.TrafficPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().TrafficPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().TrafficPolicies(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.TrafficPolicy] {
			return c.(Client).Kgateway().GatewayV1alpha1().TrafficPolicies(namespace)
		},
	)
	kubeclient.Register(
		wellknown.GatewayExtensionGVR,
		wellknown.GatewayExtensionGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().GatewayExtensions(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().GatewayExtensions(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.GatewayExtension] {
			return c.(Client).Kgateway().GatewayV1alpha1().GatewayExtensions(namespace)
		},
	)
	kubeclient.Register(
		wellknown.AgentgatewayPolicyGVR,
		wellknown.AgentgatewayPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().AgentgatewayPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().AgentgatewayPolicies(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.AgentgatewayPolicy] {
			return c.(Client).Kgateway().GatewayV1alpha1().AgentgatewayPolicies(namespace)
		},
	)
	kubeclient.Register(
		wellknown.AgentgatewayBackendGVR,
		wellknown.AgentgatewayBackendGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().AgentgatewayBackends(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayV1alpha1().AgentgatewayBackends(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*v1alpha1.AgentgatewayBackend] {
			return c.(Client).Kgateway().GatewayV1alpha1().AgentgatewayBackends(namespace)
		},
	)
}
