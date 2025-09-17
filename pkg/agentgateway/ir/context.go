package ir

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// AgentgatewayRouteContext provides context for route-level translations
type AgentgatewayRouteContext struct {
	Rule             *gwv1.HTTPRouteRule
	AttachedPolicies ir.AttachedPolicies
}

// AgentgatewayTranslationBackendContext provides context for backend translations
type AgentgatewayTranslationBackendContext struct {
	Backend        *ir.BackendObjectIR
	GatewayContext ir.GatewayContext
}
