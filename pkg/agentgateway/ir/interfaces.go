package ir

import (
	"github.com/agentgateway/agentgateway/go/api"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// AgwTranslationPass defines the interface for agent gateway translation passes
type AgwTranslationPass interface {
	// ApplyForRoute processes route-level configuration
	ApplyForRoute(pCtx *AgwRouteContext, out *api.Route) error

	// ApplyForBackend processes backend-level configuration for each backend referenced in routes
	ApplyForBackend(pCtx *AgwTranslationBackendContext, out *api.Backend) error

	// ApplyForRouteBackend processes route-specific backend configuration
	ApplyForRouteBackend(policy ir.PolicyIR, pCtx *AgwTranslationBackendContext) error
}

// UnimplementedAgwTranslationPass provides default implementations for AgwTranslationPass
type UnimplementedAgwTranslationPass struct{}

var _ AgwTranslationPass = UnimplementedAgwTranslationPass{}

func (s UnimplementedAgwTranslationPass) ApplyForRoute(pCtx *AgwRouteContext, out *api.Route) error {
	return nil
}

func (s UnimplementedAgwTranslationPass) ApplyForBackend(pCtx *AgwTranslationBackendContext, out *api.Backend) error {
	return nil
}

func (s UnimplementedAgwTranslationPass) ApplyForRouteBackend(policy ir.PolicyIR, pCtx *AgwTranslationBackendContext) error {
	return nil
}
