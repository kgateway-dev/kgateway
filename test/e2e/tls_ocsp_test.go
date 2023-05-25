package e2e_test

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"golang.org/x/crypto/ocsp"
)

var _ = Describe("TLS OCSP e2e", func() {
	var (
		testContext              *e2e.TestContext
		clientCert, clientKey    string
		fakeOcspResponder        *helpers.FakeOcspResponder
		tlsSecretWithNoOcsp      = &core.Metadata{Name: "tls-no-ocsp", Namespace: writeNamespace}
		tlsSecretWithOcsp        = &core.Metadata{Name: "tls-with-ocsp", Namespace: writeNamespace}
		tlsSecretWithExpiredOcsp = &core.Metadata{Name: "tls-with-expired-ocsp", Namespace: writeNamespace}
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
		testContext.BeforeEach()

		// Create SSL Gateway
		testContext.ResourcesToCreate().Gateways = v1.GatewayList{
			gatewaydefaults.DefaultSslGateway(writeNamespace),
		}

		rootCaX509 := helpers.GetCertificateFromString(helpers.Certificate())
		rootKeyRSA := helpers.GetPrivateKeyRSAFromString(helpers.PrivateKey())

		// create ocsp responses
		fakeOcspResponder = helpers.NewFakeOcspResponder(rootCaX509, rootKeyRSA)

		// Generate certificates signed by root CA
		clientCert, clientKey = helpers.GetCerts(helpers.Params{
			Hosts:     e2e.DefaultHost,
			IsCA:      false,
			IssuerKey: rootKeyRSA,
		})

		clientX509 := helpers.GetCertificateFromString(clientCert)

		// Generate ocsp responses for the client certificate
		ocspResponse := fakeOcspResponder.GetOcspResponse(clientX509, 60*time.Minute, false, ocsp.Response{})
		ocspResponseExpired := fakeOcspResponder.GetOcspResponse(clientX509, 0, false, ocsp.Response{})

		secretWithoutOcspResponse := &gloov1.Secret{
			Metadata: tlsSecretWithNoOcsp,
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{
					CertChain:  clientCert,
					PrivateKey: clientKey,
				},
			},
		}
		secretWithExpiredOcspResponse := &gloov1.Secret{
			Metadata: tlsSecretWithExpiredOcsp,
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{
					CertChain:  clientCert,
					PrivateKey: clientKey,
					OcspStaple: ocspResponseExpired,
				},
			},
		}
		secretWithValidOcspResponse := &gloov1.Secret{
			Metadata: tlsSecretWithOcsp,
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

	eventualConsistent := func(fn func(g Gomega)) {
		// Setting an offset of 2 as this function is called from within a helper function
		EventuallyWithOffset(2, fn, "10s", "1s").Should(Succeed())
		ConsistentlyWithOffset(2, fn, "2s", ".5s").Should(Succeed())
	}

	// expectConsistentResponseStatus expects a response status to be consistent.
	expectConsistentResponseStatus := func(httpClient *http.Client, httpRequestBuilder *testutils.HttpRequestBuilder, expectedStatusCode int) {
		eventualConsistent(func(g Gomega) {
			resp, err := httpClient.Do(httpRequestBuilder.Build())
			g.ExpectWithOffset(1, err).NotTo(HaveOccurred())
			g.ExpectWithOffset(1, resp).To(matchers.HaveStatusCode(expectedStatusCode))
		})
	}

	// expectConsistentError expects an error to happen consistently.
	// If expectedError is empty, it expects any error to occur. If set, it expects the error to contain the expectedError string.
	expectConsistentError := func(httpClient *http.Client, httpRequestBuilder *testutils.HttpRequestBuilder, expectedError string) {
		eventualConsistent(func(g Gomega) {
			_, err := httpClient.Do(httpRequestBuilder.Build())
			g.ExpectWithOffset(1, err).To(HaveOccurred())
			if expectedError != "" {
				g.ExpectWithOffset(1, err).To(MatchError(ContainSubstring(expectedError)))
			}
		})
	}

	// updateVirtualService updates the default virtual service with the given sslRef and ocspStaplePolicy.
	updateVirtualService := func(sslRef *core.ResourceRef, ocspStaplePolicy ssl.SslConfig_OcspStaplePolicy) {
		testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
			vsBuilder := helpers.BuilderFromVirtualService(vs)
			vsBuilder.
				WithSslConfig(&ssl.SslConfig{
					OcspStaplePolicy: ocspStaplePolicy,
					SslSecrets: &ssl.SslConfig_SecretRef{
						SecretRef: sslRef,
					},
				})
			return vsBuilder.Build()
		})
	}

	// buildHttpsRequestClient builds an http client and request builder.
	// It uses default values for all fields, as the default is used for all tests.
	buildHttpsRequestClient := func() (*http.Client, *testutils.HttpRequestBuilder) {
		httpClient := testutils.DefaultClientBuilder().
			WithTLSRootCa(helpers.Certificate()).
			WithTLSServerName(e2e.DefaultHost).
			Build()

		httpRequestBuilder := testContext.GetHttpsRequestBuilder()

		return httpClient, httpRequestBuilder
	}

	Context("with OCSP Staple Policy set to LENIENT_STAPLING", func() {
		DescribeTable("should successfully contact upstream", func(sslRef *core.ResourceRef, expectedStatusCode int) {
			updateVirtualService(sslRef, ssl.SslConfig_LENIENT_STAPLING)
			httpClient, httpRequestBuilder := buildHttpsRequestClient()

			expectConsistentResponseStatus(httpClient, httpRequestBuilder, expectedStatusCode)
		},
			Entry("OK with no ocsp staple", tlsSecretWithNoOcsp.Ref(), http.StatusOK),
			Entry("OK with valid ocsp staple", tlsSecretWithOcsp.Ref(), http.StatusOK),
			Entry("OK with expired ocsp staple", tlsSecretWithExpiredOcsp.Ref(), http.StatusOK),
		)
	})

	Context("with  OCSP Staple Policy set to STRICT_STAPLING", func() {
		DescribeTable("should successfully contact upstream", func(sslRef *core.ResourceRef, expectedStatusCode int) {
			updateVirtualService(sslRef, ssl.SslConfig_STRICT_STAPLING)
			httpClient, httpRequestBuilder := buildHttpsRequestClient()

			expectConsistentResponseStatus(httpClient, httpRequestBuilder, expectedStatusCode)
		},
			Entry("with no ocsp staple", tlsSecretWithNoOcsp.Ref(), http.StatusOK),
			Entry("with valid ocsp staple", tlsSecretWithOcsp.Ref(), http.StatusOK),
		)

		It("fails handshake with expired ocsp staple", func() {
			updateVirtualService(tlsSecretWithExpiredOcsp.Ref(), ssl.SslConfig_STRICT_STAPLING)
			httpClient, httpRequestBuilder := buildHttpsRequestClient()

			expectConsistentError(httpClient, httpRequestBuilder, "handshake failure")
		})

	})

	Context("with  OCSP Staple Policy set to MUST_STAPLE", func() {
		It("fails with no ocsp staple", func() {
			updateVirtualService(tlsSecretWithNoOcsp.Ref(), ssl.SslConfig_MUST_STAPLE)
			httpClient, httpRequestBuilder := buildHttpsRequestClient()

			// TODO (fabian): figure out the proper way to test this test exactly.
			// When doing this through an Envoy bootstrap Envoy fails to start, or at least resulted in many errors logs in Envoy, since the `MUST_STAPLE` requires an ocsp staple.
			// Getting an error makes sense, as the downstream/resource would, at the very least, not be created.
			// The specific error is nondeterministic. Locally, it was `EOF`, but in CI there was a `SyscallError`.
			expectConsistentError(httpClient, httpRequestBuilder, "")
		})

		It("successfully contacts upstream with valid ocsp staple", func() {
			updateVirtualService(tlsSecretWithOcsp.Ref(), ssl.SslConfig_MUST_STAPLE)
			httpClient, httpRequestBuilder := buildHttpsRequestClient()

			expectConsistentResponseStatus(httpClient, httpRequestBuilder, http.StatusOK)
		})

		It("fails handshake with expired ocsp staple", func() {
			updateVirtualService(tlsSecretWithExpiredOcsp.Ref(), ssl.SslConfig_MUST_STAPLE)
			httpClient, httpRequestBuilder := buildHttpsRequestClient()

			expectConsistentError(httpClient, httpRequestBuilder, "handshake failure")
		})
	})
})
