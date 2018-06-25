package translator

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
)

// for future-proofing possible safety issues with bad upstream names
func clusterName(upstreamName string) string {
	return upstreamName
}

// for future-proofing possible safety issues with bad virtualservice names
func virtualHostName(virtualServiceName string) string {
	return virtualServiceName
}

func routeConfigName(listener *v1.Listener) string {
	return listener.Name+"-routes"
}