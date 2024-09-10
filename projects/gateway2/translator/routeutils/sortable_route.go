package routeutils

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type SortableRoute struct {
	Route     *v1.Route
	HttpRoute *gwv1.HTTPRoute
	Idx       int
}

type SortableRoutes []*SortableRoute

func (a SortableRoutes) Len() int           { return len(a) }
func (a SortableRoutes) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortableRoutes) Less(i, j int) bool { return !routeWrapperLessFunc(a[i], a[j]) }

func (a SortableRoutes) ToRoutes() []*v1.Route {
	var routes []*v1.Route
	for _, route := range a {
		routes = append(routes, route.Route)
	}
	return routes
}

func ToSortable(route *gwv1.HTTPRoute, routes []*v1.Route) SortableRoutes {
	var wrappers SortableRoutes
	for i, glooRoute := range routes {
		wrappers = append(wrappers, &SortableRoute{
			Route:     glooRoute,
			HttpRoute: route,
			Idx:       i,
		})
	}
	return wrappers
}

// Return true if A is lower priority than B
// https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.HTTPRouteRule
func routeWrapperLessFunc(wrapperA, wrapperB *SortableRoute) bool {
	// We know there's always a single matcher because of the route translator below
	matchA, matchB := wrapperA.Route.GetMatchers()[0], wrapperB.Route.GetMatchers()[0]
	switch typedPathA := matchA.GetPathSpecifier().(type) {
	case *matchers.Matcher_Prefix:
		// If they are both prefix, then check length
		switch typedPathB := matchB.GetPathSpecifier().(type) {
		case *matchers.Matcher_Prefix:
			if len(typedPathA.Prefix) != len(typedPathB.Prefix) {
				return len(typedPathA.Prefix) < len(typedPathB.Prefix)
			}
		// Exact and Regex always takes precedence over prefix
		case *matchers.Matcher_Exact, *matchers.Matcher_Regex:
			return true
		}

	case *matchers.Matcher_Exact:
		switch typedPathB := matchB.GetPathSpecifier().(type) {
		case *matchers.Matcher_Exact:
			if len(typedPathA.Exact) != len(typedPathB.Exact) {
				return len(typedPathA.Exact) < len(typedPathB.Exact)
			}

		// Exact always takes precedence over regex and prefix
		case *matchers.Matcher_Regex, *matchers.Matcher_Prefix:
			return false
		}

	case *matchers.Matcher_Regex:
		switch matchB.GetPathSpecifier().(type) {
		// Regex always takes precedence over prefix
		case *matchers.Matcher_Prefix:
			return false
		// Exact always takes precedence over regex
		case *matchers.Matcher_Exact:
			return true
		case *matchers.Matcher_Regex:
			// Don't prioritize one regex over another based on their lengths
			// as it doesn't make sense to do so and would be quite arbitrary,
			// so prioritize on the remaining criteria evaluated below instead.
		}
	}

	// If this matcher doesn't have a method match, then it's lower priority
	if len(matchA.GetMethods()) != len(matchB.GetMethods()) {
		return len(matchA.GetMethods()) < len(matchB.GetMethods())
	}

	if len(matchA.GetHeaders()) != len(matchB.GetHeaders()) {
		return len(matchA.GetHeaders()) < len(matchB.GetHeaders())
	}

	if len(matchA.GetQueryParameters()) != len(matchB.GetQueryParameters()) {
		return len(matchA.GetQueryParameters()) < len(matchB.GetQueryParameters())
	}

	if !wrapperA.HttpRoute.CreationTimestamp.Time.Equal(wrapperB.HttpRoute.CreationTimestamp.Time) {
		return wrapperA.HttpRoute.CreationTimestamp.Time.After(wrapperB.HttpRoute.CreationTimestamp.Time)
	}
	if wrapperA.HttpRoute.Name != wrapperB.HttpRoute.Name || wrapperA.HttpRoute.Namespace != wrapperB.HttpRoute.Namespace {
		return types.NamespacedName{Namespace: wrapperA.HttpRoute.Namespace, Name: wrapperA.HttpRoute.Name}.String() >
			types.NamespacedName{Namespace: wrapperB.HttpRoute.Namespace, Name: wrapperB.HttpRoute.Name}.String()
	}

	return wrapperA.Idx > wrapperB.Idx
}
