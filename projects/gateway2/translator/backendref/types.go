package backendref

import (
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	Service = "Service"
)

// RefIsService checks if the BackendObjectReference is a service
// Note: Kind defaults to "Service" when not specified and BackendRef Group defaults to core API group when not specified.
func RefIsService(ref gwv1.BackendObjectReference) bool {
	return (ref.Kind == nil || *ref.Kind == Service) && (ref.Group == nil || *ref.Group == corev1.GroupName)
}
