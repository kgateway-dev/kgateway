package pluginutils

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
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
func GetGatewayExtension(
	gwExts krt.Collection[ir.GatewayExtension],
	kctx krt.HandlerContext,
	extensionName string,
	ns string,
) (*ir.GatewayExtension, error) {
	gwExtKey := ir.ObjectSource{
		Group:     wellknown.GatewayExtensionGVK.GroupKind().Group,
		Kind:      wellknown.GatewayExtensionGVK.GroupKind().Kind,
		Name:      extensionName,
		Namespace: ns,
	}
	gwExt := krt.FetchOne(kctx, gwExts, krt.FilterKey(gwExtKey.ResourceName()))
	if gwExt == nil {
		return nil, fmt.Errorf("failed to find GatewayExtension %s", fmt.Sprintf("%s/%s", ns, extensionName))
	}
	return gwExt, nil
}
