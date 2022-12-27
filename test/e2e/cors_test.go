package e2e_test

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/solo-io/gloo/test/e2e"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloohelpers "github.com/solo-io/gloo/test/helpers"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

const (
	requestACHMethods      = "Access-Control-Allow-Methods"
	requestACHOrigin       = "Access-Control-Allow-Origin"
	corsFilterString       = `"name": "` + wellknown.CORS + `"`
	corsActiveConfigString = `"cors":`
)

var _ = Describe("CORS", func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
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

	Context("With CORS", func() {

		var (
			allowedOrigins = []string{allowedOrigin}
			allowedMethods = []string{"GET", "POST"}
		)

		BeforeEach(func() {
			vsWithCors := gloohelpers.NewVirtualServiceBuilder().WithNamespace(writeNamespace).
				WithName("vs-cors").
				WithDomain(allowedOrigin).
				WithRouteActionToUpstream("route", testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher("route", "/cors").
				WithRouteOptions("route", &gloov1.RouteOptions{
					Cors: &cors.CorsPolicy{
						AllowOrigin:      allowedOrigins,
						AllowOriginRegex: allowedOrigins,
						AllowMethods:     allowedMethods,
					}}).
				Build()

			testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
				vsWithCors,
			}
		})

		It("should run with cors", func() {

			By("Envoy config contains CORS filer")
			Eventually(func(g Gomega) {
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(cfg).To(MatchRegexp(corsFilterString))
				g.Expect(cfg).To(MatchRegexp(corsActiveConfigString))
				g.Expect(cfg).To(MatchRegexp(allowedOrigin))
			}, "10s", ".1s").ShouldNot(HaveOccurred())

			preFlightRequest, err := http.NewRequest("OPTIONS", fmt.Sprintf("http://%s:%d/cors", testContext.EnvoyInstance().LocalAddr(), defaults.HttpPort), nil)
			Expect(err).NotTo(HaveOccurred())
			preFlightRequest.Host = allowedOrigin

			By("Request with allowed origin")
			Eventually(func(g Gomega) {
				headers := executeRequestWithAccessControlHeaders(preFlightRequest, allowedOrigins[0], "GET")
				v, ok := headers[requestACHMethods]
				g.Expect(ok).To(BeTrue())
				g.Expect(strings.Split(v[0], ",")).Should(ConsistOf(allowedMethods))

				v, ok = headers[requestACHOrigin]
				g.Expect(ok).To(BeTrue())
				g.Expect(len(v)).To(Equal(1))
				g.Expect(v[0]).To(Equal(allowedOrigins[0]))
			}).ShouldNot(HaveOccurred())

			By("Request with disallowed origin")
			Eventually(func(g Gomega) {
				headers := executeRequestWithAccessControlHeaders(preFlightRequest, unAllowedOrigin, "GET")
				_, ok := headers[requestACHMethods]
				g.Expect(ok).To(BeFalse())
			}).ShouldNot(HaveOccurred())
		})

	})

	Context("Without CORS", func() {

		BeforeEach(func() {
			vsWithoutCors := gloohelpers.NewVirtualServiceBuilder().WithNamespace(writeNamespace).
				WithName("vs-cors").
				WithDomain("cors.com").
				WithRouteActionToUpstream("route", testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher("route", "/cors").
				Build()

			testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
				vsWithoutCors,
			}
		})

		It("should run without cors", func() {
			By("Envoy config does not contain CORS filer")
			Eventually(func(g Gomega) {
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(cfg).To(MatchRegexp(corsFilterString))
				g.Expect(cfg).NotTo(MatchRegexp(corsActiveConfigString))
			}).ShouldNot(HaveOccurred())
		})
	})

})

func executeRequestWithAccessControlHeaders(req *http.Request, origin, method string) http.Header {
	h := http.Header{}
	Eventually(func(g Gomega) {
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", method)
		req.Header.Set("Access-Control-Request-Headers", "X-Requested-With")

		resp, err := http.DefaultClient.Do(req)
		g.Expect(err).NotTo(HaveOccurred())

		defer resp.Body.Close()
		h = resp.Header
	}).ShouldNot(HaveOccurred())
	return h
}
