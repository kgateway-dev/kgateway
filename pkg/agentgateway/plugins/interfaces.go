package plugins

import (
	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// PolicyPlugin defines the base interface for all policy plugins
type PolicyPlugin interface {
	// GroupKind returns the GroupKind of the policy this plugin handles
	GroupKind() schema.GroupKind

	// Name returns the name of the plugin
	Name() string

	// GeneratePolicies generates ADP policies for the given common collections
	GeneratePolicies(ctx krt.HandlerContext, agentgatewayCol *AgwCollections) ([]ADPPolicy, error)
}

// ADPPolicy wraps an ADP policy for collection handling
type ADPPolicy struct {
	Policy *api.Policy
}

// ContributesPolicies follows the pattern used in pluginsdk
type ContributesPolicies map[schema.GroupKind]PolicyPlugin

// PolicyManager coordinates all policy plugins
type PolicyManager interface {
	// RegisterPlugin registers a policy plugin by its GroupKind
	RegisterPlugin(plugin PolicyPlugin) error

	// GetPluginByGroupKind returns the plugin for a specific GroupKind
	GetPluginByGroupKind(gk schema.GroupKind) (PolicyPlugin, bool)

	// GetContributesPolicies returns the map of all registered plugins
	GetContributesPolicies() ContributesPolicies

	// GenerateAllPolicies generates policies from all registered ADP plugins
	GenerateAllPolicies(ctx krt.HandlerContext, agw *AgwCollections) ([]ADPPolicy, error)
}
