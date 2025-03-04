package wellknown

import (
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func buildKgatewayGvk(kind string) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   v1alpha1.GroupName,
		Version: v1alpha1.GroupVersion.Version,
		Kind:    kind,
	}
}

// TODO: consider generating these?
var (
	GatewayParametersGVK  = buildKgatewayGvk("GatewayParameters")
	DirectResponseGVK     = buildKgatewayGvk("DirectResponse")
	BackendGVK            = buildKgatewayGvk("Backend")
	RoutePolicyGVK        = buildKgatewayGvk("RoutePolicy")
	ListenerPolicyGVK     = buildKgatewayGvk("ListenerPolicy")
	HTTPListenerPolicyGVK = buildKgatewayGvk("HTTPListenerPolicy")
)
