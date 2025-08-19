package plugins

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

// CreateDefaultPolicyManager creates a new policy manager with all default plugins registered
func CreateDefaultPolicyManager() *DefaultPolicyManager {
	manager := NewPolicyManager()

	// Register all built-in plugins
	RegisterBuiltinPlugins(manager)

	return manager
}

// RegisterBuiltinPlugins registers all built-in policy plugins
func RegisterBuiltinPlugins(manager PolicyManager) error {
	// Create plugins directly to avoid import cycle
	plugins := []PolicyPlugin{
		NewTrafficPlugin(),
		NewInferencePlugin(),
		NewA2APlugin(),
	}

	registryLogger := logging.New("agentgateway/plugins/registry")
	var allErrors []error
	for _, plugin := range plugins {
		if err := manager.RegisterPlugin(plugin); err != nil {
			registryLogger.Error("failed to register plugin", "plugin", plugin.Name(), "error", err)
			allErrors = append(allErrors, err)
		}
	}

	if len(allErrors) > 0 {
		registryLogger.Error("some plugins failed to register", "count", len(allErrors))
		// Don't fail completely, just log errors
	}

	registryLogger.Info("registered builtin plugins", "count", len(plugins), "errors", len(allErrors))
	return nil
}

// GetDefaultPlugins returns a list of all default policy plugins without registering them
func GetDefaultPlugins() []PolicyPlugin {
	return []PolicyPlugin{
		NewTrafficPlugin(),
		NewInferencePlugin(),
		NewA2APlugin(),
	}
}
