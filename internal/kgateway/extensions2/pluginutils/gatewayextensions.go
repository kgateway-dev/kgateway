package pluginutils

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
)

// ExtensionTypeError is an error for when an extension type is mismatched
type ExtensionTypeError struct {
	expected v1alpha1.GatewayExtensionType
	actual   v1alpha1.GatewayExtensionType
}

var _ error = &ExtensionTypeError{}

// Error implements error.
func (e *ExtensionTypeError) Error() string {
	return fmt.Sprintf("expected gatewayextension type %v, got %v", e.expected, e.actual)
}

// ErrInvalidExtensionType is an error for when an extension type is invalid.
func ErrInvalidExtensionType(expected, actual v1alpha1.GatewayExtensionType) error {
	return &ExtensionTypeError{expected: expected, actual: actual}
}

// GetGatewayExtension retrieves a GatewayExtension resource by name and namespace.
// It returns the extension and any error encountered during retrieval.
func GetGatewayExtension(extensions *krtcollections.GatewayExtensionIndex, krtctx krt.HandlerContext, extensionName, ns string) (*ir.GatewayExtension, error) {
	extension, err := extensions.GetGatewayExtensionFromRef(krtctx, types.NamespacedName{Name: extensionName, Namespace: ns})
	if err != nil {
		return nil, fmt.Errorf("failed to find secret %s: %v", extensionName, err)
	}
	return extension, nil
}
