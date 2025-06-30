package deployer

// TODO(tim): Consolidate with the other wellknown packages?
const (
	// KgatewayContainerName is the name of the container in the proxy deployment.
	KgatewayContainerName = "kgateway-proxy"
	// KgatewayAIContainerName is the name of the container in the proxy deployment for the AI extension.
	KgatewayAIContainerName = "kgateway-ai-extension"
	// IstioContainerName is the name of the container in the proxy deployment for the Istio integration.
	IstioContainerName = "istio-proxy"
	// EnvoyWrapperImage is the image of the envoy wrapper container.
	EnvoyWrapperImage = "envoy-wrapper"
	// AgentgatewayImage is the agentgateway image repository
	AgentgatewayImage = "agentgateway"
	// AgentgatewayRegistry is the agentgateway registry
	AgentgatewayRegistry = "ghcr.io/agentgateway"
	// AgentgatewayDefaultTag is the default agentgateway image tag
	AgentgatewayDefaultTag = "0.5.2"
	// SdsImage is the image of the sds container.
	SdsImage = "sds"
	// SdsContainerName is the name of the container in the proxy deployment for the SDS integration.
	SdsContainerName = "sds"
)
