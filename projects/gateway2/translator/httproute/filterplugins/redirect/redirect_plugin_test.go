package redirect_test

import (
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/redirect"
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
		expectedRoute := &routev3.Route{
			Action: &routev3.Route_Redirect{
				Redirect: &routev3.RedirectAction{
					ResponseCode: routev3.RedirectAction_MOVED_PERMANENTLY,
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
