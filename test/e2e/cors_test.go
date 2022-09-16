package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

const (
	requestACHMethods      = "Access-Control-Allow-Methods"
	requestACHOrigin       = "Access-Control-Allow-Origin"
	corsFilterString       = `"name": "` + wellknown.CORS + `"`
	corsActiveConfigString = `"cors":`
)

var _ = Describe("CORS", func() {

	var (
		err           error
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		testUpstream  *v1helpers.TestUpstream

		resourcesToCreate *gloosnapshot.ApiSnapshot

		writeNamespace = defaults.GlooSystem
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()

		// run gloo
		writeNamespace = defaults.GlooSystem
		ro := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableFds: true,
				DisableUds: true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		// run envoy
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.RunWithRole(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		// this is the upstream that will handle requests
		testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		// The set of resources that these tests will generate
		resourcesToCreate = &gloosnapshot.ApiSnapshot{
			Gateways: gatewayv1.GatewayList{
				gatewaydefaults.DefaultGateway(writeNamespace),
			},
			Upstreams: gloov1.UpstreamList{
				testUpstream.Upstream,
			},
		}
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	JustBeforeEach(func() {
		// Create Resources
		err = testClients.WriteSnapshot(ctx, resourcesToCreate)
		Expect(err).NotTo(HaveOccurred())

		// Wait for a proxy to be accepted
		gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
		})
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
				WithRouteActionToUpstream("route", testUpstream.Upstream).
				WithRoutePrefixMatcher("route", "/cors").
				WithRouteOptions("route", &gloov1.RouteOptions{
					Cors: &cors.CorsPolicy{
						AllowOrigin:      allowedOrigins,
						AllowOriginRegex: allowedOrigins,
						AllowMethods:     allowedMethods,
					}}).
				Build()

			resourcesToCreate.VirtualServices = gatewayv1.VirtualServiceList{
				vsWithCors,
			}

		})

		It("should run with cors", func() {

			By("Envoy config contains CORS filer")
			Eventually(func(g Gomega) {
				cfg, err := envoyInstance.EnvoyConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(cfg).To(MatchRegexp(corsFilterString))
				g.Expect(cfg).To(MatchRegexp(corsActiveConfigString))
				g.Expect(cfg).To(MatchRegexp(allowedOrigin))
			}, "10s", ".1s").ShouldNot(HaveOccurred())

			preFlightRequest, err := http.NewRequest("OPTIONS", fmt.Sprintf("http://%s:%d/cors", envoyInstance.LocalAddr(), defaults.HttpPort), nil)
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
				WithRouteActionToUpstream("route", testUpstream.Upstream).
				WithRoutePrefixMatcher("route", "/cors").
				Build()

			resourcesToCreate.VirtualServices = gatewayv1.VirtualServiceList{
				vsWithoutCors,
			}
		})

		It("should run without cors", func() {
			By("Envoy config does not contain CORS filer")
			Eventually(func(g Gomega) {
				cfg, err := envoyInstance.EnvoyConfigDump()
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
