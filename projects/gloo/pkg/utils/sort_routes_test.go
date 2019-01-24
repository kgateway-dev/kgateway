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
	makeRoute := func(pathType int, longPath bool) *v1.Route {
		pathStr := "/short"
		if longPath {
			pathStr = "/looooonnggggggg"
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
			for _, long := range []bool{true, false} {
				routes = append(routes, makeRoute(path, long))
			}
		}
		return routes
	}

	It("sorts the routes by longest path first", func() {
		sortedRoutes := makeSortedRoutes()
		expectedRoutes := makeSortedRoutes()
		rand.Shuffle(len(expectedRoutes), func(i, j int) {
			expectedRoutes[i], expectedRoutes[j] = expectedRoutes[j], expectedRoutes[i]
		})
		Expect(sortedRoutes).NotTo(Equal(expectedRoutes))
		SortRoutesLongestPathFirst(expectedRoutes)
		Expect(expectedRoutes).To(Equal(sortedRoutes))
	})
})
