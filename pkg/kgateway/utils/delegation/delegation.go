package delegation

import (
	"path"
	"reflect"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
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

// FilterDelegatedRuleMatches returns the matches of one delegatee route rule that
// survive delegation from a parent match, in declaration order:
//
//   - When inheritMatcher is set, every child match is valid and is merged onto the
//     parent match via MergeParentChildRouteMatch. A rule with no matches inherits
//     the parent match verbatim.
//   - Otherwise, a child match survives only if IsDelegatedRouteMatch accepts it
//     against the parent match. A rule with no matches yields nothing.
//
// Neither input is mutated. Matches derived from ruleMatches are deep copies, but
// an inherited parent match is a shallow copy that shares parentMatch's pointer and
// slice fields, so callers must not mutate the returned matches in place.
func FilterDelegatedRuleMatches(
	parentMatch gwv1.HTTPRouteMatch,
	ruleMatches []gwv1.HTTPRouteMatch,
	inheritMatcher bool,
) []gwv1.HTTPRouteMatch {
	// We use validMatches to store the matches in the child rule that are valid
	// with respect to the parent matcher.
	var validMatches []gwv1.HTTPRouteMatch

	// If the child route opts to inherit the parent's matcher and it does not specify its own matcher,
	// simply inherit the parent's matcher.
	if inheritMatcher && len(ruleMatches) == 0 {
		validMatches = append(validMatches, parentMatch)
	}

	for _, match := range ruleMatches {
		match := *match.DeepCopy()
		if inheritMatcher {
			// When inheriting the parent's matcher, all matches are valid.
			// In this case, the child inherits the parents matcher so we merge
			// the parent's matcher with the child's.
			MergeParentChildRouteMatch(&parentMatch, &match)
			validMatches = append(validMatches, match)
		} else if ok := IsDelegatedRouteMatch(parentMatch, match); ok {
			// Non-inherited matcher delegation requires matching child matcher to parent matcher
			// to delegate from the parent route to the child.
			validMatches = append(validMatches, match)
		}
	}

	return validMatches
}

// MergeParentChildRouteMatch is called only when inherit-parent-matcher is set.
// It merges the parent route match into the child as follows:
//   - the resulting path consists of parent path + child path
//   - the resulting headers consist of the combined headers from parent and child, with parent header taking
//     precedence on any name conflicts
//   - the resulting query parameters consist of the combined query parameters from parent and child, with parent
//     query params taking precedence on any name conflicts
//   - the child inherits the parent's method if specified; otherwise the child retains its own method
//
// A path match without a value is treated as unset: it contributes an empty
// segment to the join, so a child with no usable path inherits the parent's path
// as a prefix match.
func MergeParentChildRouteMatch(
	parent *gwv1.HTTPRouteMatch,
	child *gwv1.HTTPRouteMatch,
) {
	if parent == nil || child == nil {
		return
	}

	// A child path match without a value is treated as an unset path: the child
	// inherits the parent's path as a prefix match.
	if child.Path == nil || child.Path.Value == nil {
		child.Path = &gwv1.HTTPPathMatch{
			Type:  new(gwv1.PathMatchPathPrefix),
			Value: new(""),
		}
	}
	joined := path.Join(pathMatchValue(parent.Path), *child.Path.Value)
	if joined == "" {
		// Gateway API path values must be absolute, so a join of two empty paths
		// resolves to the root prefix rather than the empty string.
		joined = "/"
	}
	child.Path.Value = new(joined)

	// Inherit parent and child headers and query parameters while augmenting the merge
	// with additions specified on the child
	child.Headers = mergeHeaders(parent.Headers, child.Headers)
	child.QueryParams = mergeQueries(parent.QueryParams, child.QueryParams)

	// If parent specifies a method, inherit it (this will overwrite any method specified on the child)
	if parent.Method != nil {
		child.Method = new(*parent.Method)
	}
}

// pathMatchValue returns the value of a path match, treating an unset match or an
// unset value as an empty path.
func pathMatchValue(p *gwv1.HTTPPathMatch) string {
	if p == nil || p.Value == nil {
		return ""
	}
	return *p.Value
}

// mergeHeaders merges parent and child header matches. If a header name is specified on both
// the parent and child, the parent's header value takes precedence (i.e. child cannot overwrite it).
func mergeHeaders(
	parent, child []gwv1.HTTPHeaderMatch,
) []gwv1.HTTPHeaderMatch {
	merged := make(map[gwv1.HTTPHeaderName]gwv1.HTTPHeaderMatch)
	for _, h := range parent {
		merged[h.Name] = h
	}
	for _, h := range child {
		key := h.Name
		// Only add the child if it does not conflict with the parent
		if _, ok := merged[key]; !ok {
			merged[key] = h
		}
	}
	var result []gwv1.HTTPHeaderMatch
	for _, h := range merged {
		result = append(result, h)
	}
	// Sort for deterministic ordering
	slices.SortFunc(result, func(a, b gwv1.HTTPHeaderMatch) int {
		return strings.Compare(string(a.Name), string(b.Name))
	})
	return result
}

// mergeQueries merges parent and child query param matches. If a query param name is specified on both
// the parent and child, the parent's query param value takes precedence (i.e. child cannot overwrite it).
func mergeQueries(
	parent, child []gwv1.HTTPQueryParamMatch,
) []gwv1.HTTPQueryParamMatch {
	merged := make(map[gwv1.HTTPHeaderName]gwv1.HTTPQueryParamMatch)
	for _, h := range parent {
		merged[h.Name] = h
	}
	for _, h := range child {
		key := h.Name
		// Only add the child if it does not conflict with the parent
		if _, ok := merged[key]; !ok {
			merged[key] = h
		}
	}
	var result []gwv1.HTTPQueryParamMatch
	for _, h := range merged {
		result = append(result, h)
	}
	// Sort for deterministic ordering
	slices.SortFunc(result, func(a, b gwv1.HTTPQueryParamMatch) int {
		return strings.Compare(string(a.Name), string(b.Name))
	})
	return result
}
