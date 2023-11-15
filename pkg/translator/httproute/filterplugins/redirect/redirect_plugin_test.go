package redirect_test

import (
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/redirect"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func ptr[T any](i T) *T {
	return &i
}

var _ = DescribeTable(
	"RedirectPlugin",
	func(
		plugin filterplugins.FilterPlugin,
		filter gwv1.HTTPRouteFilter,
		expectedRoute *routev3.Route,
	) {
		filtertests.AssertExpectedRoute(
			plugin,
			filter,
			expectedRoute,
			false,
		)
	},
	Entry(
		"applies redirect filter",
		redirect.NewPlugin(),
		gwv1.HTTPRouteFilter{
			Type: gwv1.HTTPRouteFilterRequestRedirect,
			RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
				Hostname:   ptr(gwv1.PreciseHostname("foo")),
				StatusCode: ptr(301),
			},
		},
		&routev3.Route{
			Action: &routev3.Route_Redirect{
				Redirect: &routev3.RedirectAction{
					ResponseCode: routev3.RedirectAction_MOVED_PERMANENTLY,
					HostRedirect: "foo",
				},
			},
		},
	),
)
