package translator

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	agwbackend "github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/backend"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// AgwBackendTranslator handles translation of backends to agent gateway resources
type AgwBackendTranslator struct {
	ContributedBackends map[schema.GroupKind]ir.BackendInit
	ContributedPolicies map[schema.GroupKind]sdk.PolicyPlugin
}

// NewAgwBackendTranslator creates a new AgwBackendTranslator
func NewAgwBackendTranslator(extensions sdk.Plugin) *AgwBackendTranslator {
	translator := &AgwBackendTranslator{
		ContributedBackends: make(map[schema.GroupKind]ir.BackendInit),
		ContributedPolicies: extensions.ContributesPolicies,
	}
	for k, up := range extensions.ContributesBackends {
		translator.ContributedBackends[k] = up.BackendInit
	}
	return translator
}

// TranslateBackend converts a BackendObjectIR to agent gateway Backend and Policy resources
func (t *AgwBackendTranslator) TranslateBackend(
	ctx krt.HandlerContext,
	backend *v1alpha1.Backend,
	svcCol krt.Collection[*corev1.Service],
	secretsCol krt.Collection[*corev1.Secret],
	nsCol krt.Collection[*corev1.Namespace],
) ([]*api.Backend, []*api.Policy, error) {
	backendIr := agwbackend.BuildAgwBackendIr(ctx, secretsCol, svcCol, nsCol, backend)
	switch backend.Spec.Type {
	case v1alpha1.BackendTypeStatic:
		return agwbackend.ProcessStaticBackendForAgw(backendIr)
	case v1alpha1.BackendTypeAI:
		return agwbackend.ProcessAIBackendForAgw(backendIr)
	case v1alpha1.BackendTypeMCP:
		return agwbackend.ProcessMCPBackendForAgw(backendIr)
	default:
		return nil, nil, fmt.Errorf("backend of type %s is not supported for agent gateway", backend.Spec.Type)
	}
}
