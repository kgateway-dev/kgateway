package agentgatewaysyncer

const (
	TargetTypeA2AUrl      = "type.googleapis.com/agentgateway.dev.a2a.target.Target"
	TargetTypeMcpUrl      = "type.googleapis.com/agentgateway.dev.mcp.target.Target"
	TargetTypeListenerUrl = "type.googleapis.com/agentgateway.dev.listener.Listener"

	MCPProtocol = "kgateway.dev/mcp"
	A2AProtocol = "kgateway.dev/a2a"

	MCPPathAnnotation = "kgateway.dev/mcp-path"
	A2APathAnnotation = "kgateway.dev/a2a-path"

	// Needs to match agentgateway role configured here: https://github.com/agentgateway/agentgateway/blob/main/crates/agentgateway/src/xds/client.rs#L293
	OwnerNodeId = "agentgateway-api"
)
