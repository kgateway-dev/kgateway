package ir

import (
	"reflect"

	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

// GatewayExtension represents the internal representation of a GatewayExtension.
type GatewayExtension struct {
	// ObjectSource identifies the source of this extension.
	ObjectSource

	// ExtAuth configuration for ExtAuth extension type.
	ExtAuth *v1alpha1.ExtAuthProvider

	// ExtProc configuration for ExtProc extension type.
	ExtProc *v1alpha1.ExtProcProvider

	// RateLimit configuration for RateLimit extension type.
	// This is specifically for global rate limiting that communicates with an external rate limit service.
	RateLimit *v1alpha1.RateLimitProvider

	// JwtProviders configures the jwt providers
	JwtProviders []v1alpha1.NamedJWTProvider

	// PrecedenceWeight specifies the precedence weight associated with the provider.
	// A higher weight implies higher priority.
	// It is used to order provider filters by their weight.
	PrecedenceWeight int32
}

var (
	_ krt.ResourceNamer             = GatewayExtension{}
	_ krt.Equaler[GatewayExtension] = GatewayExtension{}
)

// ResourceName returns the unique name for this extension.
func (e GatewayExtension) ResourceName() string {
	return e.ObjectSource.ResourceName()
}

func (e GatewayExtension) Equals(other GatewayExtension) bool {
	if !reflect.DeepEqual(e.ExtAuth, other.ExtAuth) {
		return false
	}
	if !reflect.DeepEqual(e.ExtProc, other.ExtProc) {
		return false
	}
	if !reflect.DeepEqual(e.RateLimit, other.RateLimit) {
		return false
	}
	if !reflect.DeepEqual(e.JwtProviders, other.JwtProviders) {
		return false
	}
	if e.PrecedenceWeight != other.PrecedenceWeight {
		return false
	}
	return true
}
