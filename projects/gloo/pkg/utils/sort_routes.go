package utils

import (
	"sort"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// opinionated method to sort routes by convention
//
// for each route, find the "largest" matcher
// (i.e., the most-specific one) and use that
// to sort the entire route

// matchers sort according to the following rules:
// 1. exact path < regex path < prefix path
// 2. longer path string < shorter path string
func SortRoutesByPath(routes []*v1.Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		smallest1 := *routes[i].Matchers[0] // bounds error not possible within a slice sort function
		for _, m := range routes[i].Matchers {
			if lessMatcher(m, &smallest1) {
				smallest1 = *m
			}
		}

		smallest2 := *routes[j].Matchers[0] // bounds error not possible within a slice sort function
		for _, m := range routes[j].Matchers {
			if lessMatcher(m, &smallest2) {
				smallest2 = *m
			}
		}

		return lessMatcher(&smallest1, &smallest2)
	})
}

func SortGatewayRoutesByPath(routes []*gatewayv1.Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		smallest1 := *routes[i].Matchers[0] // bounds error not possible within a slice sort function
		for _, m := range routes[i].Matchers {
			if lessMatcher(m, &smallest1) {
				smallest1 = *m
			}
		}

		smallest2 := *routes[j].Matchers[0] // bounds error not possible within a slice sort function
		for _, m := range routes[j].Matchers {
			if lessMatcher(m, &smallest2) {
				smallest2 = *m
			}
		}
		return lessMatcher(&smallest1, &smallest2)
	})
}

func lessMatcher(m1, m2 *v1.Matcher) bool {
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
	switch m.PathSpecifier.(type) {
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
