package e2e_test

import (
	"net/http"

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

var (
	// https://www.haproxy.org/download/1.9/doc/proxy-protocol.txt
	proxyProtocolBytes = []byte("PROXY TCP4 1.2.3.4 1.2.3.4 123 123\r\n")
)

var _ = Describe("Proxy Protocol", func() {

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

		Context("without TLS", func() {

			BeforeEach(func() {
				requestBuilder = testContext.GetHttpRequestBuilder()
				rootCACert = ""

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gatewaydefaults.DefaultGateway(writeNamespace),
				}
			})

			Context("without PROXY protocol", func() {

				BeforeEach(func() {
					testContext.ResourcesToCreate().Gateways[0].UseProxyProto = &wrappers.BoolValue{Value: false}
				})

				It("works", func() {
					client := getHttpClientWithoutProxyProtocol(rootCACert)

					Eventually(func(g Gomega) {
						g.Expect(client.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponse())
					}, "15s", "1s").Should(Succeed())
				})

			})

			Context("with PROXY protocol", func() {

				BeforeEach(func() {
					testContext.ResourcesToCreate().Gateways[0].UseProxyProto = &wrappers.BoolValue{Value: true}
				})

				It("works", func() {
					client := getHttpClientWithProxyProtocol(rootCACert, proxyProtocolBytes)

					Eventually(func(g Gomega) {
						g.Expect(client.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponse())
					}, "15s", "1s").Should(Succeed())
				})

			})

		})

		Context("with TLS", func() {

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

			Context("without PROXY protocol", func() {

				BeforeEach(func() {
					testContext.ResourcesToCreate().Gateways[0].UseProxyProto = &wrappers.BoolValue{Value: false}
				})

				It("works", func() {
					client := getHttpClientWithoutProxyProtocol(rootCACert)

					Eventually(func(g Gomega) {
						g.Expect(client.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponse())
					}, "15s", "1s").Should(Succeed())
				})

			})

			Context("with PROXY protocol", func() {

				BeforeEach(func() {
					testContext.ResourcesToCreate().Gateways[0].UseProxyProto = &wrappers.BoolValue{Value: true}
				})

				It("works", func() {
					client := getHttpClientWithProxyProtocol(rootCACert, proxyProtocolBytes)

					Eventually(func(g Gomega) {
						g.Expect(client.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponse())
					}, "15s", "1s").Should(Succeed())
				})

			})

			Context("with PROXY protocol and SNI", func() {

				BeforeEach(func() {
					testContext.ResourcesToCreate().Gateways[0].UseProxyProto = &wrappers.BoolValue{Value: true}

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
							SniDomains: []string{"gateway-proxy"},
						}).
						Build()

					testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
						secureVsToTestUpstream,
					}
				})

				It("works", func() {
					client := getHttpClientWithProxyProtocol(rootCACert, proxyProtocolBytes)

					Eventually(func(g Gomega) {
						g.Expect(client.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponse())
					}, "15s", "1s").Should(Succeed())
				})

			})

		})

	})

})

func getHttpClientWithoutProxyProtocol(rootCACert string) *http.Client {
	return testutils.DefaultClientBuilder().WithTLSRootCa(rootCACert).Build()
}

func getHttpClientWithProxyProtocol(rootCACert string, proxyProtocolBytes []byte) *http.Client {
	return testutils.DefaultClientBuilder().
		WithTLSRootCa(rootCACert).
		WithProxyProtocolBytes(proxyProtocolBytes).
		Build()
}
