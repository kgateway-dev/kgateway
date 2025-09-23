package translator

import (
	agwplugins "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

// AgwTranslator coordinates translation of resources for agent gateway
type AgwTranslator struct {
	agwCollection     *agwplugins.AgwCollections
	extensions        sdk.Plugin
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
