package utils

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"math/rand"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

const (
	exact = iota
	prefix
	regex
)

var _ = Describe("PathAsString", func() {
	rand.Seed(time.Now().Unix())
	makeRoute := func(pathType int, length int) *v1.Route {
		pathStr := "/"
		for i := 0; i < length; i++ {
			pathStr += "s/"
		}
		m := &v1.Matcher{}
		switch pathType {
		case exact:
			m.PathSpecifier = &v1.Matcher_Exact{pathStr}
		case prefix:
			m.PathSpecifier = &v1.Matcher_Prefix{pathStr}
		case regex:
			m.PathSpecifier = &v1.Matcher_Regex{pathStr}
		default:
			panic("bad test")
		}
		return &v1.Route{Matcher: m}
	}

	makeSortedRoutes := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{exact, regex, prefix} {
			for _, length := range []int{9, 6, 3} {
				routes = append(routes, makeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongPriority := func() []*v1.Route {
		var routes []*v1.Route
		for _, length := range []int{9, 6, 3} {
			for _, path := range []int{exact, regex, prefix} {
				routes = append(routes, makeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongPaths1 := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{regex, exact, prefix} {
			for _, length := range []int{9, 6, 3} {
				routes = append(routes, makeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongPaths2 := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{regex, prefix, exact} {
			for _, length := range []int{9, 6, 3} {
				routes = append(routes, makeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongPathPriority := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{prefix, regex, exact} {
			for _, length := range []int{9, 6, 3} {
				routes = append(routes, makeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongLength := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{prefix, regex, exact} {
			for _, length := range []int{6, 3, 9} {
				routes = append(routes, makeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongLengthPriority := func() []*v1.Route {
		var routes []*v1.Route
		for _, length := range []int{6, 3, 9} {
			for _, path := range []int{prefix, regex, exact} {
				routes = append(routes, makeRoute(path, length))
			}
		}
		return routes
	}

	It("sorts the routes by longest path first", func() {
		sortedRoutes := makeSortedRoutes()
		expectedRoutes := makeSortedRoutes()
		for count := 0; count < 100; count ++ {
			rand.Shuffle(len(expectedRoutes), func(i, j int) {
				expectedRoutes[i], expectedRoutes[j] = expectedRoutes[j], expectedRoutes[i]
			})
			SortRoutesByPath(expectedRoutes)
			Expect(expectedRoutes).To(Equal(sortedRoutes))
		}

		for i, unsortedRoutes := range [][]*v1.Route{
			makeSortedRoutes(),
			makeUnSortedRoutesWrongPriority(),
			makeUnSortedRoutesWrongPaths1(),
			makeUnSortedRoutesWrongPaths2(),
			makeUnSortedRoutesWrongPathPriority(),
			makeUnSortedRoutesWrongLength(),
			makeUnSortedRoutesWrongLengthPriority(),
		} {
			SortRoutesByPath(unsortedRoutes)
			Expect(unsortedRoutes).To(Equal(makeSortedRoutes()))
			Expect(i).To(Equal(i))
		}
	})
})
