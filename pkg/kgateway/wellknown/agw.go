package wellknown

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	agwv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
)

var (
	AgentgatewayPolicyGVK  = buildAgwGvk("AgentgatewayPolicy")
	AgentgatewayBackendGVK = buildAgwGvk("AgentgatewayBackend")
	AgentgatewayPolicyGVR  = AgentgatewayPolicyGVK.GroupVersion().WithResource("agentgatewaypolicies")
	AgentgatewayBackendGVR = AgentgatewayBackendGVK.GroupVersion().WithResource("agentgatewaybackends")
)

func buildAgwGvk(kind string) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   agwv1alpha1.GroupName,
		Version: agwv1alpha1.GroupVersion.Version,
		Kind:    kind,
	}
}
