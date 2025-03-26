package ir

import (
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

// GatewayExtension represents the internal representation of a GatewayExtension.
type GatewayExtension struct {
	// ObjectSource identifies the source of this extension.
	ObjectSource

	// Type indicates the type of the GatewayPolicy.
	Type v1alpha1.GatewayExtensionType

	// Placement configuration for where this extension should be placed in the filter chain.
	// Placement v1alpha1.Placement

	// ExtAuth configuration for ExtAuth extension type.
	ExtAuth *v1alpha1.ExtAuthProvider

	// ExtProc configuration for ExtProc extension type.
	ExtProc *v1alpha1.ExtProcProvider
}

// ResourceName returns the unique name for this extension.
func (e *GatewayExtension) ResourceName() string {
	return e.ObjectSource.ResourceName()
}

// GVK returns the GroupVersionKind for this extension.
func (e *GatewayExtension) GVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "gateway.kgateway.dev",
		Version: "v1alpha1",
		Kind:    "GatewayExtension",
	}
}

func (e *GatewayExtension) Equals(
	other *GatewayExtension,
) bool {
	if e.Type != other.Type {
		return false
	}
	// if e.Placement != other.Placement {
	// 	return false
	// }
	if !reflect.DeepEqual(e.ExtAuth, other.ExtAuth) {
		return false
	}
	return true
}
