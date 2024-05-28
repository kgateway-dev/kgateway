package urlrewrite_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/urlrewrite"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = DescribeTable(
	"UrlRewritePlugin",
	func(
		plugin plugins.RoutePlugin,
		filter gwv1.HTTPRouteFilter,
		outputRoute *v1.Route,
		expectedRoute *v1.Route,
		match *gwv1.HTTPRouteMatch,
	) {
		filtertests.AssertExpectedRouteWithMatch(
			plugin,
			outputRoute,
			expectedRoute,
			match,
			true,
			filter,
		)
	},
	Entry(
		"applies full path rewrite filter",
		urlrewrite.NewPlugin(),
		gwv1.HTTPRouteFilter{
			Type: gwv1.HTTPRouteFilterURLRewrite,
			URLRewrite: &gwv1.HTTPURLRewriteFilter{
				Hostname: ptr.To(gwv1.PreciseHostname("foo")),
				Path: &gwv1.HTTPPathModifier{
					Type:            gwv1.FullPathHTTPPathModifier,
					ReplaceFullPath: ptr.To("/bar"),
				},
			},
		},
		&v1.Route{
			Options: &v1.RouteOptions{},
		},
		&v1.Route{
			Options: &v1.RouteOptions{
				HostRewriteType: &v1.RouteOptions_HostRewrite{
					HostRewrite: "foo",
				},
				RegexRewrite: &v3.RegexMatchAndSubstitute{
					Pattern: &v3.RegexMatcher{
						Regex: ".*",
					},
					Substitution: "/bar",
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
					ReplacePrefixMatch: ptr.To("/bar"),
				},
			},
		},
		&v1.Route{
			Options: &v1.RouteOptions{},
		},
		&v1.Route{
			Options: &v1.RouteOptions{
				PrefixRewrite: &wrapperspb.StringValue{
					Value: "/bar",
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
				Hostname: ptr.To(gwv1.PreciseHostname("foo")),
				Path: &gwv1.HTTPPathModifier{
					Type:               gwv1.PrefixMatchHTTPPathModifier,
					ReplacePrefixMatch: ptr.To("/"),
				},
			},
		},
		&v1.Route{
			Options: &v1.RouteOptions{},
		},
		&v1.Route{
			Options: &v1.RouteOptions{
				HostRewriteType: &v1.RouteOptions_HostRewrite{
					HostRewrite: "foo",
				},
				RegexRewrite: &v3.RegexMatchAndSubstitute{
					Pattern: &v3.RegexMatcher{
						Regex: "^/hello/world\\/*",
					},
					Substitution: "/",
				},
			},
		},
		&gwv1.HTTPRouteMatch{
			Path: &gwv1.HTTPPathMatch{
				Value: ptr.To("/hello/world"),
			},
		},
	),
)
