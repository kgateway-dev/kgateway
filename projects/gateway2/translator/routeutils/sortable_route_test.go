package routeutils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func ptr[T any](t T) *T {
	return &t
}

var _ = Describe("RouteWrappersTest", func() {
	defaultMatcher := func() gwv1.HTTPRouteMatch {
		return gwv1.HTTPRouteMatch{
			Path: &gwv1.HTTPPathMatch{
				Value: ptr("/"),
			},
		}
	}

	defaultRt := func() *gwv1.HTTPRoute {
		return &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: "a-test",
			},
		}
	}
	defaultRtB := func() *gwv1.HTTPRoute {
		return &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: "b-test",
			},
		}
	}
	DescribeTable("Route Sorting",
		func(wrapperA, wrapperB *SortableRoute, expected bool) {

			Expect(
				routeWrapperLessFunc(wrapperA, wrapperB),
			).Should(Equal(expected))
		},
		Entry(
			"equal will return false",
			&SortableRoute{
				HttpRoute:  defaultRt(),
				InputMatch: defaultMatcher(),
			},
			&SortableRoute{
				HttpRoute:  defaultRt(),
				InputMatch: defaultMatcher(),
			},
			false,
		),
		Entry(
			"ExactPaths will take precedence over prefix",
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Value: ptr("/exact"),
					},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchExact),
						Value: ptr("/exact"),
					},
				},
			},
			true,
		),
		Entry(
			"ExactPaths will take precedence over Regex",
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchRegularExpression),
						Value: ptr("/exact"),
					},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchExact),
						Value: ptr("/exact"),
					},
				},
			},
			true,
		),
		Entry(
			"PrefixPaths check length",
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/exact"),
					},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/exact/2"),
					},
				},
			},
			true,
		),
		Entry(
			"matching paths will check method",
			&SortableRoute{
				HttpRoute:  defaultRt(),
				InputMatch: defaultMatcher(),
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/"),
					},
					Method: ptr(gwv1.HTTPMethod("GET")),
				},
			},
			true,
		),
		Entry(
			"matching paths and method will check headers",
			&SortableRoute{
				HttpRoute:  defaultRt(),
				InputMatch: defaultMatcher(),
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/"),
					},
					Headers: []gwv1.HTTPHeaderMatch{
						{
							Name:  "test",
							Value: "hello",
						},
					},
					Method: ptr(gwv1.HTTPMethod("GET")),
				},
			},
			true,
		),
		Entry(
			"different name same ns",
			&SortableRoute{
				HttpRoute: defaultRtB(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/"),
					},
					Headers: []gwv1.HTTPHeaderMatch{
						{
							Name:  "test",
							Value: "hello",
						},
					},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/"),
					},
					Headers: []gwv1.HTTPHeaderMatch{
						{
							Name:  "test2",
							Value: "hello",
						},
					},
				},
			},
			true,
		),
		Entry(
			"one has more headers",
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/"),
					},
					Headers: []gwv1.HTTPHeaderMatch{
						{
							Name:  "test",
							Value: "hello",
						},
					},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/"),
					},
					Headers: []gwv1.HTTPHeaderMatch{
						{
							Name:  "test2",
							Value: "hello",
						},
						{
							Name:  "test",
							Value: "hello",
						},
					},
				},
			},
			true,
		),
		Entry(
			"one is higher idx",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Idx:       1,
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/"),
					},
					Headers: []gwv1.HTTPHeaderMatch{
						{
							Name:  "test",
							Value: "hello",
						},
					},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				Idx:       0,
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/"),
					},
					Headers: []gwv1.HTTPHeaderMatch{
						{
							Name:  "test",
							Value: "hello2",
						},
					},
				},
			},
			true,
		),
		Entry(
			"All else fails use query",
			&SortableRoute{
				HttpRoute:  defaultRt(),
				InputMatch: defaultMatcher(),
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				InputMatch: gwv1.HTTPRouteMatch{
					Path: &gwv1.HTTPPathMatch{
						Type:  ptr(gwv1.PathMatchPathPrefix),
						Value: ptr("/"),
					},
					Headers: []gwv1.HTTPHeaderMatch{
						{
							Name:  "test",
							Value: "hello",
						},
					},
				},
			},
			true,
		),
	)
})
