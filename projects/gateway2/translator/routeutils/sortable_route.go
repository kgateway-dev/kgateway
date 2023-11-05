package routeutils

import (
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

type SortableRoute struct {
	InputMatch gwv1.HTTPRouteMatch
	Route      *routev3.Route
	HttpRoute  *gwv1.HTTPRoute
	Idx        int
}

type SortableRoutes []*SortableRoute

func (a SortableRoutes) Len() int           { return len(a) }
func (a SortableRoutes) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortableRoutes) Less(i, j int) bool { return !routeWrapperLessFunc(a[i], a[j]) }

func (a SortableRoutes) ToRoutes() []*routev3.Route {
	var routes []*routev3.Route
	for _, route := range a {
		routes = append(routes, route.Route)
	}
	return routes
}

func ToSortable(route *gwv1.HTTPRoute, routes []*routev3.Route) SortableRoutes {
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

func methodLen(p *gwv1.HTTPMethod) int {
	if p == nil {
		return 0
	}
	return 1
}

// Return true if A is lower priority than B
// https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.HTTPRouteRule
func routeWrapperLessFunc(wrapperA, wrapperB *SortableRoute) bool {
	// We know there's always a single matcher because of the route translator below
	matchA, matchB := wrapperA.InputMatch, wrapperB.InputMatch
	pathTypeA, pathA := ParsePath(matchA.Path)
	pathTypeB, pathB := ParsePath(matchB.Path)
	switch pathTypeA {
	case gwv1.PathMatchPathPrefix:
		// If they are both prefix, then check length
		switch pathTypeB {
		case gwv1.PathMatchPathPrefix:
			if len(pathA) != len(pathB) {
				return len(pathA) < len(pathB)
			}
		case gwv1.PathMatchExact: // Exact always takes precedence
			return true
		case gwv1.PathMatchRegularExpression: // Prefix > regex
			return false
		}
	case gwv1.PathMatchExact:
		// Exact always takes precedence, but for double exact it doesn't really matter
		switch pathTypeB {
		case gwv1.PathMatchPathPrefix:
			return false
		case gwv1.PathMatchExact:
			if len(pathA) != len(pathB) {
				return len(pathA) < len(pathB)
			}
		case gwv1.PathMatchRegularExpression:
			return false
		}
	case gwv1.PathMatchRegularExpression:
		switch pathTypeB {
		case gwv1.PathMatchPathPrefix:
			return true
		case gwv1.PathMatchExact:
			return true
		case gwv1.PathMatchRegularExpression:
			if len(pathA) != len(pathB) {
				return len(pathA) < len(pathB)
			}
		}
	}

	// If this matcher doesn't have a method match, then it's lower priority
	if methodLen(matchA.Method) != methodLen(matchB.Method) {
		return methodLen(matchA.Method) < methodLen(matchB.Method)
	}

	if len(matchA.Headers) != len(matchB.Headers) {
		return len(matchA.Headers) < len(matchB.Headers)
	}

	if len(matchA.QueryParams) != len(matchB.QueryParams) {
		return len(matchA.QueryParams) < len(matchB.QueryParams)
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
