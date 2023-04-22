package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"golang.org/x/crypto/ocsp"
	"net/http"
	"time"
)

var _ = Describe("TLS e2e", func() {
	var (
		testContext           *e2e.TestContext
		rootCA, rootKey       string
		clientCert, clientKey string
		fakeOcspResponder     *helpers.FakeOcspResponder
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
		testContext.BeforeEach()

		testContext.ResourcesToCreate().Gateways = v1.GatewayList{
			gatewaydefaults.DefaultSslGateway(writeNamespace),
		}

		// create CA
		rootCA, rootKey = helpers.GetCerts(helpers.Params{
			Hosts: "ca.com",
			IsCA:  true,
		})

		// create ocsp responses
		fakeOcspResponder = helpers.NewFakeOcspResponder(helpers.GetCertificateFromString(rootCA), helpers.GetPrivateKeyFromString(rootKey))

		clientCert, clientKey = helpers.GetCerts(helpers.Params{
			Hosts:     "client.com",
			IsCA:      false,
			IssuerKey: helpers.GetPrivateKeyFromString(rootKey),
		})

		clientX509 := helpers.GetCertificateFromString(clientCert)
		ocspResponse := fakeOcspResponder.GetOcspResponse(clientX509, 60*time.Minute, false, ocsp.Response{})
		ocspResponseExpired := fakeOcspResponder.GetOcspResponse(clientX509, 1*time.Second, false, ocsp.Response{})

		secretWithoutOcspResponse := &gloov1.Secret{
			Metadata: &core.Metadata{
				Name:      "tls-no-ocsp",
				Namespace: writeNamespace,
			},
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{
					CertChain:  clientCert,
					PrivateKey: clientKey,
				},
			},
		}
		secretWithExpiredOcspResponse := &gloov1.Secret{
			Metadata: &core.Metadata{
				Name:      "tls-expired-ocsp",
				Namespace: writeNamespace,
			},
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{
					CertChain:  clientCert,
					PrivateKey: clientKey,
					OcspStaple: ocspResponseExpired,
				},
			},
		}
		secretWithValidOcspResponse := &gloov1.Secret{
			Metadata: &core.Metadata{
				Name:      "tls-valid-ocsp",
				Namespace: writeNamespace,
			},
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{
					CertChain:  clientCert,
					PrivateKey: clientKey,
					OcspStaple: ocspResponse,
				},
			},
		}

		testContext.ResourcesToCreate().Secrets = gloov1.SecretList{
			secretWithoutOcspResponse, secretWithExpiredOcspResponse, secretWithValidOcspResponse,
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

	buildVirtualService := func(sslRef *core.ResourceRef, ocspStaplePolicy ssl.SslConfig_OcspStaplePolicy) {
		customVS := helpers.NewVirtualServiceBuilder().
			WithName("client-vs").
			WithNamespace(writeNamespace).
			WithDomain("client.com").
			WithRoutePrefixMatcher(e2e.DefaultRouteName, "/").
			WithRouteActionToUpstream(e2e.DefaultRouteName, testContext.TestUpstream().Upstream).
			WithSslConfig(&ssl.SslConfig{
				OcspStaplePolicy: ocspStaplePolicy,
				SniDomains:       []string{"client.com"},
				SslSecrets: &ssl.SslConfig_SecretRef{
					SecretRef: sslRef,
				},
			}).
			Build()

		// We need to manually write a snapshot of the virtual service, because we're creating the resource during the test's run.
		// Snapshots are written in the test context's JustBeforeEach.
		err := testContext.TestClients().WriteSnapshot(testContext.Ctx(), &gloosnapshot.ApiSnapshot{
			VirtualServices: v1.VirtualServiceList{customVS},
		})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	Context("with ocsp_staple_policy set to LENIENT_STAPLING", func() {
		DescribeTable("should successfully contact upstream", func(sslRef *core.ResourceRef, expectedStatusCode int) {
			buildVirtualService(sslRef, ssl.SslConfig_LENIENT_STAPLING)

			httpClient := testutils.DefaultClientBuilder().
				WithTLSRootCa(rootCA).
				WithTLSServerName("client.com").
				Build()

			httpRequestBuilder := testContext.GetHttpsRequestBuilder().
				WithHost("client.com").
				WithPath("")

			Eventually(func(g Gomega) {
				g.Expect(httpClient.Do(httpRequestBuilder.Build())).To(matchers.HaveStatusCode(expectedStatusCode))
			}, "10s", "1s").Should(Succeed())
			Consistently(func(g Gomega) {
				g.Expect(httpClient.Do(httpRequestBuilder.Build())).To(matchers.HaveStatusCode(expectedStatusCode))
			}, "2s", ".5s").Should(Succeed())
		},
			Entry("OK with no ocsp staple", &core.ResourceRef{Name: "tls-no-ocsp", Namespace: writeNamespace}, http.StatusOK),
			Entry("OK with valid ocsp staple", &core.ResourceRef{Name: "tls-valid-ocsp", Namespace: writeNamespace}, http.StatusOK),
			Entry("OK with expired ocsp staple", &core.ResourceRef{Name: "tls-expired-ocsp", Namespace: writeNamespace}, http.StatusOK),
		)
	})

	Context("with ocsp_staple_policy set to STRICT_STAPLING", func() {
		DescribeTable("should successfully contact upstream", func(sslRef *core.ResourceRef, expectedStatusCode int) {
			buildVirtualService(sslRef, ssl.SslConfig_STRICT_STAPLING)

			httpClient := testutils.DefaultClientBuilder().
				WithTLSRootCa(rootCA).
				WithTLSServerName("client.com").
				Build()

			httpRequestBuilder := testContext.GetHttpsRequestBuilder().
				WithHost("client.com").
				WithPath("")

			Eventually(func(g Gomega) {
				g.Expect(httpClient.Do(httpRequestBuilder.Build())).To(matchers.HaveStatusCode(expectedStatusCode))
			}, "10s", "1s").Should(Succeed())
			Consistently(func(g Gomega) {
				g.Expect(httpClient.Do(httpRequestBuilder.Build())).To(matchers.HaveStatusCode(expectedStatusCode))
			}, "2s", ".5s").Should(Succeed())
		},
			Entry("with no ocsp staple", &core.ResourceRef{Name: "tls-no-ocsp", Namespace: writeNamespace}, http.StatusOK),
			Entry("with valid ocsp staple", &core.ResourceRef{Name: "tls-valid-ocsp", Namespace: writeNamespace}, http.StatusOK),
		)

		It("fails handshake with expired ocsp staple", func() {
			buildVirtualService(&core.ResourceRef{Name: "tls-expired-ocsp", Namespace: writeNamespace}, ssl.SslConfig_STRICT_STAPLING)

			httpClient := testutils.DefaultClientBuilder().
				WithTLSRootCa(rootCA).
				WithTLSServerName("client.com").
				Build()

			httpRequestBuilder := testContext.GetHttpsRequestBuilder().
				WithHost("client.com").
				WithPath("")

			Eventually(func(g Gomega) error {
				resp, err := httpClient.Do(httpRequestBuilder.Build())
				Expect(err).To(HaveOccurred())
				Expect(resp).To(BeNil())
				return err
			}, "10s", "1s").Should(MatchError(ContainSubstring("handshake failure")))
			Consistently(func(g Gomega) error {
				resp, err := httpClient.Do(httpRequestBuilder.Build())
				Expect(err).To(HaveOccurred())
				Expect(resp).To(BeNil())
				return err
			}, "2s", ".5s").Should(MatchError(ContainSubstring("handshake failure")))
		})

	})

	Context("with ocsp_staple_policy set to MUST_STAPLE", func() {
		It("fails with no ocsp staple", func() {
			buildVirtualService(&core.ResourceRef{Name: "tls-no-ocsp", Namespace: writeNamespace}, ssl.SslConfig_MUST_STAPLE)

			httpClient := testutils.DefaultClientBuilder().
				WithTLSRootCa(rootCA).
				WithTLSServerName("client.com").
				Build()

			httpRequestBuilder := testContext.GetHttpsRequestBuilder().
				WithHost("client.com").
				WithPath("")

			// TODO (fabian): figure out the proper way to test this test exactly.
			// When doing this through a pure Envoy bootstrap, Envoy fails to start. Or at the very least, resulted in many errors since `MUST_STAPLE` requires a staple.
			// Getting an EOF error (or just no specific server/client error) makes sense, as the downstream/resource would not be created.
			Eventually(func(g Gomega) error {
				resp, err := httpClient.Do(httpRequestBuilder.Build())
				Expect(err).To(HaveOccurred())
				Expect(resp).To(BeNil())
				return err
			}, "10s", "1s").Should(MatchError(ContainSubstring("EOF")))
			Consistently(func(g Gomega) error {
				resp, err := httpClient.Do(httpRequestBuilder.Build())
				Expect(err).To(HaveOccurred())
				Expect(resp).To(BeNil())
				return err
			}, "2s", ".5s").Should(MatchError(ContainSubstring("EOF")))
		})

		It("successfully contacts upstream with valid ocsp staple", func() {
			buildVirtualService(&core.ResourceRef{Name: "tls-valid-ocsp", Namespace: writeNamespace}, ssl.SslConfig_MUST_STAPLE)

			httpClient := testutils.DefaultClientBuilder().
				WithTLSRootCa(rootCA).
				WithTLSServerName("client.com").
				Build()

			httpRequestBuilder := testContext.GetHttpsRequestBuilder().
				WithHost("client.com").
				WithPath("")

			Eventually(func(g Gomega) {
				g.Expect(httpClient.Do(httpRequestBuilder.Build())).To(matchers.HaveStatusCode(http.StatusOK))
			}, "10s", "1s").Should(Succeed())
			Consistently(func(g Gomega) {
				g.Expect(httpClient.Do(httpRequestBuilder.Build())).To(matchers.HaveStatusCode(http.StatusOK))
			}, "2s", ".5s").Should(Succeed())
		})

		It("fails handshake with expired ocsp staple", func() {
			buildVirtualService(&core.ResourceRef{Name: "tls-expired-ocsp", Namespace: writeNamespace}, ssl.SslConfig_MUST_STAPLE)

			httpClient := testutils.DefaultClientBuilder().
				WithTLSRootCa(rootCA).
				WithTLSServerName("client.com").
				Build()

			httpRequestBuilder := testContext.GetHttpsRequestBuilder().
				WithHost("client.com").
				WithPath("")

			Eventually(func(g Gomega) error {
				resp, err := httpClient.Do(httpRequestBuilder.Build())
				Expect(err).To(HaveOccurred())
				Expect(resp).To(BeNil())
				return err
			}, "10s", "1s").Should(MatchError(ContainSubstring("handshake failure")))
			Consistently(func(g Gomega) error {
				resp, err := httpClient.Do(httpRequestBuilder.Build())
				Expect(err).To(HaveOccurred())
				Expect(resp).To(BeNil())
				return err
			}, "2s", ".5s").Should(MatchError(ContainSubstring("handshake failure")))
		})
	})
})
