package deployer

// TODO(tim): Consolidate with the other wellknown packages?
const (
	// KgatewayContainerName is the name of the container in the proxy deployment.
	KgatewayContainerName = "kgateway-proxy"
	// IstioContainerName is the name of the container in the proxy deployment for the Istio integration.
	IstioContainerName = "istio-proxy"
	// IstioWaypointPort - Port 15008 is reserved for Istio. This port enables sidecars to include waypoint proxies
	// in the list of possible communication targets. There is no actual traffic on this port.
	IstioWaypointPort = 15008
	// EnvoyWrapperImage is the image of the envoy wrapper container.
	EnvoyWrapperImage = "envoy-wrapper"
	// AgentgatewayImage is the agentgateway image repository
	AgentgatewayImage = "agentgateway"
	// AgentgatewayRegistry is the agentgateway registry
	AgentgatewayRegistry = "ghcr.io/agentgateway"
	// AgentgatewayDefaultTag is the default agentgateway image tag
	// Note: should be in sync with version in go.mod and test/deployer/testdata/*
	AgentgatewayDefaultTag = "0.11.0-alpha.2f71d0e845d0105037cbb5524262a5f1c1bd94a9"
	// SdsImage is the image of the sds container.
	SdsImage = "sds"
	// SdsContainerName is the name of the container in the proxy deployment for the SDS integration.
	SdsContainerName = "sds"
)
