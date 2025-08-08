package ir

import (
	"github.com/agentgateway/agentgateway/go/api"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// AgentGatewayTranslationPass defines the interface for agent gateway translation passes
type AgentGatewayTranslationPass interface {
	// ApplyForRoute processes route-level configuration
	ApplyForRoute(pCtx *AgentGatewayRouteContext, out *api.Route) error

	// ApplyForBackend processes backend-level configuration for each backend referenced in routes
	ApplyForBackend(pCtx *AgentGatewayTranslationBackendContext, out *api.Backend) error

	// ApplyForRouteBackend processes route-specific backend configuration
	ApplyForRouteBackend(policy ir.PolicyIR, pCtx *AgentGatewayTranslationBackendContext) error
}

// UnimplementedAgentGatewayTranslationPass provides default implementations for AgentGatewayTranslationPass
type UnimplementedAgentGatewayTranslationPass struct{}

var _ AgentGatewayTranslationPass = UnimplementedAgentGatewayTranslationPass{}

func (s UnimplementedAgentGatewayTranslationPass) ApplyForRoute(pCtx *AgentGatewayRouteContext, out *api.Route) error {
	return nil
}

func (s UnimplementedAgentGatewayTranslationPass) ApplyForBackend(pCtx *AgentGatewayTranslationBackendContext, out *api.Backend) error {
	return nil
}

func (s UnimplementedAgentGatewayTranslationPass) ApplyForRouteBackend(policy ir.PolicyIR, pCtx *AgentGatewayTranslationBackendContext) error {
	return nil
}
