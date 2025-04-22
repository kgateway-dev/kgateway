package delegation

import (
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// ChildRouteCanAttachToParentRef returns a boolean indicating whether the given delegatee/child
// route can attach to a parent referenced by its NamespacedName.
//
// A delegatee route can attach to a parent if either of the following conditions are true:
//   - the child does not specify ParentRefs (implicit attachment)
//   - the child has an HTTPRoute ParentReference that matches parentRef
func ChildRouteCanAttachToParentRef(
	routeNamespace string,
	routeParentRefs []gwv1.ParentReference,
	parentRef types.NamespacedName,
) bool {
	// no explicit parentRefs, so any parent is allowed
	if len(routeParentRefs) == 0 {
		return true
	}

	// validate that the child's parentRefs contains the specified parentRef
	for _, ref := range routeParentRefs {
		// default to the child's namespace if not specified
		refNs := routeNamespace
		if ref.Namespace != nil {
			refNs = string(*ref.Namespace)
		}
		// check if the ref matches the desired parentRef
		if ref.Group != nil && *ref.Group == wellknown.GatewayGroup &&
			ref.Kind != nil && *ref.Kind == wellknown.HTTPRouteKind &&
			string(ref.Name) == parentRef.Name &&
			refNs == parentRef.Namespace {
			return true
		}
	}
	return false
}

// ShouldInheritParentMatcher returns true if the inherit-parent-matcher annotation is set
func ShouldInheritParentMatcher(annotations map[string]string) bool {
	val, ok := annotations[apiannotations.DelegationInheritMatcher]
	if !ok {
		return false
	}
	switch strings.ToLower(val) {
	case "true", "yes", "enabled":
		return true

	default:
		return false
	}
}

// IsDelegatedRouteMatch returns true if the child is a valid delegatee of the parent.
// This will be true if the following conditions are met:
// - the parent path matcher must be of type PathPrefix
// - the parent path matcher value must be a prefix of the child path matcher value
// - the child header matchers must be a superset of the parent header matchers
// - the child query param matchers must be a superset of the parent query param matchers
// - if the parent method matcher is set, the child's method matcher value must be equal to the parent method matcher value
//
// Note: It is NOT called when DelegationInheritMatcher is set
func IsDelegatedRouteMatch(
	parent gwv1.HTTPRouteMatch,
	child gwv1.HTTPRouteMatch,
) bool {
	// Validate path
	if parent.Path == nil || parent.Path.Type == nil || *parent.Path.Type != gwv1.PathMatchPathPrefix {
		return false
	}
	parentPath := *parent.Path.Value
	if child.Path == nil || child.Path.Type == nil {
		return false
	}
	childPath := *child.Path.Value
	if !strings.HasPrefix(childPath, parentPath) {
		return false
	}

	// Validate that the child headers are a superset of the parent headers
	for _, parentHeader := range parent.Headers {
		found := false
		for _, childHeader := range child.Headers {
			if reflect.DeepEqual(parentHeader, childHeader) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Validate that the child query parameters are a superset of the parent headers
	for _, parentQuery := range parent.QueryParams {
		found := false
		for _, childQuery := range child.QueryParams {
			if reflect.DeepEqual(parentQuery, childQuery) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Validate that the child method matches the parent method
	if parent.Method != nil && (child.Method == nil || *parent.Method != *child.Method) {
		return false
	}

	return true
}
