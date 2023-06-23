package e2e_test

import (
	"fmt"
	"net/http"

	"github.com/solo-io/gloo/test/services"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/gomega/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"
)

var _ = FDescribe("Example E2E Test For Developers", Label(), func() {

	// The TestContext is a framework for writing e2e tests
	// This test provides some basic use cases to demonstrate how to leverage the framework

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		// For an individual test, we can define the environmental requirements necessary for it to succeed.
		// Ideally our tests are environment agnostic. However, if there are certain conditions that must
		// be met, you can define those here. By explicitly defining these requirements, we can error loudly
		// when they are not met. See `testutils.ValidateRequirementsAndNotifyGinkgo` for a more detailed
		// overview of this feature
		var testRequirements []testutils.Requirement

		testContext = testContextFactory.NewTestContext(testRequirements...)
		testContext.BeforeEach()

		testContext.SetRunServices(services.What{
			DisableUds: false,
		})
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

	Context("Using default resources", func() {
		// The TestContext creates the minimum resources necessary for e2e tests to run by default
		// Without creating any additional configuration, we have a Gateway, Virtual Service, and Upstream.
		// This means that a Proxy object is dynamically generated, and from there an xDS snapshot is computed
		// and sent to Envoy to handle traffic

		It("can route traffic", func() {
			Eventually(func(g Gomega) {
				req := testContext.GetHttpRequestBuilder().Build()
				g.Expect(testutils.DefaultHttpClient.Do(req)).Should(matchers.HaveOkResponse())
			}, "5s", ".5s").Should(Succeed(), "GET with valid host returns a 200")

			Eventually(func(g Gomega) {
				req := testContext.GetHttpRequestBuilder().
					WithHost("invalid-host").
					Build()
				g.Expect(testutils.DefaultHttpClient.Do(req)).Should(matchers.HaveStatusCode(http.StatusNotFound))
			}, "5s", ".5s").Should(Succeed(), "GET with invalid host returns a 404")

			requestBody := "some custom data"
			requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(requestBody)
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveExactResponseBody(requestBody)) // The default server that we route to is an echo server
			}, "5s", ".5s").Should(Succeed(), "POST with request body should return same body in response")

			Expect(1).To(Equal(2))
		})

		It("can access envoy config dump", func() {
			Eventually(func(g Gomega) {
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				// The TestContext creates a default VirtualService
				vs := testContext.ResourcesToCreate().VirtualServices[0]

				// We expect the envoy configuration to contain these properties in the configuration dump
				g.Expect(cfg).To(MatchRegexp(fmt.Sprintf("%v", vs.GetVirtualHost().GetDomains())))
				g.Expect(cfg).To(MatchRegexp(vs.GetMetadata().GetName()))
			}, "5s", ".5s").Should(Succeed())
		})

		It("can access statistics", func() {
			Eventually(func(g Gomega) {
				stats, err := testContext.EnvoyInstance().Statistics()
				g.Expect(err).NotTo(HaveOccurred())

				// The TestContext creates a default Gateway
				gw := testContext.ResourcesToCreate().Gateways[0]

				// We expect the Envoy statistics to contain details about the listener generated from that Gateway object
				g.Expect(stats).To(MatchRegexp(fmt.Sprintf("http.http.rds.listener-__-%d-routes.version_text", gw.GetBindPort())))
			}, "5s", ".5s").Should(Succeed())
		})
	})

	Context("Using custom resources", func() {

		BeforeEach(func() {
			// We can modify the resources that tests will use in the BeforeEach
			customVS := helpers.NewVirtualServiceBuilder().
				WithName("my-custom-vs").
				WithNamespace(writeNamespace).
				WithDomain("custom-domain.com").
				WithRoutePrefixMatcher(e2e.DefaultRouteName, "/endpoint").
				WithRouteActionToUpstream(e2e.DefaultRouteName, testContext.TestUpstream().Upstream).
				Build()

			// By including the new resource in the ResourcesToCreate variable, the TestContext
			// persists this resource for us during the JustBeforeEach
			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				customVS,
			}
		})

		It("can route traffic", func() {
			requestBuilder := testContext.GetHttpRequestBuilder().
				WithHost("custom-domain.com"). // to match the customVS.domains definition
				WithPath("endpoint")           // to match the customVS.route prefix match definition

			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())
			}, "5s", ".5s").Should(Succeed())
			Consistently(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())
			}, "2s", ".5s").Should(Succeed())
		})
	})

	Context("Modifying resources directly in a test", func() {

		It("can route traffic", func() {
			requestBuilder := testContext.GetHttpRequestBuilder().WithPath("test")

			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())
			}, "5s", ".5s").Should(Succeed(), "traffic to /test eventually returns a 200")
			Consistently(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())
			}, "2s", ".5s").Should(Succeed(), "traffic to /test consistently returns a 200")

			By("Patch the VS to only handle traffic prefixed with /new")
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithRoutePrefixMatcher(e2e.DefaultRouteName, "/new")
				return vsBuilder.Build()
			})

			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveStatusCode(http.StatusNotFound))
			}, "5s", ".5s").Should(Succeed(), "traffic to /test eventually returns a 404")
			Consistently(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveStatusCode(http.StatusNotFound))
			}, "2s", ".5s").Should(Succeed(), "traffic to /test consistently returns a 404")
		})

	})

})
