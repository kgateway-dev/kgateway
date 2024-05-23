package sidecars

import (
	"errors"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// ErrNoSupportedSidecar occurs when we can't find any premade sidecar for the given Istio version
var ErrNoSupportedSidecar = errors.New("no valid istio sidecar found for this istio version")

// GetIstioSidecar will return an Istio sidecar for the given
// version of Istio, with the given jwtPolicy, to run
// in the gateway-proxy pod
func GetIstioSidecar(istioVersion, jwtPolicy string, istioMetaMeshID string, istioMetaClusterID string, istioDiscoveryAddress string) (*corev1.Container, error) {

	istio1dot7to1dotX, _ := regexp.MatchString("1.([7-9]|[1-9][0-9]+).[0-9]*", istioVersion)

	if istio1dot7to1dotX {
		return generateIstio17to1xSidecar(istioVersion, jwtPolicy, istioMetaMeshID, istioMetaClusterID, istioDiscoveryAddress), nil
	} else if strings.HasPrefix(istioVersion, "1.6.") {
		return generateIstio16Sidecar(istioVersion, jwtPolicy, istioMetaMeshID, istioMetaClusterID, istioDiscoveryAddress), nil
	}
	return nil, ErrNoSupportedSidecar
}
