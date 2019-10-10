package get_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Get RouteTable", func() {
	BeforeEach(func() {
		helpers.UseMemoryClients()
	})
	AfterEach(func() {
		helpers.UseDefaultClients()
	})
	It("gets the route table list", func() {
		rt := helpers.MustRouteTableClient()
		_, rts := samples.LinkedRouteTablesWithVirtualService("vs", defaults.GlooSystem)
		_, err := rt.Write(rts[0], clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		out, err := testutils.GlooctlOut("get rt")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(`+-------------+--------------------------------+---------+
| ROUTE TABLE |             ROUTES             | STATUS  |
+-------------+--------------------------------+---------+
| node-0      | /root/0 -> gloo-system.node-1  | Pending |
|             | (route table)                  |         |
+-------------+--------------------------------+---------+`))
	})
})
