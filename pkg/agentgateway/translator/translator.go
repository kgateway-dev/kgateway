package translator

import (
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	agentgatewayplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
)

// AgentgatewayTranslator coordinates translation of resources for agent gateway
type AgentgatewayTranslator struct {
	agwCollection     *agentgatewayplugins.AgwCollections
	extensions        extensionsplug.Plugin
	backendTranslator *AgentgatewayBackendTranslator
}

// NewAgentgatewayTranslator creates a new AgentgatewayTranslator
func NewAgentgatewayTranslator(
	agwCollection *agentgatewayplugins.AgwCollections,
) *AgentgatewayTranslator {
	return &AgentgatewayTranslator{
		agwCollection: agwCollection,
	}
}

// Init initializes the translator components
func (s *AgentgatewayTranslator) Init() {
	s.backendTranslator = NewAgentgatewayBackendTranslator(s.extensions)
}

// BackendTranslator returns the initialized backend translator on the AgentgatewayTranslator receiver
func (s *AgentgatewayTranslator) BackendTranslator() *AgentgatewayBackendTranslator {
	return s.backendTranslator
}
