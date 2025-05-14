package backendref

import (
	"fmt"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apilabels "github.com/kgateway-dev/kgateway/v2/api/labels"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// IsHTTPRoute checks if the BackendObjectReference is an HTTPRoute
// Parent routes may delegate to child routes using an HTTPRoute backend reference.
func IsHTTPRoute(ref gwv1.BackendObjectReference) bool {
	return (ref.Kind != nil && *ref.Kind == wellknown.HTTPRouteKind) && (ref.Group != nil && *ref.Group == gwv1.GroupName)
}

// IsHTTPRouteDelegationLabelSelector checks if the BackendObjectReference is an HTTPRoute delegation label selector
// Parent routes may delegate to child routes using an HTTPRoute backend reference.
func IsHTTPRouteDelegationLabelSelector(ref gwv1.BackendObjectReference) bool {
	return ref.Group != nil && ref.Kind != nil && (string(*ref.Group)+"/"+string(*ref.Kind)) == apilabels.DelegationLabelSelector
}

// IsDelegatedHTTPRoute checks if the BackendObjectReference is a delegated HTTPRoute
// selected by an HTTPRoute or DelegationLabelSelector GVK reference.
func IsDelegatedHTTPRoute(ref gwv1.BackendObjectReference) bool {
	return IsHTTPRoute(ref) || IsHTTPRouteDelegationLabelSelector(ref)
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
