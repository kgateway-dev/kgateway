package plugins

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

// DefaultPolicyManager implements the PolicyManager interface
type DefaultPolicyManager struct {
	contributesPolicies ContributesPolicies
}

// NewPolicyManager creates a new DefaultPolicyManager
func NewPolicyManager() *DefaultPolicyManager {
	return &DefaultPolicyManager{
		contributesPolicies: make(ContributesPolicies),
	}
}

// RegisterPlugin registers a policy plugin
func (m *DefaultPolicyManager) RegisterPlugin(plugin PolicyPlugin) error {
	if plugin == nil {
		return fmt.Errorf("cannot register nil plugin")
	}

	groupKind := plugin.GroupKind()
	managerLogger := logging.New("agentgateway/plugins/manager")
	managerLogger.Info("registering policy plugin", "name", plugin.Name(), "groupKind", groupKind.String())

	// Check for duplicate GroupKind registration
	if existing, exists := m.contributesPolicies[groupKind]; exists {
		return fmt.Errorf("plugin for GroupKind %s is already registered: %s", groupKind.String(), existing.Name())
	}

	// Add to the contributions policies map
	m.contributesPolicies[groupKind] = plugin

	return nil
}

// GetPluginByGroupKind returns the plugin for a specific GroupKind
func (m *DefaultPolicyManager) GetPluginByGroupKind(gk schema.GroupKind) (PolicyPlugin, bool) {
	plugin, exists := m.contributesPolicies[gk]
	return plugin, exists
}

// GetContributesPolicies returns the map of all registered plugins
func (m *DefaultPolicyManager) GetContributesPolicies() ContributesPolicies {
	// Return a copy to prevent external modification
	result := make(ContributesPolicies)
	for gk, plugin := range m.contributesPolicies {
		result[gk] = plugin
	}
	return result
}

// GenerateAllPolicies generates policies from all registered ADP plugins
func (m *DefaultPolicyManager) GenerateAllPolicies(ctx krt.HandlerContext, agw *AgwCollections) ([]ADPPolicy, error) {
	var allPolicies []ADPPolicy
	var allErrors []error

	for groupKind, plugin := range m.contributesPolicies {
		managerLogger := logging.New("agentgateway/plugins/manager")
		managerLogger.Debug("generating policies", "plugin", plugin.Name(), "groupKind", groupKind.String())

		policies, err := plugin.GeneratePolicies(ctx, agw)
		if err != nil {
			managerLogger.Error("failed to generate policies", "plugin", plugin.Name(), "groupKind", groupKind.String(), "error", err)
			allErrors = append(allErrors, fmt.Errorf("plugin %s (GroupKind: %s) failed: %w", plugin.Name(), groupKind.String(), err))
			continue
		}

		allPolicies = append(allPolicies, policies...)
		managerLogger.Debug("generated policies", "plugin", plugin.Name(), "groupKind", groupKind.String(), "count", len(policies))
	}

	managerLogger := logging.New("agentgateway/plugins/manager")
	if len(allErrors) > 0 {
		// Log errors but don't fail completely - return partial results
		for _, err := range allErrors {
			managerLogger.Error("policy generation error", "error", err)
		}
	}

	managerLogger.Info("generated all policies", "total_policies", len(allPolicies), "errors", len(allErrors))
	return allPolicies, nil
}
