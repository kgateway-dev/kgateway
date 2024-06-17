package kubegatewayutils

import (
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// Returns true if Kubernetes Gateway API CRDs are on the cluster.
// Note: this doesn't check for specific CRD names; it returns true if *any* k8s Gateway CRD is detected
func DetectKubeGatewayCrds(cfg *rest.Config) (bool, error) {
	discClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return false, err
	}

	groups, err := discClient.ServerGroups()
	if err != nil {
		return false, err
	}

	// Check if gateway group exists
	for _, group := range groups.Groups {
		if group.Name == "gateway.networking.k8s.io" {
			return true, nil
		}
	}

	return false, nil
}
