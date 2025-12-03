package apiclient

import (
	"context"

	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kubetypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// RegisterTypes registers all the types used by our API Client
func RegisterTypes() {
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
	kubeclient.Register(
		wellknown.AgentgatewayPolicyGVR,
		wellknown.AgentgatewayPolicyGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayAgentgateway().AgentgatewayPolicies(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayAgentgateway().AgentgatewayPolicies(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*agentgateway.AgentgatewayPolicy] {
			return c.(Client).Kgateway().GatewayAgentgateway().AgentgatewayPolicies(namespace)
		},
	)
	kubeclient.Register(
		wellknown.AgentgatewayBackendGVR,
		wellknown.AgentgatewayBackendGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return c.(Client).Kgateway().GatewayAgentgateway().AgentgatewayBackends(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return c.(Client).Kgateway().GatewayAgentgateway().AgentgatewayBackends(namespace).Watch(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string) kubetypes.WriteAPI[*agentgateway.AgentgatewayBackend] {
			return c.(Client).Kgateway().GatewayAgentgateway().AgentgatewayBackends(namespace)
		},
	)
}
