package agentgatewaybackend

import (
	"errors"
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
)

const (
	// mcpProtocol specifies that streamable HTTP protocol is to be used for the MCP target
	mcpProtocol = "kgateway.dev/mcp"

	// mcpProtocolSSE specifies that Server-Sent Events (SSE) protocol is to be used for the MCP target
	mcpProtocolSSE = "kgateway.dev/mcp-sse"
)

// ProcessMCPBackendForAgw processes MCP backend using pre-resolved IR data
func ProcessMCPBackendForAgw(be *AgwBackendIr) ([]*api.Backend, []*api.Policy, error) {
	if len(be.Errors) > 0 {
		return nil, nil, fmt.Errorf("errors occurred while processing mcp backend for agent gateway: %w", errors.Join(be.Errors...))
	}
	if be.MCPIr == nil {
		return nil, nil, fmt.Errorf("mcp backend IR must not be nil for MCP backend type")
	}

	// TODO: add support for backend auth policy for mcp
	return be.MCPIr.Backends, nil, nil
}
