package utils

import (
	"sort"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// opinionated method to sort routes by convention
// routes are sorted in the following way:
// 1. exact path < regex path < path prefix
// 2. longer path string < shorter path string
func SortRoutesByPath(routes []*v1.Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		return lessMatcher(routes[i].Matcher, routes[j].Matcher)
	})
}

func SortGatewayRoutesByPath(routes []*gatewayv1.Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		return lessMatcher(routes[i].Matcher, routes[j].Matcher)
	})
}

func lessMatcher(m1, m2 *v1.Matcher) bool {

	// Handle nil matchers by de-prioritizing them.
	// This is just to handle panics, as Gloo will reject routes with nil matchers
	if m1 == nil {
		return false
	} else if m2 == nil {
		return true
	}

	if len(m1.Methods) != len(m2.Methods) {
		return len(m1.Methods) > len(m2.Methods)
	}
	if pathTypePriority(m1) != pathTypePriority(m2) {
		return pathTypePriority(m1) < pathTypePriority(m2)
	}
	// all else being equal
	return PathAsString(m1) > PathAsString(m2)
}

const (
	// order matters here. iota assigns each const = 0, 1, 2 etc.
	pathPriorityExact = iota
	pathPriorityRegex
	pathPriorityPrefix
)

func pathTypePriority(m *v1.Matcher) int {
	switch m.GetPathSpecifier().(type) {
	case *v1.Matcher_Exact:
		return pathPriorityExact
	case *v1.Matcher_Regex:
		return pathPriorityRegex
	case *v1.Matcher_Prefix:
		return pathPriorityPrefix
	default:
		panic("invalid matcher path type, must be one of: {Matcher_Regex, Matcher_Exact, Matcher_Prefix}")
	}
}
