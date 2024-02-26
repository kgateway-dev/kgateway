package wellknown

const (
	// GatewayClassName represents the name of the GatewayClass to watch for
	GatewayClassName = "gloo-gateway"

	// GatewayControllerName is the name of the controller that has implemented the Gateway API
	// It is configured to manage GatewayClasses with the name GatewayClassName
	GatewayControllerName = "solo.io/gloo-gateway"
)
