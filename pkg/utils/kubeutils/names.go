package kubeutils

const (
	GlooDeploymentName    = "kgateway"
	GlooServiceName       = "kgateway"
	KgatewayContainerName = "kgateway"

	// GlooXdsPortName is the name of the port in the Gloo Gateway control plane Kubernetes Service that serves xDS config.
	// See: install/helm/gloo/templates/2-gloo-service.yaml
	GlooXdsPortName = "grpc-xds"

	DiscoveryDeploymentName = "discovery"
)
