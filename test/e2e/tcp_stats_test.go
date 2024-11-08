package e2e_test

import (
	"github.com/solo-io/gloo/test/testutils"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/test/e2e"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("TCP Stats transport_socket", func() {

	var (
		testContext *e2e.TestContext

		secret         *gloov1.Secret
		requestBuilder *testutils.HttpRequestBuilder
		rootCACert     string
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
		testContext.BeforeEach()

		// prepare default resources
		secret = &gloov1.Secret{
			Metadata: &core.Metadata{
				Name:      "secret",
				Namespace: "default",
			},
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{
					CertChain:  gloohelpers.Certificate(),
					PrivateKey: gloohelpers.PrivateKey(),
				},
			},
		}

		testContext.ResourcesToCreate().Secrets = gloov1.SecretList{
			secret,
		}
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("HttpGateway", func() {

		Context("without any existing transport socket defined in filter chain", func() {

			BeforeEach(func() {
				requestBuilder = testContext.GetHttpRequestBuilder()
				rootCACert = ""

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gatewaydefaults.DefaultGateway(writeNamespace),
				}
			})

			Context("without outer tcp_stats transport socket wrapper", func() {

				BeforeEach(func() {
					testContext.ResourcesToCreate().Gateways[0].Options = &gloov1.ListenerOptions{
						TcpStats: &wrappers.BoolValue{Value: false},
					}
				})

				It("listener functions", func() {
					client := testutils.DefaultClientBuilder().WithTLSRootCa(rootCACert).Build()

					Eventually(func(g Gomega) {
						g.Expect(client.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponse())
					}, "15s", "1s").Should(Succeed())
				})

				It("does not emit tcp_stats", func() {
					Eventually(func(g Gomega) {
						stats, err := testContext.EnvoyInstance().Statistics()
						g.Expect(err).NotTo(HaveOccurred())

						// We expect the Envoy statistics to contain detailed TCP stats
						g.Expect(stats).ToNot(ContainSubstring("tcp_stats.cx_rx"))
					}, "5s", ".5s").Should(Succeed())
				})

			})

			Context("with outer tcp_stats transport socket wrapper", func() {

				BeforeEach(func() {
					testContext.ResourcesToCreate().Gateways[0].Options = &gloov1.ListenerOptions{
						TcpStats: &wrappers.BoolValue{Value: true},
					}
				})

				It("listener functions", func() {
					client := testutils.DefaultClientBuilder().WithTLSRootCa(rootCACert).Build()

					Eventually(func(g Gomega) {
						g.Expect(client.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponse())
					}, "15s", "1s").Should(Succeed())
				})

				It("emits tcp_stats", func() {
					Eventually(func(g Gomega) {
						stats, err := testContext.EnvoyInstance().Statistics()
						g.Expect(err).NotTo(HaveOccurred())

						// We expect the Envoy statistics to contain detailed TCP stats
						g.Expect(stats).To(ContainSubstring("tcp_stats.cx_rx"))
					}, "5s", ".5s").Should(Succeed())
				})

			})

		})

		// Any kind of preexisting transport socket can be wrapped, TLS is picked here
		// because it's used a lot, and easy to test/verify in the rig with a client.
		Context("with existing TLS transport socket defined in filter chain", func() {

			BeforeEach(func() {
				requestBuilder = testContext.GetHttpsRequestBuilder()
				rootCACert = gloohelpers.Certificate()

				secureVsToTestUpstream := gloohelpers.NewVirtualServiceBuilder().
					WithName(e2e.DefaultVirtualServiceName).
					WithNamespace(writeNamespace).
					WithDomain(e2e.DefaultHost).
					WithRoutePrefixMatcher(e2e.DefaultRouteName, "/").
					WithRouteActionToUpstream(e2e.DefaultRouteName, testContext.TestUpstream().Upstream).
					WithSslConfig(&ssl.SslConfig{
						SslSecrets: &ssl.SslConfig_SecretRef{
							SecretRef: secret.Metadata.Ref(),
						},
					}).
					Build()

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gatewaydefaults.DefaultSslGateway(writeNamespace),
				}
				testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
					secureVsToTestUpstream,
				}
			})

			Context("without outer tcp_stats transport socket wrapper", func() {

				BeforeEach(func() {
					testContext.ResourcesToCreate().Gateways[0].Options = &gloov1.ListenerOptions{
						TcpStats: &wrappers.BoolValue{Value: false},
					}
				})

				It("listener functions", func() {
					client := testutils.DefaultClientBuilder().WithTLSRootCa(rootCACert).Build()

					Eventually(func(g Gomega) {
						g.Expect(client.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponse())
					}, "15s", "1s").Should(Succeed())
				})

				It("does not emit tcp_stats", func() {
					Eventually(func(g Gomega) {
						stats, err := testContext.EnvoyInstance().Statistics()
						g.Expect(err).NotTo(HaveOccurred())

						// We expect the Envoy statistics to contain detailed TCP stats
						g.Expect(stats).ToNot(ContainSubstring("tcp_stats.cx_rx"))
					}, "5s", ".5s").Should(Succeed())
				})

			})

			Context("with outer tcp_stats transport socket wrapper", func() {

				BeforeEach(func() {
					testContext.ResourcesToCreate().Gateways[0].Options = &gloov1.ListenerOptions{
						TcpStats: &wrappers.BoolValue{Value: true},
					}
				})

				It("listener functions", func() {
					client := testutils.DefaultClientBuilder().WithTLSRootCa(rootCACert).Build()

					Eventually(func(g Gomega) {
						g.Expect(client.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponse())
					}, "15s", "1s").Should(Succeed())
				})

				It("emits tcp_stats", func() {
					Eventually(func(g Gomega) {
						stats, err := testContext.EnvoyInstance().Statistics()
						g.Expect(err).NotTo(HaveOccurred())

						// We expect the Envoy statistics to contain detailed TCP stats
						g.Expect(stats).To(ContainSubstring("tcp_stats.cx_rx"))
					}, "5s", ".5s").Should(Succeed())
				})

			})

		})

	})

})
