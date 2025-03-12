package wellknown

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	infextv1a1 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func buildKgatewayGvk(kind string) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   v1alpha1.GroupName,
		Version: v1alpha1.GroupVersion.Version,
		Kind:    kind,
	}
}

func buildInferExtGvk(kind string) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   infextv1a1.GroupVersion.Group,
		Version: v1alpha1.GroupVersion.Version,
		Kind:    kind,
	}
}

// TODO: consider generating these?
// manually updated GVKs of the kgateway API types; for convenience
var (
	GatewayParametersGVK  = buildKgatewayGvk("GatewayParameters")
	DirectResponseGVK     = buildKgatewayGvk("DirectResponse")
	BackendGVK            = buildKgatewayGvk("Backend")
	RoutePolicyGVK        = buildKgatewayGvk("RoutePolicy")
	ListenerPolicyGVK     = buildKgatewayGvk("ListenerPolicy")
	HTTPListenerPolicyGVK = buildKgatewayGvk("HTTPListenerPolicy")
	InferencePoolGVK      = buildInferExtGvk("InferencePool")

	GatewayParametersGVR  = GatewayParametersGVK.GroupVersion().WithResource("gatewayparameters")
	DirectResponseGVR     = DirectResponseGVK.GroupVersion().WithResource("directresponses")
	BackendGVR            = BackendGVK.GroupVersion().WithResource("backends")
	RoutePolicyGVR        = RoutePolicyGVK.GroupVersion().WithResource("routepolicies")
	ListenerPolicyGVR     = ListenerPolicyGVK.GroupVersion().WithResource("listenerpolicies")
	HTTPListenerPolicyGVR = HTTPListenerPolicyGVK.GroupVersion().WithResource("httplistenerpolicies")
)
