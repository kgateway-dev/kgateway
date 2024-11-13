package sidecars

import (
	corev1 "k8s.io/api/core/v1"
)

// GetIstioSidecar will return an Istio sidecar for the given
// version of Istio, with the given jwtPolicy, to run
// in the gateway-proxy pod
func GetIstioSidecar(istioVersion, jwtPolicy, istioMetaMeshID, istioMetaClusterID, istioDiscoveryAddress string) (*corev1.Container, error) {
	return generateIstioSidecar(istioVersion, jwtPolicy, istioMetaMeshID, istioMetaClusterID, istioDiscoveryAddress), nil
}
