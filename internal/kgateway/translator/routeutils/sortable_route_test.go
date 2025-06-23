package routeutils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func defaultMatcher() gwv1.HTTPRouteMatch {
	t := gwv1.PathMatchPathPrefix
	v := "/"
	return gwv1.HTTPRouteMatch{
		Path: &gwv1.HTTPPathMatch{
			Type:  &t,
			Value: &v,
		},
	}
}

func prefixMatch(s string) *gwv1.HTTPPathMatch {
	t := gwv1.PathMatchPathPrefix
	return &gwv1.HTTPPathMatch{
		Type:  &t,
		Value: &s,
	}
}

func prefixMatcher(s string) gwv1.HTTPRouteMatch {
	return gwv1.HTTPRouteMatch{
		Path: prefixMatch(s),
	}
}

func exactMatcher(s string) gwv1.HTTPRouteMatch {
	t := gwv1.PathMatchExact
	return gwv1.HTTPRouteMatch{
		Path: &gwv1.HTTPPathMatch{
			Type:  &t,
			Value: &s,
		},
	}
}

func regexMatcher(s string) gwv1.HTTPRouteMatch {
	t := gwv1.PathMatchRegularExpression
	return gwv1.HTTPRouteMatch{
		Path: &gwv1.HTTPPathMatch{
			Type:  &t,
			Value: &s,
		},
	}
}

func defaultRt() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: "a-test",
		},
	}
}

func defaultRtB() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: "b-test",
		},
	}
}

var _ = DescribeTable("SortableRoutes Less()",
	func(first, second *SortableRoute, shouldSwap bool) {
		in := SortableRoutes{first, second}
		Expect(in.Less(0, 1)).ToNot(Equal(shouldSwap), "failed swap=%t", shouldSwap)
	},
	Entry(
		"equal will not swap",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: defaultMatcher(),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: defaultMatcher(),
			},
		},
		false,
	),
	Entry(
		"Exact paths will take precedence over prefix",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: prefixMatcher("/exact"),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: exactMatcher("/exact"),
			},
		},
		true,
	),
	Entry(
		"Exact paths will take precedence over Regex",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: regexMatcher("/exact"),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: exactMatcher("/exact"),
			},
		},
		true,
	),
	Entry(
		"Regex paths will take precedence over Prefix",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: regexMatcher("/regex.*"),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: prefixMatcher("/prefix"),
			},
		},
		false,
	),
	Entry(
		"Regex paths will not take precedence over Regex regardless of their lengths",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: regexMatcher("/regex.*"),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: regexMatcher("/re.*"),
			},
		},
		false,
	),
	Entry(
		"Regex paths will not take precedence over Regex regardless of their lengths",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: regexMatcher("/re.*"),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: regexMatcher("/regex.*"),
			},
		},
		false,
	),
	Entry(
		"PrefixPaths check length",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: prefixMatcher("/exact"),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: prefixMatcher("/exact/2"),
			},
		},
		true,
	),
	Entry(
		"matching paths will check method",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: defaultMatcher(),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: gwv1.HTTPRouteMatch{
					Path:   prefixMatch("/"),
					Method: ptr.To(gwv1.HTTPMethod("GET")),
				},
			},
		},
		true,
	),
	Entry(
		"matching paths and method will check headers",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: defaultMatcher(),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: gwv1.HTTPRouteMatch{
					Path: prefixMatch("/"),
					Headers: []gwv1.HTTPHeaderMatch{{
						Name:  gwv1.HTTPHeaderName("test"),
						Value: "hello",
					}},
				},
			},
		},
		true,
	),
	Entry(
		"different name same ns",
		&SortableRoute{
			RouteObject: defaultRtB(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: gwv1.HTTPRouteMatch{
					Path: prefixMatch("/"),
					Headers: []gwv1.HTTPHeaderMatch{{
						Name:  gwv1.HTTPHeaderName("test"),
						Value: "hello",
					}},
				},
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: gwv1.HTTPRouteMatch{
					Path: prefixMatch("/"),
					Headers: []gwv1.HTTPHeaderMatch{{
						Name:  gwv1.HTTPHeaderName("test2"),
						Value: "hello",
					}},
				},
			},
		},
		true,
	),
	Entry(
		"one has more headers",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: gwv1.HTTPRouteMatch{
					Path: prefixMatch("/"),
					Headers: []gwv1.HTTPHeaderMatch{{
						Name:  gwv1.HTTPHeaderName("test"),
						Value: "hello",
					}},
				},
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: gwv1.HTTPRouteMatch{
					Path: prefixMatch("/"),
					Headers: []gwv1.HTTPHeaderMatch{{
						Name:  gwv1.HTTPHeaderName("test2"),
						Value: "hello",
					}, {
						Name:  gwv1.HTTPHeaderName("test"),
						Value: "hello",
					}},
				},
			},
		},
		true,
	),
	Entry(
		"one is higher more headers",
		&SortableRoute{
			RouteObject: defaultRt(),
			Idx:         1,
			Route: ir.HttpRouteRuleMatchIR{
				Match: gwv1.HTTPRouteMatch{
					Path: prefixMatch("/"),
					Headers: []gwv1.HTTPHeaderMatch{{
						Name:  gwv1.HTTPHeaderName("test"),
						Value: "hello",
					}},
				},
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Idx:         0,
			Route: ir.HttpRouteRuleMatchIR{
				Match: gwv1.HTTPRouteMatch{
					Path: prefixMatch("/"),
					Headers: []gwv1.HTTPHeaderMatch{{
						Name:  gwv1.HTTPHeaderName("test"),
						Value: "hello2",
					}},
				},
			},
		},
		true,
	),
	Entry(
		"All else fails use query",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: defaultMatcher(),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: gwv1.HTTPRouteMatch{
					Path: prefixMatch("/"),
					QueryParams: []gwv1.HTTPQueryParamMatch{
						{
							Type:  ptr.To(gwv1.QueryParamMatchExact),
							Name:  "test",
							Value: "hello",
						},
					},
				},
			},
		},
		true,
	),
	Entry(
		"Higher PrecedenceWeights will always take precedence: preserve order",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				PrecedenceWeight: 1,
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				PrecedenceWeight: -1,
			},
		},
		false,
	),
	Entry(
		"Higher PrecedenceWeights will always take precedence: swap",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				PrecedenceWeight: -1,
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				PrecedenceWeight: 1,
			},
		},
		true,
	),
	Entry(
		"Same PrecedenceWeights will defer to Gateway API precedence rules (prefer exact over regex)",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: regexMatcher("/exact"),
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match: exactMatcher("/exact"),
			},
		},
		true,
	),
	Entry(
		"Higher PrecedenceWeight will override Gateway API precedence rules (use regex over exact)",
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match:            regexMatcher("/exact"),
				PrecedenceWeight: 1,
			},
		},
		&SortableRoute{
			RouteObject: defaultRt(),
			Route: ir.HttpRouteRuleMatchIR{
				Match:            exactMatcher("/exact"),
				PrecedenceWeight: 0,
			},
		},
		false,
	),
)
