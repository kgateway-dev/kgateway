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
			"Exact paths will take precedence over prefix",
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
			"Exact paths will take precedence over Regex",
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
			"Regex paths will take precedence over Prefix",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Regex{
								Regex: "/regex.*",
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
								Prefix: "/prefix",
							},
						},
					},
				},
			},
			false,
		),
		Entry(
			"Regex paths will not take precedence over Regex regardless of their lengths",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Regex{
								Regex: "/regex.*",
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
							PathSpecifier: &matchers.Matcher_Regex{
								Regex: "/re.*",
							},
						},
					},
				},
			},
			false,
		),
		Entry(
			"Regex paths will not take precedence over Regex regardless of their lengths",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Regex{
								Regex: "/re.*",
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
							PathSpecifier: &matchers.Matcher_Regex{
								Regex: "/regex.*",
							},
						},
					},
				},
			},
			false,
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
			"different name same ns",
			&SortableRoute{
				HttpRoute: defaultRtB(),
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
									Name:  "test2",
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
			"one has more headers",
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
				},
			},
			true,
		),
		Entry(
			"one is higher more headers",
			&SortableRoute{
				HttpRoute: defaultRt(),
				Idx:       1,
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
			&SortableRoute{
				HttpRoute: defaultRt(),
				Idx:       0,
				Route: &v1.Route{
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/",
							},
							Headers: []*matchers.HeaderMatcher{
								{
									Name:  "test",
									Value: "hello2",
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
