package translator

import (
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
)

// AgentGatewayTranslator coordinates translation of resources for agent gateway
type AgentGatewayTranslator struct {
	commonCols        *common.CommonCollections
	extensions        extensionsplug.Plugin
	backendTranslator *AgentGatewayBackendTranslator
}

// NewAgentGatewayTranslator creates a new AgentGatewayTranslator
func NewAgentGatewayTranslator(
	commonCols *common.CommonCollections,
	extensions extensionsplug.Plugin,
) *AgentGatewayTranslator {
	return &AgentGatewayTranslator{
		commonCols: commonCols,
		extensions: extensions,
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
