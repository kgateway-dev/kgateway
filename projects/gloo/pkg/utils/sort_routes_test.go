package utils

import (
	"math/rand"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo/v2"
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

		for _, unsortedRoutes := range [][]*v1.Route{
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
		}
	})

	// Creates a slice of routes, each with two matchers. The second matcher is always the "smaller" one
	// in this test to make sure we actually traverse the slice to find the most-specific matcher on each route
	makeSortedMultiMatcherRoutes := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{helpers.ExactPath, helpers.RegexPath, helpers.PrefixPath} {
			for _, length := range []int{3, 2, 1} {
				routes = append(routes, helpers.MakeMultiMatcherRoute((path+1)%3, length, path, length+3))
			}
		}
		return routes
	}

	makeMultiMatcherRoutesWrongPathPriority := func() []*v1.Route {
		var routes []*v1.Route
		for _, path := range []int{helpers.ExactPath, helpers.PrefixPath, helpers.RegexPath} {
			for _, length := range []int{3, 2, 1} {
				routes = append(routes, helpers.MakeMultiMatcherRoute((path+1)%3, length, path, length+3))
			}
		}
		return routes
	}

	It("sorts the routes by longest matcher found on the route", func() {
		sortedRoutes := makeSortedMultiMatcherRoutes()
		expectedRoutes := makeSortedMultiMatcherRoutes()
		for count := 0; count < 100; count++ {
			rand.Shuffle(len(expectedRoutes), func(i, j int) {
				expectedRoutes[i], expectedRoutes[j] = expectedRoutes[j], expectedRoutes[i]
			})
			SortRoutesByPath(expectedRoutes)
			Expect(expectedRoutes).To(Equal(sortedRoutes))
		}

		for _, unsortedRoutes := range [][]*v1.Route{
			makeSortedMultiMatcherRoutes(),
			makeMultiMatcherRoutesWrongPathPriority(),
		} {
			SortRoutesByPath(unsortedRoutes)
			Expect(unsortedRoutes).To(Equal(makeSortedMultiMatcherRoutes()))
		}
	})

	It("sorts routes with nil matchers (they default to `/` prefix matcher) as largest", func() {
		routes := []*v1.Route{
			{Matchers: nil},
			{Matchers: []*matchers.Matcher{helpers.MakeMatcher(helpers.ExactPath, 10)}},
		}
		sortedRoutes := []*v1.Route{
			{Matchers: []*matchers.Matcher{helpers.MakeMatcher(helpers.ExactPath, 10)}},
			{Matchers: nil},
		}
		SortRoutesByPath(routes)
		Expect(routes).To(Equal(sortedRoutes))
	})

	It("all else being equal, paths are sorted lexicographically in descending order", func() {
		routes := []*v1.Route{
			{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo/a",
					}},
				},
			},
			{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/",
					}},
				},
			},
			{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo/a/hello",
					}},
				},
			},
			{
				// This one is lexicographically greater than the previous one,
				// so it should come first even though it's longer
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo/b",
					}},
				},
			},
		}
		sortedRoutes := []*v1.Route{
			{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo/b",
					}},
				},
			},
			{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo/a/hello",
					}},
				},
			},
			{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo/a",
					}},
				},
			},
			{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/",
					}},
				},
			},
		}
		SortRoutesByPath(routes)
		Expect(routes).To(Equal(sortedRoutes))
	})
})
