package backendref

import (
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// RefIsService checks if the BackendObjectReference is a service
// Note: Kind defaults to "Service" when not specified and BackendRef Group defaults to core API group when not specified.
func RefIsService(ref gwv1.BackendObjectReference) bool {
	return (ref.Kind == nil || *ref.Kind == wellknown.ServiceKind) && (ref.Group == nil || *ref.Group == corev1.GroupName)
}

// RefIsHTTPRoute checks if the BackendObjectReference is an HTTPRoute
// Parent routes may delegate to child routes using an HTTPRoute backend reference.
func RefIsHTTPRoute(ref gwv1.BackendObjectReference) bool {
	return (ref.Kind != nil && *ref.Kind == wellknown.HTTPRouteKind) && (ref.Group != nil && *ref.Group == gwv1.GroupName)
}
