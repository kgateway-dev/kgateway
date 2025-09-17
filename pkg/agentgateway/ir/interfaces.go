package ir

import (
	"github.com/agentgateway/agentgateway/go/api"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// AgentgatewayTranslationPass defines the interface for agent gateway translation passes
type AgentgatewayTranslationPass interface {
	// ApplyForRoute processes route-level configuration
	ApplyForRoute(pCtx *AgentgatewayRouteContext, out *api.Route) error

	// ApplyForBackend processes backend-level configuration for each backend referenced in routes
	ApplyForBackend(pCtx *AgentgatewayTranslationBackendContext, out *api.Backend) error

	// ApplyForRouteBackend processes route-specific backend configuration
	ApplyForRouteBackend(policy ir.PolicyIR, pCtx *AgentgatewayTranslationBackendContext) error
}

// UnimplementedAgentgatewayTranslationPass provides default implementations for AgentgatewayTranslationPass
type UnimplementedAgentgatewayTranslationPass struct{}

var _ AgentgatewayTranslationPass = UnimplementedAgentgatewayTranslationPass{}

func (s UnimplementedAgentgatewayTranslationPass) ApplyForRoute(pCtx *AgentgatewayRouteContext, out *api.Route) error {
	return nil
}

func (s UnimplementedAgentgatewayTranslationPass) ApplyForBackend(pCtx *AgentgatewayTranslationBackendContext, out *api.Backend) error {
	return nil
}

func (s UnimplementedAgentgatewayTranslationPass) ApplyForRouteBackend(policy ir.PolicyIR, pCtx *AgentgatewayTranslationBackendContext) error {
	return nil
}
