package get_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
)

var _ = Describe("VirtualService", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	getVs := func() *gatewayv1.VirtualService {
		upstream := &gloov1.Upstream{
			Metadata: core.Metadata{
				Namespace: defaults.GlooSystem,
				Name:      "us",
			},
		}

		return &gatewayv1.VirtualService{
			Metadata: core.Metadata{
				Name:      "default",
				Namespace: defaults.GlooSystem,
			},
			VirtualHost: &gatewayv1.VirtualHost{
				Domains: []string{"*"},
				Routes: []*gatewayv1.Route{
					{
						Matcher: &gloov1.Matcher{
							PathSpecifier: &gloov1.Matcher_Prefix{Prefix: "/"},
						},
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
										},
									},
								},
							},
						},
					},
				},
			},
		}
	}

	Context("Prints virtual services with table formatting", func() {

		It("gets the virtual service", func() {
			vsc := helpers.MustVirtualServiceClient()
			_, err := vsc.Write(getVs(), clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			out, err := testutils.GlooctlOut("get vs default")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal(`+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+
| VIRTUAL SERVICE | DISPLAY NAME | DOMAINS | SSL  | STATUS  | LISTENERPLUGINS |             ROUTES             |
+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+
| default         |              | *       | none | Pending |                 | / -> gloo-system.us (upstream) |
+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+`))
		})

		It("gets the virtual service", func() {
			vsc := helpers.MustVirtualServiceClient()
			_, err := vsc.Write(getVs(), clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			out, err := testutils.GlooctlOut("get vs route default")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal(`Route Action
+----+---------+-------------+------+--------+--------------+---------+---------+
| ID | MATCHER |    TYPE     | VERB | HEADER |    ACTION    | CUSTOM1 | CUSTOM2 |
+----+---------+-------------+------+--------+--------------+---------+---------+
| 1  | /       | Path Prefix | *    |        | route action |
+----+---------+-------------+------+--------+--------------+---------+---------+`))
		})
	})
})
