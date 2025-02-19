package kubeutils

const (
	// control plane resource names
	// TODO if we make the names configurable we should stop using these constants (https://github.com/kgateway-dev/kgateway/issues/10658)
	KgatewayDeploymentName = "kgateway"
	KgatewayServiceName    = "kgateway"
	KgatewayContainerName  = "kgateway"

	// XdsPortName is the name of the port in the kgateway control plane Kubernetes Service that serves xDS config.
	// See: install/helm/kgateway/templates/service.yaml
	XdsPortName = "grpc-xds"
)
