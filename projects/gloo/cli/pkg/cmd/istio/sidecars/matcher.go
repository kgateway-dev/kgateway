package sidecars

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
)

// ErrNoSupportedSidecar occurs when we can't find any premade sidecar for the given Istio version
var ErrNoSupportedSidecar = errors.New("no valid istio sidecar found for this istio version")

// GetIstioSidecar will return an Istio sidecar for the given
// version of Istio, with the given jwtPolicy, to run
// in the gateway-proxy pod
func GetIstioSidecar(istioVersion, jwtPolicy string, istioMetaMeshID string, istioMetaClusterID string, istioDiscoveryAddress string) (*corev1.Container, error) {
	return generateIstioSidecar(istioVersion, jwtPolicy, istioMetaMeshID, istioMetaClusterID, istioDiscoveryAddress), nil
}
