package wellknown

import (
	"fmt"

	"istio.io/istio/pkg/config"
	istiogvk "istio.io/istio/pkg/config/schema/gvk"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func buildKgatewayGvk(kind string) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   v1alpha1.GroupName,
		Version: v1alpha1.GroupVersion.Version,
		Kind:    kind,
	}
}

// TODO: consider generating these?
// manually updated GVKs of the kgateway API types; for convenience
var (
	GatewayParametersGVK   = buildKgatewayGvk("GatewayParameters")
	GatewayExtensionGVK    = buildKgatewayGvk("GatewayExtension")
	DirectResponseGVK      = buildKgatewayGvk("DirectResponse")
	BackendGVK             = buildKgatewayGvk("Backend")
	TrafficPolicyGVK       = buildKgatewayGvk("TrafficPolicy")
	AgentgatewayPolicyGVK  = buildKgatewayGvk("AgentgatewayPolicy")
	HTTPListenerPolicyGVK  = buildKgatewayGvk("HTTPListenerPolicy")
	BackendConfigPolicyGVK = buildKgatewayGvk("BackendConfigPolicy")
	GatewayParametersGVR   = GatewayParametersGVK.GroupVersion().WithResource("gatewayparameters")
	GatewayExtensionGVR    = GatewayExtensionGVK.GroupVersion().WithResource("gatewayextensions")
	DirectResponseGVR      = DirectResponseGVK.GroupVersion().WithResource("directresponses")
	BackendGVR             = BackendGVK.GroupVersion().WithResource("backends")
	TrafficPolicyGVR       = TrafficPolicyGVK.GroupVersion().WithResource("trafficpolicies")
	AgentgatewayPolicyGVR  = AgentgatewayPolicyGVK.GroupVersion().WithResource("agentgatewaypolicies")
	HTTPListenerPolicyGVR  = HTTPListenerPolicyGVK.GroupVersion().WithResource("httplistenerpolicies")
	BackendConfigPolicyGVR = BackendConfigPolicyGVK.GroupVersion().WithResource("backendconfigpolicies")
)

// GVKToGVR maps a known kgateway GVK to its corresponding GVR
func GVKToGVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	// Try Istio lib to resolve common GVKs
	istioGVK := config.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind,
	}
	gvr, found := istiogvk.ToGVR(istioGVK)
	if found {
		return gvr, nil
	}

	// Try kgateway types
	switch gvk {
	case GatewayParametersGVK:
		return GatewayParametersGVR, nil
	case GatewayExtensionGVK:
		return GatewayExtensionGVR, nil
	case DirectResponseGVK:
		return DirectResponseGVR, nil
	case BackendGVK:
		return BackendGVR, nil
	case TrafficPolicyGVK:
		return TrafficPolicyGVR, nil
	case HTTPListenerPolicyGVK:
		return HTTPListenerPolicyGVR, nil
	case BackendConfigPolicyGVK:
		return BackendConfigPolicyGVR, nil
	case AgentgatewayPolicyGVK:
		return AgentgatewayPolicyGVR, nil
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("unknown GVK: %v", gvk)
	}
}
