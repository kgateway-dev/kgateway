package translator

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

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
