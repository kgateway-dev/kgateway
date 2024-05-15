package backendref

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// RefIsService checks if the BackendObjectReference is a service
// Note: Kind defaults to "Service" when not specified and BackendRef Group defaults to core API group when not specified.
func RefIsService(ref gwv1.BackendObjectReference) bool {
	return (ref.Kind == nil || *ref.Kind == wellknown.ServiceKind) && (ref.Group == nil || *ref.Group == corev1.GroupName)
}

// RefIsUpstream checks if the BackendObjectReference is an Upstream.
func RefIsUpstream(ref gwv1.BackendObjectReference) bool {
	return (ref.Kind != nil && string(*ref.Kind) == gloov1.UpstreamGVK.Kind) && (ref.Group != nil && *ref.Group == v1.GroupName)
}

// RefIsHTTPRoute checks if the BackendObjectReference is an HTTPRoute
// Parent routes may delegate to child routes using an HTTPRoute backend reference.
func RefIsHTTPRoute(ref gwv1.BackendObjectReference) bool {
	return (ref.Kind != nil && *ref.Kind == wellknown.HTTPRouteKind) && (ref.Group != nil && *ref.Group == gwv1.GroupName)
}

// ToString returns a string representation of the BackendObjectReference
func ToString(ref gwv1.BackendObjectReference) string {
	var group, kind, namespace string
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return fmt.Sprintf("%s.%s %s/%s", kind, group, namespace, ref.Name)
}
