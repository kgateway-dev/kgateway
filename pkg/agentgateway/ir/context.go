package ir

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// AgentGatewayRouteContext provides context for route-level translations
type AgentGatewayRouteContext struct {
	Rule *gwv1.HTTPRouteRule
}

// AgentGatewayTranslationBackendContext provides context for backend translations
type AgentGatewayTranslationBackendContext struct {
	Backend        *ir.BackendObjectIR
	GatewayContext ir.GatewayContext
}
