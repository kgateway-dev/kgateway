package translator

import (
	"github.com/agentgateway/agentgateway/go/api"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	agwbackend "github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/backend"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
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
	ctx plugins.PolicyCtx,
	backend *agentgateway.AgentgatewayBackend,
) ([]*api.Backend, error) {
	return agwbackend.BuildAgwBackend(ctx, backend)
}
