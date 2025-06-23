package routeutils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

type SortableRoute struct {
	Route       ir.HttpRouteRuleMatchIR
	RouteObject metav1.Object
	Idx         int
}

type SortableRoutes []*SortableRoute

func (a SortableRoutes) Len() int {
	return len(a)
}

func (a SortableRoutes) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a SortableRoutes) Less(i, j int) bool {
	// If weights are different, use weight to determine order
	if a[i].Route.PrecedenceWeight != a[j].Route.PrecedenceWeight {
		return a[i].Route.PrecedenceWeight > a[j].Route.PrecedenceWeight
	}

	// If weights are equal, use the existing comparison logic
	return !routeWrapperLessFunc(a[i], a[j])
}

func (a SortableRoutes) ToRoutes() []ir.HttpRouteRuleMatchIR {
	routes := make([]ir.HttpRouteRuleMatchIR, 0, len(a))
	for _, route := range a {
		routes = append(routes, route.Route)
	}
	return routes
}

func ToSortable(obj metav1.Object, routes []ir.HttpRouteRuleMatchIR) SortableRoutes {
	var wrappers SortableRoutes
	for i, route := range routes {
		wrappers = append(wrappers, &SortableRoute{
			Route:       route,
			RouteObject: obj,
			Idx:         i,
		})
	}
	return wrappers
}

func ParsePath(path *gwv1.HTTPPathMatch) (gwv1.PathMatchType, string) {
	pathType := gwv1.PathMatchPathPrefix
	pathValue := "/"
	if path != nil && path.Type != nil {
		pathType = *path.Type
	}
	if path != nil && path.Value != nil {
		pathValue = *path.Value
	}
	return pathType, pathValue
}

func lessPath(a, b *gwv1.HTTPPathMatch) *bool {
	atype, avalue := ParsePath(a)
	btype, bvalue := ParsePath(b)

	switch atype {
	case gwv1.PathMatchPathPrefix:
		// If they are both prefix, then check length
		switch btype {
		case gwv1.PathMatchPathPrefix:
			if len(avalue) != len(bvalue) {
				return ptr.To(len(avalue) < len(bvalue))
			}
		// Exact and Regex always takes precedence over prefix
		case gwv1.PathMatchExact, gwv1.PathMatchRegularExpression:
			return ptr.To(true)
		}

	case gwv1.PathMatchExact:
		switch btype {
		case gwv1.PathMatchExact:
			if len(avalue) != len(bvalue) {
				return ptr.To(len(avalue) < len(bvalue))
			}

		// Exact always takes precedence over regex and prefix
		case gwv1.PathMatchRegularExpression, gwv1.PathMatchPathPrefix:
			return ptr.To(false)
		}

	case gwv1.PathMatchRegularExpression:
		switch btype {
		// Regex always takes precedence over prefix
		case gwv1.PathMatchPathPrefix:
			return ptr.To(false)
		// Exact always takes precedence over regex
		case gwv1.PathMatchExact:
			return ptr.To(true)
		case gwv1.PathMatchRegularExpression:
			// Don't prioritize one regex over another based on their lengths
			// as it doesn't make sense to do so and would be quite arbitrary,
			// so prioritize on the remaining criteria evaluated below instead.
		}
	}
	// TODO: log dpanic here, this should never happen
	return nil
}

// Return true if A is lower priority than B
// https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.HTTPRouteRule
func routeWrapperLessFunc(wrapperA, wrapperB *SortableRoute) bool {
	// We know there's always a single matcher because of the route translator below
	matchA, matchB := wrapperA.Route.Match, wrapperB.Route.Match

	pathCompare := lessPath(matchA.Path, matchB.Path)
	if pathCompare != nil {
		return *pathCompare
	}

	// If this matcher doesn't have a method match, then it's lower priority
	if (matchA.Method == nil) != (matchB.Method == nil) {
		return matchB.Method != nil
	}

	if len(matchA.Headers) != len(matchB.Headers) {
		return len(matchA.Headers) < len(matchB.Headers)
	}

	if len(matchA.QueryParams) != len(matchB.QueryParams) {
		return len(matchA.QueryParams) < len(matchB.QueryParams)
	}

	wrapperASource := wrapperA.RouteObject
	wrapperBSource := wrapperB.RouteObject

	// Compare the 2 objects
	if !wrapperASource.GetCreationTimestamp().Time.Equal(wrapperBSource.GetCreationTimestamp().Time) {
		return wrapperASource.GetCreationTimestamp().After(wrapperBSource.GetCreationTimestamp().Time)
	}
	if wrapperASource.GetName() != wrapperBSource.GetName() || wrapperASource.GetNamespace() != wrapperBSource.GetNamespace() {
		return types.NamespacedName{Namespace: wrapperASource.GetNamespace(), Name: wrapperASource.GetName()}.String() >
			types.NamespacedName{Namespace: wrapperBSource.GetNamespace(), Name: wrapperBSource.GetName()}.String()
	}

	// If these are delegated routes, compare their sources
	if wrapperA.Route.DelegatingParent != nil {
		wrapperASource = wrapperA.Route.Parent.SourceObject
	}
	if wrapperB.Route.DelegatingParent != nil {
		wrapperBSource = wrapperB.Route.Parent.SourceObject
	}
	// Repeat the object comparison but with original sources
	if !wrapperASource.GetCreationTimestamp().Time.Equal(wrapperBSource.GetCreationTimestamp().Time) {
		return wrapperASource.GetCreationTimestamp().After(wrapperBSource.GetCreationTimestamp().Time)
	}
	if wrapperASource.GetName() != wrapperBSource.GetName() || wrapperASource.GetNamespace() != wrapperBSource.GetNamespace() {
		return types.NamespacedName{Namespace: wrapperASource.GetNamespace(), Name: wrapperASource.GetName()}.String() >
			types.NamespacedName{Namespace: wrapperBSource.GetNamespace(), Name: wrapperBSource.GetName()}.String()
	}

	return wrapperA.Idx > wrapperB.Idx
}
