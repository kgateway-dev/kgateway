package translator

import (
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	agentgatewayplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
)

// AgentGatewayTranslator coordinates translation of resources for agent gateway
type AgentGatewayTranslator struct {
	agwCollection     *agentgatewayplugins.AgwCollections
	extensions        extensionsplug.Plugin
	backendTranslator *AgentGatewayBackendTranslator
}

// NewAgentGatewayTranslator creates a new AgentGatewayTranslator
func NewAgentGatewayTranslator(
	agwCollection *agentgatewayplugins.AgwCollections,
	extensions extensionsplug.Plugin,
) *AgentGatewayTranslator {
	return &AgentGatewayTranslator{
		agwCollection: agwCollection,
		extensions:    extensions,
	}
}

// Init initializes the translator components
func (s *AgentGatewayTranslator) Init() {
	s.backendTranslator = NewAgentGatewayBackendTranslator(s.extensions)
}

// BackendTranslator returns the initialized backend translator on the AgentGatewayTranslator receiver
func (s *AgentGatewayTranslator) BackendTranslator() *AgentGatewayBackendTranslator {
	return s.backendTranslator
}
