package agentgatewaysyncer

const (
	TargetTypeA2AUrl      = "type.googleapis.com/agentgateway.dev.a2a.target.Target"
	TargetTypeMcpUrl      = "type.googleapis.com/agentgateway.dev.mcp.target.Target"
	TargetTypeListenerUrl = "type.googleapis.com/agentgateway.dev.listener.Listener"

	MCPProtocol = "kgateway.dev/mcp"
	A2AProtocol = "kgateway.dev/a2a"

	MCPPathAnnotation = "kgateway.dev/mcp-path"
	A2APathAnnotation = "kgateway.dev/a2a-path"

	// TODO: Agent Gateway currently uses this node ID. Should change it to be configurable.
	// https://github.com/agentgateway/agentgateway/blob/a553ae20c786787371621fe7c6e8964e65f3f2c8/crates/agentgateway/src/xds/client.rs#L294
	OwnerNodeId = "mcp-kgateway-kube-gateway-api"
)
