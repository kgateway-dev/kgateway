package crds

import (
	_ "embed"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/solo-io/gloo/projects/gateway2/wellknown"
)

const (
	// GatewayClass is the name of the GatewayClass CRD.
	GatewayClass = "gatewayclasses.gateway.networking.k8s.io"
	// Gateway is the name of the Gateway CRD.
	Gateway = "gateways.gateway.networking.k8s.io"
	// HTTPRoute is the name of the HTTPRoute CRD.
	HTTPRoute = "httproutes.gateway.networking.k8s.io"
	// ReferenceGrant is the name of the ReferenceGrant CRD.
	ReferenceGrant = "referencegrants.gateway.networking.k8s.io"
)

//go:embed gateway-crds.yaml
var GatewayCrds []byte

// Required is a list of required Gateway API CRDs.
var Required = []string{GatewayClass, Gateway, HTTPRoute, ReferenceGrant}

// IsSupportedVersion checks if the CRD version is recognized and supported.
func IsSupportedVersion(version string) bool {
	supportedVersions := sets.NewString(wellknown.SupportedVersions...)
	return supportedVersions.Has(version)
}

// IsSupported checks if the CRD is supported based on the provided name.
func IsSupported(name string) bool {
	return name == GatewayClass ||
		name == Gateway ||
		name == HTTPRoute ||
		name == ReferenceGrant
}
