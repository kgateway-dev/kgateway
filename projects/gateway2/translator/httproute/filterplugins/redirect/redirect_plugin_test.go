package redirect_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/redirect"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func intptr(i int) *int {
	return &i
}

func hostname(s string) *gwv1.PreciseHostname {
	h := gwv1.PreciseHostname(s)
	return &h
}

var _ = Describe("RedirectPlugin", func() {
	It("applies redirect filter", func() {

		plugin := redirect.NewPlugin()
		filter := gwv1.HTTPRouteFilter{
			Type: gwv1.HTTPRouteFilterRequestRedirect,
			RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
				Hostname:   hostname("foo"),
				StatusCode: intptr(301),
			},
		}
		expectedRoute := &v1.Route{
			Options: &v1.RouteOptions{},
			Action: &v1.Route_RedirectAction{
				RedirectAction: &v1.RedirectAction{
					ResponseCode: v1.RedirectAction_MOVED_PERMANENTLY,
					HostRedirect: "foo",
				},
			},
		}
		filtertests.AssertExpectedRoute(
			plugin,
			filter,
			expectedRoute,
			false,
		)
	})
})
