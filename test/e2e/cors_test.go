package e2e_test

import (
	"net/http"
	"strings"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/gomega/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/e2e"
	gloohelpers "github.com/solo-io/gloo/test/helpers"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
)

const (
	requestACHMethods       = "Access-Control-Allow-Methods"
	requestACHOrigin        = "Access-Control-Allow-Origin"
	requestACHExposeHeaders = "Access-Control-Expose-Headers"
	corsFilterString        = `"name": "` + wellknown.CORS + `"`
	corsActiveConfigString  = `"envoy.filters.http.cors": {`

	commonHeader = "common-header"
	routeHeader  = "route-header"
	vhHeader     = "vh-header"
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

	var (
		allowedOrigins = []string{allowedOrigin}
		allowedMethods = []string{http.MethodGet, http.MethodPost}

		routeAllowedOrigins = []string{"routeAllowThisOne.solo.io"}
	)

	When("CORS is defined on VirtualHost", func() {

		When("RouteAction is Upstream", func() {
			BeforeEach(func() {
				vsWithCors := gloohelpers.NewVirtualServiceBuilder().
					WithNamespace(writeNamespace).
					WithName("vs-cors").
					WithDomain(e2e.DefaultHost).
					WithRouteActionToUpstream("route", testContext.TestUpstream().Upstream).
					WithRoutePrefixMatcher("route", "/cors").
					WithVirtualHostOptions(&gloov1.VirtualHostOptions{
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

			It("should respect CORS", func() {
				Eventually(func(g Gomega) {
					cfg, err := testContext.EnvoyInstance().ConfigDump()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(cfg).To(MatchRegexp(corsFilterString))
					g.Expect(cfg).To(MatchRegexp(corsActiveConfigString))
					g.Expect(cfg).To(MatchRegexp(allowedOrigin))
				}, "10s", ".1s").ShouldNot(HaveOccurred(), "Envoy config contains CORS filer")

				allowedOriginRequestBuilder := testContext.GetHttpRequestBuilder().
					WithOptionsMethod().
					WithPath("cors").
					WithHeader("Origin", allowedOrigins[0]).
					WithHeader("Access-Control-Request-Method", http.MethodGet).
					WithHeader("Access-Control-Request-Headers", "X-Requested-With")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(allowedOriginRequestBuilder.Build())).Should(matchers.HaveOkResponseWithHeaders(map[string]interface{}{
						requestACHMethods: MatchRegexp(strings.Join(allowedMethods, ",")),
						requestACHOrigin:  Equal(allowedOrigins[0]),
					}))
				}).Should(Succeed(), "Request with allowed origin")

				disallowedOriginRequestBuilder := allowedOriginRequestBuilder.WithHeader("Origin", unAllowedOrigin)
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(disallowedOriginRequestBuilder.Build())).Should(matchers.HaveOkResponseWithHeaders(map[string]interface{}{
						requestACHMethods: BeEmpty(),
					}))
				}).Should(Succeed(), "Request with disallowed origin")
			})

		})

		When("RouteAction is DirectResponseAction", func() {

			BeforeEach(func() {
				vs := gloohelpers.NewVirtualServiceBuilder().
					WithNamespace(writeNamespace).
					WithName(e2e.DefaultVirtualServiceName).
					WithDomain(e2e.DefaultHost).
					WithRouteDirectResponseAction("route", &gloov1.DirectResponseAction{
						Status: 200,
					}).
					WithRoutePrefixMatcher("route", "/cors").
					WithRouteOptions("route", &gloov1.RouteOptions{
						Cors: &cors.CorsPolicy{
							AllowOrigin:      allowedOrigins,
							AllowOriginRegex: allowedOrigins,
							AllowMethods:     allowedMethods,
						}}).
					Build()

				testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
					vs,
				}
			})

			It("should respect CORS", func() {
				Eventually(func(g Gomega) {
					cfg, err := testContext.EnvoyInstance().ConfigDump()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(cfg).To(MatchRegexp(corsFilterString))
					g.Expect(cfg).NotTo(MatchRegexp(corsActiveConfigString))
				}, "10s", ".1s").ShouldNot(HaveOccurred(), "Envoy config contains CORS filer")

				allowedOriginRequestBuilder := testContext.GetHttpRequestBuilder().
					WithOptionsMethod().
					WithPath("cors").
					WithHeader("Origin", allowedOrigins[0]).
					WithHeader("Access-Control-Request-Method", http.MethodGet).
					WithHeader("Access-Control-Request-Headers", "X-Requested-With")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(allowedOriginRequestBuilder.Build())).Should(matchers.HaveOkResponseWithHeaders(map[string]interface{}{
						requestACHMethods: MatchRegexp(strings.Join(allowedMethods, ",")),
						requestACHOrigin:  Equal(allowedOrigins[0]),
					}))
				}).Should(Succeed(), "Request with allowed origin")

				disallowedOriginRequestBuilder := allowedOriginRequestBuilder.WithHeader("Origin", unAllowedOrigin)
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(disallowedOriginRequestBuilder.Build())).Should(matchers.HaveOkResponseWithHeaders(map[string]interface{}{
						requestACHMethods: MatchRegexp(strings.Join(allowedMethods, ",")),
						requestACHOrigin:  Equal(allowedOrigins[0]),
					}))
				}).Should(Succeed(), "Request with disallowed origin")
			})

		})

		When("CORS is defined on RouteOptions and VirtualHostOptions without corsPolicyMergeSettings set", func() {
			BeforeEach(func() {
				vsWithCors := gloohelpers.NewVirtualServiceBuilder().
					WithNamespace(writeNamespace).
					WithName("vs-cors").
					WithDomain(e2e.DefaultHost).
					WithRouteActionToUpstream("route", testContext.TestUpstream().Upstream).
					WithRoutePrefixMatcher("route", "/cors").
					WithVirtualHostOptions(&gloov1.VirtualHostOptions{
						Cors: &cors.CorsPolicy{
							AllowOrigin:      allowedOrigins,
							AllowOriginRegex: allowedOrigins,
							AllowMethods:     allowedMethods,
							ExposeHeaders:    []string{commonHeader, vhHeader},
						}}).
					WithRouteOptions("route", &gloov1.RouteOptions{
						// We don't set allowed methods to show that we still get this from VirtualHost
						Cors: &cors.CorsPolicy{
							AllowOrigin:      routeAllowedOrigins,
							AllowOriginRegex: routeAllowedOrigins,
							ExposeHeaders:    []string{commonHeader, routeHeader},
						}}).
					Build()

				testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
					vsWithCors,
				}
			})

			It("should respect CORS", func() {
				Eventually(func(g Gomega) {
					cfg, err := testContext.EnvoyInstance().ConfigDump()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(cfg).To(MatchRegexp(corsFilterString))
					g.Expect(cfg).To(MatchRegexp(corsActiveConfigString))
					g.Expect(cfg).To(MatchRegexp(allowedOrigin))
				}, "10s", ".1s").ShouldNot(HaveOccurred(), "Envoy config contains CORS filer")

				allowedRouteOriginRequestBuilder := testContext.GetHttpRequestBuilder().
					WithOptionsMethod().
					WithPath("cors").
					WithHeader("Origin", routeAllowedOrigins[0]).
					WithHeader("Access-Control-Request-Method", http.MethodGet).
					WithHeader("Access-Control-Request-Headers", "X-Requested-With")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(allowedRouteOriginRequestBuilder.Build())).Should(matchers.HaveOkResponseWithHeaders(map[string]interface{}{
						requestACHMethods:       MatchRegexp(strings.Join(allowedMethods, ",")),
						requestACHOrigin:        Equal(routeAllowedOrigins[0]),
						requestACHExposeHeaders: Equal(strings.Join([]string{commonHeader, routeHeader}, ",")),
					}))
				}).Should(Succeed(), "Request with allowed route origin, has expose headers from route only")

				// This demonstrates that when you define options both on the VirtualHost and Route levels,
				// only the route definition is respected
				allowedVhostOriginRequestBuilder := testContext.GetHttpRequestBuilder().
					WithOptionsMethod().
					WithPath("cors").
					// use the allowed origins defined on the vhost, not the route
					WithHeader("Origin", allowedOrigins[0]).
					WithHeader("Access-Control-Request-Method", http.MethodGet).
					WithHeader("Access-Control-Request-Headers", "X-Requested-With")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(allowedVhostOriginRequestBuilder.Build())).Should(matchers.HaveOkResponseWithHeaders(map[string]interface{}{
						requestACHMethods: BeEmpty(),
					}))
				}).Should(Succeed(), "Request with allowed origin from vhost is not allowed, since route overrides it")

				disallowedOriginRequestBuilder := allowedRouteOriginRequestBuilder.WithHeader("Origin", unAllowedOrigin)
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(disallowedOriginRequestBuilder.Build())).Should(matchers.HaveOkResponseWithHeaders(map[string]interface{}{
						requestACHMethods: BeEmpty(),
					}))
				}).Should(Succeed(), "Request with disallowed origin")

				// request with disallowed method
				// shows that vhost field is respected iff route field is not set
				allowedOriginRequestBuilder := testContext.GetHttpRequestBuilder().
					WithOptionsMethod().
					WithPath("cors").
					WithHeader("Origin", routeAllowedOrigins[0]).
					WithHeader("Access-Control-Request-Method", http.MethodDelete).
					WithHeader("Access-Control-Request-Headers", "X-Requested-With")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(allowedOriginRequestBuilder.Build())).Should(matchers.HaveOkResponseWithHeaders(map[string]interface{}{
						requestACHMethods: MatchRegexp(strings.Join(allowedMethods, ",")), // show that methods are still coming through despite being on the vhost only
					}))
				}).Should(Succeed(), "Request with disallowed method via vhost")

			})

		})

		When("CORS is defined on RouteOptions and VirtualHostOptions with corsPolicyMergeSettings set", func() {
			BeforeEach(func() {
				vsWithCors := gloohelpers.NewVirtualServiceBuilder().
					WithNamespace(writeNamespace).
					WithName("vs-cors").
					WithDomain(e2e.DefaultHost).
					WithRouteActionToUpstream("route", testContext.TestUpstream().Upstream).
					WithRoutePrefixMatcher("route", "/cors").
					WithVirtualHostOptions(&gloov1.VirtualHostOptions{
						Cors: &cors.CorsPolicy{
							AllowOrigin:      allowedOrigins,
							AllowOriginRegex: allowedOrigins,
							AllowMethods:     allowedMethods,
							ExposeHeaders:    []string{commonHeader, vhHeader},
						},
						// These CorsPolicyMergeSettings should result in a policy that has all ExposeHeaders from
						// both VH and Route
						CorsPolicyMergeSettings: &cors.CorsPolicyMergeSettings{
							ExposeHeaders: cors.CorsPolicyMergeSettings_UNION,
						},
					}).
					WithRouteOptions("route", &gloov1.RouteOptions{
						// We don't set allowed methods to show that we still get this from VirtualHost
						Cors: &cors.CorsPolicy{
							AllowOrigin:      routeAllowedOrigins,
							AllowOriginRegex: routeAllowedOrigins,
							ExposeHeaders:    []string{commonHeader, routeHeader},
						}}).
					Build()

				testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
					vsWithCors,
				}
			})

			It("should respect CORS policy derived according to merge settings", func() {
				Eventually(func(g Gomega) {
					cfg, err := testContext.EnvoyInstance().ConfigDump()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(cfg).To(MatchRegexp(corsFilterString))
					g.Expect(cfg).To(MatchRegexp(corsActiveConfigString))
					g.Expect(cfg).To(MatchRegexp(allowedOrigin))
				}, "10s", ".1s").ShouldNot(HaveOccurred(), "Envoy config contains CORS filer")

				allowedRouteOriginRequestBuilder := testContext.GetHttpRequestBuilder().
					WithOptionsMethod().
					WithPath("cors").
					WithHeader("Origin", routeAllowedOrigins[0]).
					WithHeader("Access-Control-Request-Method", http.MethodGet).
					WithHeader("Access-Control-Request-Headers", "X-Requested-With")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(allowedRouteOriginRequestBuilder.Build())).Should(matchers.HaveOkResponseWithHeaders(map[string]interface{}{
						requestACHMethods: MatchRegexp(strings.Join(allowedMethods, ",")),
						requestACHOrigin:  Equal(routeAllowedOrigins[0]),
						// Expect that we have expose headers from both VH and Route
						requestACHExposeHeaders: Equal(strings.Join([]string{commonHeader, vhHeader, routeHeader}, ",")),
					}))
				}).Should(Succeed(), "Request with allowed route origin, has expose headers from both route and vh")
			})

		})

	})

	When("CORS is not defined on VirtualHost", func() {

		When("RouteAction is Upstream", func() {
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
				Eventually(func(g Gomega) {
					cfg, err := testContext.EnvoyInstance().ConfigDump()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(cfg).To(MatchRegexp(corsFilterString))
					g.Expect(cfg).NotTo(MatchRegexp(corsActiveConfigString))
				}).Should(Succeed(), "Envoy config does not contain CORS filer")
			})
		})
	})

})
