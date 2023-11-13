package redirect_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/redirect"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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
		expectedRoute *v1.Route,
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
		&v1.Route{
			Options: &v1.RouteOptions{},
			Action: &v1.Route_RedirectAction{
				RedirectAction: &v1.RedirectAction{
					ResponseCode: v1.RedirectAction_MOVED_PERMANENTLY,
					HostRedirect: "foo",
				},
			},
		},
	),
)
