package surveyutils_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	. "github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Route", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()

		vsClient := helpers.MustVirtualServiceClient()
		vs := &gatewayv1.VirtualService{
			Metadata: core.Metadata{
				Name:      "vs",
				Namespace: "gloo-system",
			},
			VirtualHost: &v1.VirtualHost{
				Name: "vs",
				Routes: []*v1.Route{{
					Matcher: &v1.Matcher{
						PathSpecifier: &v1.Matcher_Prefix{Prefix: "/"},
					}}, {
					Matcher: &v1.Matcher{
						PathSpecifier: &v1.Matcher_Prefix{Prefix: "/r"},
					}},
				},
			},
		}
		_, err := vsClient.Write(vs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should select a route", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString("vsvc prompt:")
			c.SendLine("")
			c.ExpectString("route prompt:")
			c.PressDown()
			c.SendLine("")
			c.ExpectEOF()
		}, func() {
			var opts options.Options
			_, idx, err := SelectRouteInteractive(&opts, "vsvc prompt:", "route prompt:")
			Expect(err).NotTo(HaveOccurred())
			Expect(idx).To(Equal(1))
		})
	})
})
