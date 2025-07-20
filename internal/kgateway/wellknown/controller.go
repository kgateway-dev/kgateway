package wellknown

const (
	// DefaultGatewayClassName represents the name of the GatewayClass to watch for
	DefaultGatewayClassName = "kgateway"

	// DefaultWaypointClassName is the GatewayClass name for the waypoint.
	DefaultWaypointClassName = "kgateway-waypoint"

	// DefaultAgentGatewayClassName is the GatewayClass name for the agentgateway proxy.
	DefaultAgentGatewayClassName = "agentgateway"

	// DefaultGatewayControllerName is the name of the controller that has implemented the Gateway API
	// It is configured to manage GatewayClasses with the name DefaultGatewayClassName
	DefaultGatewayControllerName = "kgateway.dev/kgateway"

	// DefaultGatewayParametersName is the name of the GatewayParameters which is attached by
	// parametersRef to the GatewayClass.
	DefaultGatewayParametersName = "kgateway"

	// InferencePoolFinalizer is the InferencePool finalizer name to ensure cluster-scoped
	// objects are cleaned up.
	InferencePoolFinalizer = "kgateway/inferencepool-cleanup"

	// GatewayNameLabel is a label on GW pods to indicate the name of the gateway
	// they are associated with.
	GatewayNameLabel = "gateway.networking.k8s.io/gateway-name"
)
