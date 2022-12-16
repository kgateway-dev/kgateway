package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/test/helpers"
	matchers2 "github.com/solo-io/gloo/test/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	fault "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ = Describe("Fault Injection", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		envoyInstance *services.EnvoyInstance

		testClients  services.TestClients
		testUpstream *v1helpers.TestUpstream

		resourcesToCreate *gloosnapshot.ApiSnapshot
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Run gloo
		ro := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableFds: true,
				DisableUds: true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		// Run Envoy
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.RunWithRole(envoyRole, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		// The upstream that will handle requests
		testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		// The set of resources that these tests will generate
		resourcesToCreate = &gloosnapshot.ApiSnapshot{
			Gateways: v1.GatewayList{
				gwdefaults.DefaultGateway(writeNamespace),
			},
			VirtualServices: v1.VirtualServiceList{},
			Upstreams: gloov1.UpstreamList{
				testUpstream.Upstream,
			},
		}

	})

	AfterEach(func() {
		// Stop Envoy
		envoyInstance.Clean()

		cancel()
	})

	JustBeforeEach(func() {
		// Create Resources
		err := testClients.WriteSnapshot(ctx, resourcesToCreate)
		Expect(err).NotTo(HaveOccurred())

		// Wait for a proxy to be accepted
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(writeNamespace, gwdefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
		})
	})

	JustAfterEach(func() {
		// We do not need to clean up the Snapshot that was written in the JustBeforeEach
		// That is because each test uses its own InMemoryCache
	})

	Context("Envoy Abort Fault", func() {

		BeforeEach(func() {
			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithRoutePrefixMatcher("test", "/").
				WithRouteOptions("test", &gloov1.RouteOptions{
					Faults: &fault.RouteFaults{
						Abort: &fault.RouteAbort{
							HttpStatus: uint32(503),
							Percentage: float32(100),
						},
					},
				}).
				WithRouteActionToUpstream("test", testUpstream.Upstream).
				Build()

			resourcesToCreate.VirtualServices = v1.VirtualServiceList{
				vs,
			}
		})

		It("works", func() {
			client := &http.Client{}
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/", "localhost", defaults.HttpPort), nil)
			Expect(err).NotTo(HaveOccurred())
			req.Host = "test.com" // to match the vs-test

			Eventually(func(g Gomega) (*http.Response, error) {
				return client.Do(req)
			}, "5s", ".5s").Should(matchers2.MatchHttpResponse(&http.Response{
				StatusCode: http.StatusServiceUnavailable,
			}))

		})
	})

	Context("Envoy Delay Fault", func() {

		BeforeEach(func() {
			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithRoutePrefixMatcher("test", "/").
				WithRouteOptions("test", &gloov1.RouteOptions{
					Faults: &fault.RouteFaults{
						Delay: &fault.RouteDelay{
							FixedDelay: prototime.DurationToProto(time.Second * 3),
							Percentage: float32(100),
						},
					},
				}).
				WithRouteActionToUpstream("test", testUpstream.Upstream).
				Build()

			resourcesToCreate.VirtualServices = v1.VirtualServiceList{
				vs,
			}
		})

		It("works", func() {
			client := &http.Client{}
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/", "localhost", defaults.HttpPort), nil)
			Expect(err).NotTo(HaveOccurred())
			req.Host = "test.com" // to match the vs-test

			Eventually(func(g Gomega) *http.Response {
				start := time.Now()
				response, err := client.Do(req)
				g.Expect(err).NotTo(HaveOccurred())

				elapsed := time.Now().Sub(start)
				// This test regularly flakes, and the error is usually of the form:
				// "Elapsed time 2.998280684s not longer than delay 3s"
				// There's a small precision issue when communicating with Envoy, so including a small
				// margin of error to eliminate the test flake.
				marginOfError := 100 * time.Millisecond
				g.Expect(elapsed + marginOfError).To(BeNumerically(">", 3*time.Second))

				return response
			}, "20s", ".1s").Should(matchers2.MatchHttpResponse(&http.Response{
				StatusCode: http.StatusOK,
			}))

		})
	})
})
