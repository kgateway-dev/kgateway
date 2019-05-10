package add_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Routes interactive", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ugclient := helpers.MustUpstreamGroupClient()
		ugclient.Write(&gloov1.UpstreamGroup{
			Metadata: core.Metadata{
				Name:      "default",
				Namespace: "default",
			},
		}, clients.WriteOpts{})

		upClient := helpers.MustUpstreamClient()
		upClient.Write(&gloov1.Upstream{
			Metadata: core.Metadata{
				Name:      "up",
				Namespace: "gloo-system",
			},
		}, clients.WriteOpts{})
	})

	It("Create interactive route", func() {
		testutil.ExpectInteractive(func(c *testutil.Console) {
			c.ExpectString("Choose a Virtual Service to add the route to:")
			c.SendLine("")
			c.ExpectString("name of the virtual service:")
			c.SendLine("default")
			c.ExpectString("namespace of the virtual service:")
			c.SendLine("gloo-system")
			c.ExpectString("Choose a path match type:")
			c.SendLine("pre")
			c.ExpectString("What path prefix should we match?")
			c.SendLine("")
			c.ExpectString("Add a header matcher for this function (empty to skip)?")
			c.SendLine("")
			c.ExpectString("HTTP Method to match for this route (empty to skip)?")
			c.SendLine("")
			c.ExpectString("Choose the upstream or upstream group to route to: ")
			c.SendLine("upstream-group")

			c.ExpectString("do you wish to add a prefix-rewrite transformation to the route")
			c.SendLine("n")

			c.ExpectEOF()
		}, func() {

			err := testutils.Glooctl("add route -i")
			Expect(err).NotTo(HaveOccurred())

			vs, err := helpers.MustVirtualServiceClient().Read("gloo-system", "default", clients.ReadOpts{})
			ug := vs.VirtualHost.Routes[0].GetRouteAction().GetUpstreamGroup()
			Expect(ug.GetName()).To(Equal("default"))
			Expect(ug.GetNamespace()).To(Equal("default"))
		})
	})

})
