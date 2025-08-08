package translator

import (
	"errors"
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

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
	backend *ir.BackendObjectIR,
	svcCol krt.Collection[*corev1.Service],
	secretsCol krt.Collection[*corev1.Secret],
	nsCol krt.Collection[*corev1.Namespace],
) ([]*api.Backend, []*api.Policy, error) {
	gk := schema.GroupKind{
		Group: backend.Group,
		Kind:  backend.Kind,
	}
	process, ok := t.ContributedBackends[gk]
	if !ok {
		return nil, nil, errors.New("no backend translator found for " + gk.String())
	}
	if process.InitAgentBackend == nil {
		return nil, nil, errors.New("no agent gateway backend plugin found for " + gk.String())
	}
	if backend.Errors != nil {
		return nil, nil, fmt.Errorf("backend has errors: %w", errors.Join(backend.Errors...))
	}

	backends, policies, err := process.InitAgentBackend(*backend)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize agent backend: %w", err)
	}

	for _, agentBackend := range backends {
		err := t.runBackendPolicies(ctx, backend, agentBackend)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to process backend policies: %w", err)
		}
	}

	return backends, policies, nil
}

// runBackendPolicies applies backend policies to the translated backend
func (t *AgentGatewayBackendTranslator) runBackendPolicies(
	ctx krt.HandlerContext,
	backend *ir.BackendObjectIR,
	agentBackend *api.Backend,
) error {
	var errs []error
	for gk, policyPlugin := range t.ContributedPolicies {
		if policyPlugin.ProcessAgentBackend == nil {
			continue
		}
		for _, polAttachment := range backend.AttachedPolicies.Policies[gk] {
			if len(polAttachment.Errors) > 0 {
				errs = append(errs, polAttachment.Errors...)
				continue
			}
			err := policyPlugin.ProcessAgentBackend(polAttachment.PolicyIr, *backend)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
