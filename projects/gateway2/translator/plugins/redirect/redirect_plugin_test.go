package redirect_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/redirect"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = DescribeTable(
	"RedirectPlugin",
	func(
		plugin plugins.RoutePlugin,
		filter gwv1.HTTPRouteFilter,
		expectedRoute *v1.Route,
	) {
		filtertests.AssertExpectedRoute(
			plugin,
			expectedRoute,
			false,
			filter,
		)
	},
	Entry(
		"applies redirect filter",
		redirect.NewPlugin(),
		gwv1.HTTPRouteFilter{
			Type: gwv1.HTTPRouteFilterRequestRedirect,
			RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
				Hostname:   ptr.To(gwv1.PreciseHostname("foo")),
				StatusCode: ptr.To(301),
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
