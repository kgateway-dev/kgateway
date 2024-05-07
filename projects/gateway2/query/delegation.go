package query

import (
	"context"
	"reflect"
	"strings"

	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GetDelegatedRoutes returns the child routes that match the specified selector using
// backend ref and the parent route matcher.
//
// A child route matches a parent if all of the following conditions are met:
//
// 1. The child route matches the route selector specified by the parent's backendRef
//
// 2.The child route has a rule that matches the parent's route matcher:
//   - The child route's path must be a prefix of the parent's path
//   - The child route's headers must be a superset of the parent's headers
//   - The child route's query parameters must be a superset of the parent's query parameters
//
// If a child route's rule does not match the given parent match, it is not included in the route returned.
func (r *gatewayQueries) GetDelegatedRoutes(
	ctx context.Context,
	backendRef gwv1.BackendObjectReference,
	parentMatch gwv1.HTTPRouteMatch,
	parentRef types.NamespacedName,
) ([]gwv1.HTTPRoute, error) {
	if parentMatch.Path == nil || parentMatch.Path.Type == nil || *parentMatch.Path.Type != gwv1.PathMatchPathPrefix {
		return nil, nil
	}

	// Lookup the delegated routes; default to parent namespace
	delegatedNs := parentRef.Namespace
	if backendRef.Namespace != nil {
		delegatedNs = string(*backendRef.Namespace)
	}

	var children []gwv1.HTTPRoute
	if string(backendRef.Name) == "" || string(backendRef.Name) == "*" {
		// consider this to be a wildcard
		var hrlist gwv1.HTTPRouteList
		err := r.client.List(ctx, &hrlist, client.InNamespace(delegatedNs))
		if err != nil {
			return nil, err
		}
		children = append(children, hrlist.Items...)
	} else {
		delegatedRef := types.NamespacedName{
			Namespace: delegatedNs,
			Name:      string(backendRef.Name),
		}
		// Lookup the child route
		child := &gwv1.HTTPRoute{}
		err := r.client.Get(ctx, types.NamespacedName{Namespace: delegatedRef.Namespace, Name: delegatedRef.Name}, child)
		if err != nil {
			return nil, err
		}
		children = append(children, *child)
	}

	// Select the child routes that match the parent
	var selected []gwv1.HTTPRoute
	for _, child := range children {
		// Check if the child route has a prefix that matches the parent.
		// Only rules matching the parent prefix are considered.
		matched := false
		for _, rule := range child.Spec.Rules {
			var validMatches []gwv1.HTTPRouteMatch
			for _, match := range rule.Matches {
				if ok := isDelegatedRouteMatch(parentMatch, parentRef, match, child.Namespace, child.Spec.ParentRefs); ok {
					validMatches = append(validMatches, match)
					matched = true
				}
			}
			rule.Matches = validMatches
		}
		if matched {
			selected = append(selected, child)
		}
	}

	return selected, nil
}

func isDelegatedRouteMatch(
	parent gwv1.HTTPRouteMatch,
	parentRef types.NamespacedName,
	child gwv1.HTTPRouteMatch,
	childNs string,
	parentRefs []gwv1.ParentReference,
) bool {
	// If the child has parentRefs set, validate that it matches the parent route
	if len(parentRefs) > 0 {
		matched := false
		for _, ref := range parentRefs {
			refNs := childNs
			if ref.Namespace != nil {
				refNs = string(*ref.Namespace)
			}
			if ref.Group != nil && *ref.Group == wellknown.GatewayGroup &&
				ref.Kind != nil && *ref.Kind == wellknown.HTTPRouteKind &&
				string(ref.Name) == parentRef.Name &&
				refNs == parentRef.Namespace {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

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

	return true
}
