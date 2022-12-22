package e2e_test

import (
	"bytes"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	matchers2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

var _ = Describe("Example E2E Test For Developers", func() {

	// The TestContext is a framework for writing e2e tests
	// This test provides some basic use cases to demonstrate how to leverage the framework

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		// For an individual test, we can define the environmental requirements necessary for it to succeed.
		// Ideally our tests are environment agnostic. However, if there are certain conditions that must
		// be met, you can define those here. By explicitly defining these requirements, we can error loudly
		// when they are not met. See `helpers.ValidateRequirementsAndNotifyGinkgo` for a more detailed
		// overview of this feature
		var testRequirements []helpers.Requirement

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

	Context("Using default resources", func() {
		// The TestContext creates the minimum resources necessary for e2e tests to run by default
		// Without creating any additional configuration, we have a Gateway, Virtual Service, and Upstream.
		// This means that a Proxy object is dynamically generated, and from there an xDS snapshot is computed
		// and sent to Envoy to handle traffic

		It("can route traffic", func() {
			By("GET returns 200")
			Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/", "localhost", defaults.HttpPort), nil)
				g.Expect(err).NotTo(HaveOccurred())

				// The default Virtual Service routes traffic only with a particular Host header
				req.Host = e2e.DefaultHost
				g.Expect(http.DefaultClient.Do(req)).Should(matchers.HaveOkResponse())
			}, "5s", ".5s").Should(Succeed())

			By("GET returns 404")
			Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/", "localhost", defaults.HttpPort), nil)
				g.Expect(err).NotTo(HaveOccurred())

				// The default Virtual Service routes traffic only with a particular Host header
				req.Host = fmt.Sprintf("bad-prefix-%s", e2e.DefaultHost)
				g.Expect(http.DefaultClient.Do(req)).Should(matchers.HaveStatusCode(http.StatusNotFound))
			}, "5s", ".5s").Should(Succeed())

			By("POST returns 200")
			Eventually(func(g Gomega) {
				requestBody := "some custom data"
				req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/", "localhost", defaults.HttpPort), bytes.NewBufferString(requestBody))
				g.Expect(err).NotTo(HaveOccurred())

				// The default Virtual Service routes traffic only with a particular Host header
				req.Host = e2e.DefaultHost
				g.Expect(http.DefaultClient.Do(req)).Should(matchers.HaveExactResponseBody(requestBody)) // The default server that we route to is an echo server
			}, "5s", ".5s").Should(Succeed())
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
				cfg, err := testContext.EnvoyInstance().Statistics()
				g.Expect(err).NotTo(HaveOccurred())

				// The TestContext creates a default Gateway
				gw := testContext.ResourcesToCreate().Gateways[0]

				// We expect the Envoy statistics to contain details about the listener generated from that Gateway object
				g.Expect(cfg).To(MatchRegexp(fmt.Sprintf("http.http.rds.listener-__-%d-routes.version_text", gw.GetBindPort())))
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
				WithRoutePrefixMatcher("test", "/endpoint").
				WithRouteActionToUpstream("test", testContext.TestUpstream().Upstream).
				Build()

			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				customVS,
			}
		})

		It("can route traffic", func() {
			Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/endpoint", "localhost", defaults.HttpPort), nil)
				g.Expect(err).NotTo(HaveOccurred())

				req.Host = "custom-domain.com" // to match the customVS.domains definition
				g.Expect(http.DefaultClient.Do(req)).Should(matchers.HaveOkResponse())
			}, "5s", ".5s").Should(Succeed())

		})
	})

	Context("Modifying resources directly in a test", func() {

		It("can route traffic", func() {

			By("Route traffic to /test returns a 200")
			Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), nil)
				g.Expect(err).NotTo(HaveOccurred())

				req.Host = e2e.DefaultHost
				g.Expect(http.DefaultClient.Do(req)).Should(matchers.HaveOkResponse())
			}, "5s", ".5s").Should(Succeed())

			By("Patch the VS to only handle traffic prefixed with /new")
			err := helpers.PatchResource(
				testContext.Ctx(),
				testContext.ResourcesToCreate().VirtualServices[0].GetMetadata().Ref(),
				func(resource resources.Resource) {
					vs := resource.(*v1.VirtualService)
					vs.VirtualHost.Routes[0].Matchers[0].PathSpecifier = &matchers2.Matcher_Prefix{
						Prefix: "/new",
					}
				},
				testContext.TestClients().VirtualServiceClient.BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			By("Route traffic to /test returns a 404")
			Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), nil)
				g.Expect(err).NotTo(HaveOccurred())

				req.Host = e2e.DefaultHost
				g.Expect(http.DefaultClient.Do(req)).Should(matchers.HaveStatusCode(http.StatusNotFound))
			}, "5s", ".5s").Should(Succeed())
		})

	})

})
