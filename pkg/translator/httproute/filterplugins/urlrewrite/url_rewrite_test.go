package urlrewrite_test

import (
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/pkg/utils/regexutils"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/urlrewrite"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func ptr[T any](i T) *T {
	return &i
}

var _ = DescribeTable(
	"UrlRewritePlugin",
	func(
		plugin filterplugins.FilterPlugin,
		filter gwv1.HTTPRouteFilter,
		outputRoute *routev3.Route,
		expectedRoute *routev3.Route,
		match *gwv1.HTTPRouteMatch,
	) {
		filtertests.AssertExpectedRouteWith(
			plugin,
			filter,
			outputRoute,
			expectedRoute,
			match,
			true,
		)
	},
	Entry(
		"applies full path rewrite filter",
		urlrewrite.NewPlugin(),
		gwv1.HTTPRouteFilter{
			Type: gwv1.HTTPRouteFilterURLRewrite,
			URLRewrite: &gwv1.HTTPURLRewriteFilter{
				Hostname: ptr(gwv1.PreciseHostname("foo")),
				Path: &gwv1.HTTPPathModifier{
					Type:            gwv1.FullPathHTTPPathModifier,
					ReplaceFullPath: ptr("/bar"),
				},
			},
		},
		&routev3.Route{
			Action: &routev3.Route_Route{
				Route: &routev3.RouteAction{},
			},
		},
		&routev3.Route{
			Action: &routev3.Route_Route{
				Route: &routev3.RouteAction{
					HostRewriteSpecifier: &routev3.RouteAction_HostRewriteLiteral{HostRewriteLiteral: "foo"},
					RegexRewrite: &matcherv3.RegexMatchAndSubstitute{
						Pattern:      regexutils.NewRegexWithProgramSize(".*", nil),
						Substitution: "/bar",
					},
				},
			},
		},
		nil,
	),
	Entry(
		"applies prefix rewrite filter",
		urlrewrite.NewPlugin(),
		gwv1.HTTPRouteFilter{
			Type: gwv1.HTTPRouteFilterURLRewrite,
			URLRewrite: &gwv1.HTTPURLRewriteFilter{
				Path: &gwv1.HTTPPathModifier{
					Type:               gwv1.PrefixMatchHTTPPathModifier,
					ReplacePrefixMatch: ptr("/bar"),
				},
			},
		},
		&routev3.Route{
			Action: &routev3.Route_Route{
				Route: &routev3.RouteAction{},
			},
		},
		&routev3.Route{
			Action: &routev3.Route_Route{
				Route: &routev3.RouteAction{
					PrefixRewrite: "/bar",
				},
			},
		},
		&gwv1.HTTPRouteMatch{
			Path: &gwv1.HTTPPathMatch{},
		},
	),
	Entry(
		"applies prefix rewrite filter with / rewrite",
		urlrewrite.NewPlugin(),
		gwv1.HTTPRouteFilter{
			Type: gwv1.HTTPRouteFilterURLRewrite,
			URLRewrite: &gwv1.HTTPURLRewriteFilter{
				Hostname: ptr(gwv1.PreciseHostname("foo")),
				Path: &gwv1.HTTPPathModifier{
					Type:               gwv1.PrefixMatchHTTPPathModifier,
					ReplacePrefixMatch: ptr("/"),
				},
			},
		},
		&routev3.Route{
			Action: &routev3.Route_Route{
				Route: &routev3.RouteAction{},
			},
		},
		&routev3.Route{
			Action: &routev3.Route_Route{
				Route: &routev3.RouteAction{
					HostRewriteSpecifier: &routev3.RouteAction_HostRewriteLiteral{HostRewriteLiteral: "foo"},
					RegexRewrite: &matcherv3.RegexMatchAndSubstitute{
						Pattern:      regexutils.NewRegexWithProgramSize("^/hello/world\\/*", nil),
						Substitution: "/",
					},
				},
			},
		},
		&gwv1.HTTPRouteMatch{
			Path: &gwv1.HTTPPathMatch{
				Value: ptr("/hello/world"),
			},
		},
	),
)
