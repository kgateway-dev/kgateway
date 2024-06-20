package e2e_test

import (
	"net/http"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/gomega/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
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

	Context("Using default resources", func() {
		// The TestContext creates the minimum resources necessary for e2e tests to run by default
		// Without creating any additional configuration, we have a Gateway, Virtual Service, and Upstream.
		// This means that a Proxy object is dynamically generated, and from there an xDS snapshot is computed
		// and sent to Envoy to handle traffic

		It("defaults to returning HTTP 400 on requests with custom HTTP methods", func() {
			waitUntilProxyIsRunning()
			req := buildRequest()
			Expect(testutils.DefaultHttpClient.Do(req)).Should(matchers.HaveStatusCode(http.StatusBadRequest))
		})
	})

	Context("With header_validation set to false", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				HeaderValidationSettings: &header_validation.HeaderValidationSettings{
					CustomMethods: &header_validation.HeaderValidationSettings_Deny{},
				},
			}
			testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{gw}
		})

		It("allows custom HTTP methods when enabled", func() {
			waitUntilProxyIsRunning()
			req := buildRequest()
			Expect(testutils.DefaultHttpClient.Do(req)).Should(matchers.HaveStatusCode(http.StatusOK))
		})
	})

})
