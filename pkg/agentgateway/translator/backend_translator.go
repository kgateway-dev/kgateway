package translator

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	agentgatewaybackend "github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/backend"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// AgentGatewayBackendTranslator handles translation of backends to agent gateway resources
type AgentGatewayBackendTranslator struct {
	ContributedBackends map[schema.GroupKind]ir.BackendInit
	ContributedPolicies map[schema.GroupKind]extensionsplug.PolicyPlugin
}

// NewAgentGatewayBackendTranslator creates a new AgentGatewayBackendTranslator
func NewAgentGatewayBackendTranslator(extensions extensionsplug.Plugin) *AgentGatewayBackendTranslator {
	translator := &AgentGatewayBackendTranslator{
		ContributedBackends: make(map[schema.GroupKind]ir.BackendInit),
		ContributedPolicies: extensions.ContributesPolicies,
	}
	for k, up := range extensions.ContributesBackends {
		translator.ContributedBackends[k] = up.BackendInit
	}
	return translator
}

// TranslateBackend converts a BackendObjectIR to agent gateway Backend and Policy resources
func (t *AgentGatewayBackendTranslator) TranslateBackend(
	ctx krt.HandlerContext,
	backend *v1alpha1.Backend,
	svcCol krt.Collection[*corev1.Service],
	secretsCol krt.Collection[*corev1.Secret],
	nsCol krt.Collection[*corev1.Namespace],
) ([]*api.Backend, []*api.Policy, error) {
	backendIr := agentgatewaybackend.BuildAgentGatewayBackendIr(ctx, secretsCol, svcCol, nsCol, backend)
	switch backend.Spec.Type {
	case v1alpha1.BackendTypeStatic:
		return agentgatewaybackend.ProcessStaticBackendForAgentGateway(backendIr)
	case v1alpha1.BackendTypeAI:
		return agentgatewaybackend.ProcessAIBackendForAgentGateway(backendIr)
	case v1alpha1.BackendTypeMCP:
		return agentgatewaybackend.ProcessMCPBackendForAgentGateway(backendIr)
	default:
		return nil, nil, fmt.Errorf("backend of type %s is not supported for agent gateway", backend.Spec.Type)
	}
}
