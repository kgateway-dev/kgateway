package deployer

import apiv1 "sigs.k8s.io/gateway-api/apis/v1"

// GatewayClassInfo describes the desired configuration for a GatewayClass.
type GatewayClassInfo struct {
	// Description is a human-readable description of the GatewayClass.
	Description string
	// Labels are the labels to be added to the GatewayClass.
	Labels map[string]string
	// Annotations are the annotations to be added to the GatewayClass.
	Annotations map[string]string
	// ParametersRef is the reference to the GatewayParameters object.
	ParametersRef *apiv1.ParametersReference
	// ControllerName is the name of the controller that is managing the GatewayClass.
	ControllerName string
}
