package translator

import (
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	agwplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
)

// AgwTranslator coordinates translation of resources for agent gateway
type AgwTranslator struct {
	agwCollection     *agwplugins.AgwCollections
	extensions        extensionsplug.Plugin
	backendTranslator *AgwBackendTranslator
}

// NewAgwTranslator creates a new AgwTranslator
func NewAgwTranslator(
	agwCollection *agwplugins.AgwCollections,
) *AgwTranslator {
	return &AgwTranslator{
		agwCollection: agwCollection,
	}
}

// Init initializes the translator components
func (s *AgwTranslator) Init() {
	s.backendTranslator = NewAgwBackendTranslator(s.extensions)
}

// BackendTranslator returns the initialized backend translator on the AgwTranslator receiver
func (s *AgwTranslator) BackendTranslator() *AgwBackendTranslator {
	return s.backendTranslator
}
