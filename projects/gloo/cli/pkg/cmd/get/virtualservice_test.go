package get_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("VirtualService", func() {

	BeforeEach(func() {
		helpers.UseMemoryClients()
		_, err := helpers.MustKubeClient().CoreV1().Namespaces().Create(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaults.GlooSystem,
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	getVs := func() *gatewayv1.VirtualService {
		upstream := samples.SimpleUpstream()
		return &gatewayv1.VirtualService{
			Metadata: core.Metadata{
				Name:      "default",
				Namespace: defaults.GlooSystem,
			},
			VirtualHost: &gatewayv1.VirtualHost{
				Domains: []string{"*"},
				Routes: []*gatewayv1.Route{
					{
						Name: "testRouteName",
						Matchers: []*matchers.Matcher{
							{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo"}},
							{PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/bar"}},
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
			Expect(out).To(ContainSubstring(`+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+
| VIRTUAL SERVICE | DISPLAY NAME | DOMAINS | SSL  | STATUS  | LISTENERPLUGINS |             ROUTES             |
+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+
| default         |              | *       | none | Pending |                 | testRouteName: /foo, /bar ->   |
|                 |              |         |      |         |                 | gloo-system.test (upstream)    |
+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+`))
		})

		It("gets the virtual service routes", func() {
			vsc := helpers.MustVirtualServiceClient()
			_, err := vsc.Write(getVs(), clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			out, err := testutils.GlooctlOut("get vs route default")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring(`Route Action
+----+---------------+----------+-------------+-------+---------+--------------+---------+---------+
| ID |     NAME      | MATCHERS |    TYPES    | VERBS | HEADERS |    ACTION    | CUSTOM1 | CUSTOM2 |
+----+---------------+----------+-------------+-------+---------+--------------+---------+---------+
| 1  | testRouteName | /foo     | Path Prefix | *     |         | route action |
|    |               | /bar     | Path Prefix | *     |         |              |
+----+---------------+----------+-------------+-------+---------+--------------+---------+---------+`))
		})
	})
})
