package e2e_test

import (
	"net/http"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/gomega/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	header_validation "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/header_validation"
	"github.com/solo-io/gloo/test/e2e"
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
		Eventually(func(g Gomega) {
			req := testContext.GetHttpRequestBuilder().Build()
			result, err := testutils.DefaultHttpClient.Do(req)
			g.ExpectWithOffset(1, err).NotTo(HaveOccurred())
			g.ExpectWithOffset(1, result).Should(matchers.HaveOkResponse())
		}, "5s", ".5s").Should(Succeed(), "GET with valid host returns a 200")
	}

	buildRequest := func() *http.Request {
		return testContext.GetHttpRequestBuilder().
			WithMethod("CUSTOMMETHOD").
			Build()
	}

	Context("Using default configuration", func() {
		It("defaults to returning HTTP 400 on requests with custom HTTP methods", func() {
			waitUntilProxyIsRunning()
			req := buildRequest()
			Expect(testutils.DefaultHttpClient.Do(req)).Should(matchers.HaveStatusCode(http.StatusBadRequest))
		})
	})

	Context("With header validation disabled", func() {

		It("custom HTTP methods are allowed", func() {
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
			  g.Expect(testutils.DefaultHttpClient.Do(buildRequest())).Should(matchers.HaveStatusCode(http.StatusOK))
			}, "10s", "1s").Should(Succeed())

		})
	})

})
