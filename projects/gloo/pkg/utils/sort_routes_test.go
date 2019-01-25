package utils

import (
	"math/rand"
	"time"

	"github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("PathAsString", func() {
	rand.Seed(time.Now().Unix())

	makeSortedRoutes := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{helpers.ExactPath, helpers.RegexPath, helpers.PrefixPath} {
			for _, length := range []int{9, 6, 3} {
				routes = append(routes, helpers.MakeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongPriority := func() []*v1.Route {
		var routes []*v1.Route
		for _, length := range []int{9, 6, 3} {
			for _, path := range []int{helpers.ExactPath, helpers.RegexPath, helpers.PrefixPath} {
				routes = append(routes, helpers.MakeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongPaths1 := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{helpers.RegexPath, helpers.ExactPath, helpers.PrefixPath} {
			for _, length := range []int{9, 6, 3} {
				routes = append(routes, helpers.MakeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongPaths2 := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{helpers.RegexPath, helpers.PrefixPath, helpers.ExactPath} {
			for _, length := range []int{9, 6, 3} {
				routes = append(routes, helpers.MakeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongPathPriority := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{helpers.PrefixPath, helpers.RegexPath, helpers.ExactPath} {
			for _, length := range []int{9, 6, 3} {
				routes = append(routes, helpers.MakeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongLength := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{helpers.PrefixPath, helpers.RegexPath, helpers.ExactPath} {
			for _, length := range []int{6, 3, 9} {
				routes = append(routes, helpers.MakeRoute(path, length))
			}
		}
		return routes
	}

	makeUnSortedRoutesWrongLengthPriority := func() []*v1.Route {
		var routes []*v1.Route
		for _, length := range []int{6, 3, 9} {
			for _, path := range []int{helpers.PrefixPath, helpers.RegexPath, helpers.ExactPath} {
				routes = append(routes, helpers.MakeRoute(path, length))
			}
		}
		return routes
	}

	It("sorts the routes by longest path first", func() {
		sortedRoutes := makeSortedRoutes()
		expectedRoutes := makeSortedRoutes()
		for count := 0; count < 100; count++ {
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
