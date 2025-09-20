package ir

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// AgwRouteContext provides context for route-level translations
type AgwRouteContext struct {
	Rule             *gwv1.HTTPRouteRule
	AttachedPolicies ir.AttachedPolicies
}

// AgwTranslationBackendContext provides context for backend translations
type AgwTranslationBackendContext struct {
	Backend        *ir.BackendObjectIR
	GatewayContext ir.GatewayContext
}
