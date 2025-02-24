package kubeutils

import (
	"os"
)

const (
	// default names for the control plane deployment and service; these can be overridden via helm values
	defaultKgatewayDeploymentName = "kgateway"
	defaultKgatewayServiceName    = "kgateway"

	// container within the kgateway pod; its name is currently hardcoded
	KgatewayContainerName = "kgateway"

	// xdsPortName is the name of the port in the kgateway control plane Kubernetes Service that serves xDS config.
	// See: install/helm/kgateway/templates/service.yaml
	xdsPortName = "grpc-xds"
)

// GetKgatewayDeploymentName gets the control plane deployment name.
// Note that this is currently the same as the Service name.
func GetKgatewayDeploymentName() string {
	return GetKgatewayServiceName()
}

// GetKgatewayServiceName gets the control plane service name.
func GetKgatewayServiceName() string {
	if name := os.Getenv("SERVICE_NAME"); name != "" {
		return name
	}
	return defaultKgatewayServiceName
}
