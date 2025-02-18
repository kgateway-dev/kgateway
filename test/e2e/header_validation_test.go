//go:build ignore

package e2e_test

import (
	"net/http"

	"github.com/kgateway-dev/kgateway/v2/test/testutils"

	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gatewayv1 "github.com/kgateway-dev/kgateway/v2/internal/gateway/pkg/api/v1"
	gloov1 "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1"
	header_validation "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1/options/header_validation"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
)

var _ = Describe("Header Validation", Label(), func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		var testRequirements []testutils.Requirement

		testContext = testContextFactory.NewTestContext(testRequirements...)
		testContext.BeforeEach()
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

	waitUntilProxyIsRunning := func() {
		// Do a GET request to make sure the proxy is running
		EventuallyWithOffset(1, func(g Gomega) {
			req := testContext.GetHttpRequestBuilder().Build()
			result, err := testutils.DefaultHttpClient.Do(req)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result).Should(matchers.HaveOkResponse())
		}, "5s", ".5s").Should(Succeed(), "GET with valid host returns a 200")
	}

	buildRequest := func(methodName string) *http.Request {
		return testContext.GetHttpRequestBuilder().
			WithMethod(methodName).
			Build()
	}

	Context("Header Validation tests", func() {
		It("rejects custom methods with default configuration", func() {
			waitUntilProxyIsRunning()
			Expect(testutils.DefaultHttpClient.Do(buildRequest("CUSTOMMETHOD"))).Should(matchers.HaveStatusCode(http.StatusBadRequest))
		})

		It("rejects standard extended methods with default configuration", func() {
			// We expect that this test will fail when UHV is enabled. By
			// default, UHV does not restrict any HTTP methods. However, even
			// when UHV is configured to restrict HTTP methods (using this
			// option):
			// https://github.com/envoyproxy/envoy/blob/release/v1.30/api/envoy/extensions/http/header_validators/envoy_default/v3/header_validator.proto#L143
			// it will still allow methods from this list:
			// https://github.com/envoyproxy/envoy/blob/0b9f67e7f71bcba3ff49575dc61676478cb68614/source/extensions/http/header_validators/envoy_default/header_validator.cc#L53-L93
			// which is substantially more methods than the original list of
			// allowed methods:
			// https://github.com/envoyproxy/envoy/blob/2970ddbd4ade787dd51dfbe605ae2e8c5d8ffcf7/source/common/http/http1/balsa_parser.cc#L54
			waitUntilProxyIsRunning()
			Expect(testutils.DefaultHttpClient.Do(buildRequest("LABEL"))).Should(matchers.HaveStatusCode(http.StatusBadRequest))
		})

		It("allows custom methods when DisableHttp1MethodValidation is set", func() {
			testContext.PatchDefaultGateway(func(gateway *gatewayv1.Gateway) *gatewayv1.Gateway {
				gateway.GatewayType = &gatewayv1.Gateway_HttpGateway{
					HttpGateway: &gatewayv1.HttpGateway{
						Options: &gloov1.HttpListenerOptions{
							HeaderValidationSettings: &header_validation.HeaderValidationSettings{
								HeaderMethodValidation: &header_validation.HeaderValidationSettings_DisableHttp1MethodValidation{},
							},
						},
					},
				}
				return gateway
			})
			testContext.EventuallyProxyAccepted()
			waitUntilProxyIsRunning()
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(buildRequest("CUSTOMMETHOD"))).Should(matchers.HaveStatusCode(http.StatusOK))
			}, "10s", "1s").Should(Succeed())

		})
	})

})
