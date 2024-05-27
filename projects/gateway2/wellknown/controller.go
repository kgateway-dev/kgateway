package wellknown

const (
	// GatewayClassName represents the name of the GatewayClass to watch for
	GatewayClassName = "gloo-gateway"

	// GatewayControllerName is the name of the controller that has implemented the Gateway API
	// It is configured to manage GatewayClasses with the name GatewayClassName
	GatewayControllerName = "solo.io/gloo-gateway"

	// GatewayParametersAnnotationName is the name of the Gateway annotation that specifies
	// the name of a GatewayParameters CR, which is used to dynamically provision the data plane
	// resources for the Gateway. The GatewayParameters is assumed to be in the same namespace
	// as the Gateway.
	GatewayParametersAnnotationName = "gateway.gloo.solo.io/gateway-parameters-name"

	// DefaultGatewayParametersName is the name of the GatewayParameters which is attached by
	// parametersRef to the GatewayClass.
	DefaultGatewayParametersName = "gloo-gateway"
)
