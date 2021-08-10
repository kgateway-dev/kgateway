package utils

import (
	"sort"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// Opinionated method to sort routes by convention.
//
// For each route, find the "smallest" matcher (i.e., the most-specific one) and use that to sort the entire route.

// Matchers sort according to the following rules:
// 1. exact path < regex path < prefix path
// 2. lexicographically greater path string < lexicographically smaller path string
func SortRoutesByPath(routes []*v1.Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		smallest1 := getSmallestOrDefaultMatcher(routes[i].GetMatchers())
		smallest2 := getSmallestOrDefaultMatcher(routes[j].GetMatchers())
		return lessMatcher(smallest1, smallest2)
	})
}

func SortGatewayRoutesByPath(routes []*gatewayv1.Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		smallest1 := getSmallestOrDefaultMatcher(routes[i].GetMatchers())
		smallest2 := getSmallestOrDefaultMatcher(routes[j].GetMatchers())
		return lessMatcher(smallest1, smallest2)
	})
}

func getSmallestOrDefaultMatcher(matchers []*matchers.Matcher) *matchers.Matcher {
	smallest := defaults.DefaultMatcher()
	if len(matchers) > 0 {
		smallest = matchers[0]
	}
	for _, m := range matchers {
		if lessMatcher(m, smallest) {
			smallest = m
		}
	}
	return smallest
}

func lessMatcher(m1, m2 *matchers.Matcher) bool {
	if len(m1.GetMethods()) != len(m2.GetMethods()) {
		return len(m1.GetMethods()) > len(m2.GetMethods())
	}
	if pathTypePriority(m1) != pathTypePriority(m2) {
		return pathTypePriority(m1) < pathTypePriority(m2)
	}
	// all else being equal
	return PathAsString(m1) > PathAsString(m2)
}

const (
	// order matters here. iota assigns each const = 0, 1, 2 etc.
	pathPriorityEmpty = iota
	pathPriorityExact
	pathPriorityRegex
	pathPriorityPrefix
)

func pathTypePriority(m *matchers.Matcher) int {
	switch m.GetPathSpecifier().(type) {
	case *matchers.Matcher_Exact:
		return pathPriorityExact
	case *matchers.Matcher_Regex:
		return pathPriorityRegex
	case *matchers.Matcher_Prefix:
		return pathPriorityPrefix
	default:
		return pathPriorityEmpty
	}
}
