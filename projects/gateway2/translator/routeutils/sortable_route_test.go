package routeutils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("RouteWrappersTest", func() {
	defaultMatcher := func() *matchers.Matcher {
		return &matchers.Matcher{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: "/",
			},
		}
	}

	defaultRt := func() *gwv1.HTTPRoute {
		return &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{},
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
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{defaultMatcher()},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{defaultMatcher()},
				},
			},
			false,
		),
		Entry(
			"ExactPaths will take precedence over prefix",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/exact",
							},
						},
					},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Exact{
								Exact: "/exact",
							},
						},
					},
				},
			},
			true,
		),
		Entry(
			"ExactPaths will take precedence over Regex",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Regex{
								Regex: "/exact",
							},
						},
					},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Exact{
								Exact: "/exact",
							},
						},
					},
				},
			},
			true,
		),
		Entry(
			"PrefixPaths check length",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/exact",
							},
						},
					},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/exact/2",
							},
						},
					},
				},
			},
			true,
		),
		Entry(
			"matching paths will check method",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{defaultMatcher()},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/",
							},
							Methods: []string{"GET"},
						},
					},
				},
			},
			true,
		),
		Entry(
			"matching paths and method will check headers",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{defaultMatcher()},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/",
							},
							Headers: []*matchers.HeaderMatcher{
								{
									Name:  "test",
									Value: "hello",
								},
							},
						},
					},
				},
			},
			true,
		),
		Entry(
			"All else fails use query",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{defaultMatcher()},
				},
			},
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/",
							},
							QueryParameters: []*matchers.QueryParameterMatcher{
								{
									Name:  "test",
									Value: "hello",
								},
							},
						},
					},
				},
			},
			true,
		),
	)
})
