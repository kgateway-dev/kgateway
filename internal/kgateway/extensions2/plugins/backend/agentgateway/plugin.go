// Package agentgatewaybackend contains agent gateway specific backend processing logic,
// separated from the main envoy backend plugin implementation in the backend/plugin.go file.
package agentgatewaybackend

import (
	"github.com/agentgateway/agentgateway/go/api"

	agwir "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var logger = logging.New("plugin/backend/agentgateway_backend")

// agentGatewayBackendPlugin implements agent gateway specific backend processing
type agentGatewayBackendPlugin struct {
	agwir.UnimplementedAgentGatewayTranslationPass
}

var _ agwir.AgentGatewayTranslationPass = &agentGatewayBackendPlugin{}

// NewAgentGatewayPlug creates a new agent gateway translation pass
func NewAgentGatewayPlug(reporter reports.Reporter) agwir.AgentGatewayTranslationPass {
	return &agentGatewayBackendPlugin{}
}

// ApplyForBackend processes backend configuration for agent gateway
func (p *agentGatewayBackendPlugin) ApplyForBackend(pCtx *agwir.AgentGatewayTranslationBackendContext, out *api.Backend) error {
	logger.Debug("agent gateway backend plugin processed backend (not implemented)", "backend", out.Name)
	return nil
}
