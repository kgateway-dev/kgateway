package kubeutils

import (
	"os"
	"strconv"
)

const (
	// The default name of the Kubernetes Service that serves xDS config. This corresponds to the Service name
	// in install/helm/kgateway/templates/service.yaml
	defaultXdsServiceName = "kgateway"
	// The default port of the Kubernetes Service that serves xDS config. This corresponds to the number of the
	// grpc-xds port in install/helm/kgateway/templates/service.yaml
	defaultXdsServicePort = 9977
)

// GetXdsServiceName returns the name of the Kubernetes Service that serves xDS config.
func GetXdsServiceName() string {
	if name := os.Getenv("XDS_SERVICE_NAME"); name != "" {
		return name
	}
	return defaultXdsServiceName
}

// GetXdsServicePort returns the port of the Kubernetes Service that serves xDS config.
func GetXdsServicePort() (uint32, error) {
	if portStr := os.Getenv("XDS_SERVICE_PORT"); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return 0, err
		}
		return uint32(port), nil
	}
	return defaultXdsServicePort, nil
}
