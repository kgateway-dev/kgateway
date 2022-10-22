package e2e_test

import (
	"context"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	envoytrace_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/trace/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tracing"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/test/services"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var _ = Describe("Zipkin config loading", func() {

	// Non-functional change to trigger CI (#3)

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		envoyInstance *services.EnvoyInstance
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	Context("Tracing defined on Envoy bootstrap", func() {

		It("should send trace msgs to the zipkin server", func() {
			err := envoyInstance.RunWithConfigFile(int(defaults.HttpPort), "./envoyconfigs/zipkin-envoy-conf.yaml")
			Expect(err).NotTo(HaveOccurred())

			// Start a dummy server listening on 9411 for Zipkin requests
			apiHit := make(chan bool, 1)
			zipkinHandler := http.NewServeMux()
			zipkinHandler.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v2/spans")) // Zipkin json collector API
				fmt.Fprintf(w, "Dummy Zipkin Collector received request on - %q", html.EscapeString(r.URL.Path))
				apiHit <- true
			}))
			startCancellableZipkinServer(ctx, ":9411", zipkinHandler)

			// Execute a request against the admin endpoint, as this should result in a trace
			testRequest := createRequestWithTracingEnabled("127.0.0.1", 11082)
			Eventually(testRequest, 15, 1).Should(ContainSubstring(`<title>Envoy Admin</title>`))

			truez := true
			Eventually(apiHit, 5*time.Second).Should(Receive(&truez))
		})

	})

	Context("Tracing defined on Gloo resources", func() {

		var (
			testClients  services.TestClients
			testUpstream *v1helpers.TestUpstream

			resourcesToCreate *gloosnapshot.ApiSnapshot

			writeNamespace = defaults.GlooSystem

			zipkinApiHit chan bool
		)

		BeforeEach(func() {
			// run gloo
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
			err := envoyInstance.RunWithRole(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort)
			Expect(err).NotTo(HaveOccurred())

			// this is the upstream that will handle requests
			testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

			vsToTestUpstream := gloohelpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithRoutePrefixMatcher("test", "/").
				WithRouteActionToUpstream("test", testUpstream.Upstream).
				Build()

			// create zipkin upstream
			zipkinUs := &gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "zipkin",
					Namespace: writeNamespace,
				},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &static_plugin_gloo.UpstreamSpec{
						Hosts: []*static_plugin_gloo.Host{
							{
								Addr: envoyInstance.LocalAddr(),
								Port: 9411,
							},
						},
					},
				},
			}

			// The set of resources that these tests will generate
			resourcesToCreate = &gloosnapshot.ApiSnapshot{
				Gateways: gatewayv1.GatewayList{
					gatewaydefaults.DefaultGateway(writeNamespace),
				},
				VirtualServices: gatewayv1.VirtualServiceList{
					vsToTestUpstream,
				},
				Upstreams: gloov1.UpstreamList{
					zipkinUs,
					testUpstream.Upstream,
				},
			}

			// Start a dummy server listening on 9411 for Zipkin requests
			zipkinApiHit = make(chan bool, 1)
			zipkinHandler := http.NewServeMux()
			zipkinHandler.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v2/spans")) // Zipkin json collector API
				fmt.Fprintf(w, "Dummy Zipkin Collector received request on - %q", html.EscapeString(r.URL.Path))
				zipkinApiHit <- true
			}))
			startCancellableZipkinServer(ctx, envoyInstance.LocalAddr()+":9411", zipkinHandler)
		})

		AfterEach(func() {
			envoyInstance.Clean()
			cancel()
		})

		JustBeforeEach(func() {
			// Create Resources
			err := testClients.WriteSnapshot(ctx, resourcesToCreate)
			Expect(err).NotTo(HaveOccurred())

			// Wait for a proxy to be accepted
			gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
			})

			// Ensure the testUpstream is reachable
			v1helpers.ExpectCurlWithOffset(
				0,
				v1helpers.CurlRequest{
					RootCA: nil,
					Port:   defaults.HttpPort,
					Host:   "test.com", // to match the vs-test
					Path:   "/",
					Body:   []byte("solo.io test"),
				},
				v1helpers.CurlResponse{
					Status:  http.StatusOK,
					Message: "",
				},
			)
		})

		JustAfterEach(func() {
			// We do not need to clean up the Snapshot that was written in the JustBeforeEach
			// That is because each test uses its own InMemoryCache
		})

		It("should not send trace msgs with nil provider", func() {
			err := gloohelpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Name:      gatewaydefaults.GatewayProxyName,
					Namespace: writeNamespace,
				},
				func(resource resources.Resource) {
					gw := resource.(*gatewayv1.Gateway)
					gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
							Tracing: nil,
						},
					}
				},
				testClients.GatewayClient.BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			testRequest := createRequestWithTracingEnabled("127.0.0.1", defaults.HttpPort)
			Eventually(func(g Gomega) {
				g.Eventually(testRequest).Should(BeEmpty())
				g.Eventually(zipkinApiHit).Should(Not(Receive()))
			}, time.Second*5, time.Millisecond*250, "zipkin server should not receive trace request")
		})

		It("should send trace msgs with valid zipkin provider (collector_ref)", func() {
			err := gloohelpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Name:      gatewaydefaults.GatewayProxyName,
					Namespace: writeNamespace,
				},
				func(resource resources.Resource) {
					gw := resource.(*gatewayv1.Gateway)
					gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
							Tracing: &tracing.ListenerTracingSettings{
								ProviderConfig: &tracing.ListenerTracingSettings_ZipkinConfig{
									ZipkinConfig: &envoytrace_gloo.ZipkinConfig{
										CollectorCluster: &envoytrace_gloo.ZipkinConfig_CollectorUpstreamRef{
											CollectorUpstreamRef: &core.ResourceRef{
												Name:      "zipkin", // matches the name of the zipkin upstream we created in the BeforeEach
												Namespace: writeNamespace,
											},
										},
										CollectorEndpoint:        "/api/v2/spans",
										CollectorEndpointVersion: envoytrace_gloo.ZipkinConfig_HTTP_JSON,
									},
								},
							},
						},
					}
				},
				testClients.GatewayClient.BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			testRequest := createRequestWithTracingEnabled("127.0.0.1", defaults.HttpPort)
			Eventually(func(g Gomega) {
				g.Eventually(testRequest).Should(BeEmpty())
				g.Eventually(zipkinApiHit).Should(Receive())
			}, time.Second*10, time.Second, "zipkin server should receive trace request").Should(Succeed())
		})

		It("should send trace msgs with valid zipkin provider (cluster_name)", func() {
			err := gloohelpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Name:      gatewaydefaults.GatewayProxyName,
					Namespace: writeNamespace,
				},
				func(resource resources.Resource) {
					gw := resource.(*gatewayv1.Gateway)
					gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
							Tracing: &tracing.ListenerTracingSettings{
								ProviderConfig: &tracing.ListenerTracingSettings_ZipkinConfig{
									ZipkinConfig: &envoytrace_gloo.ZipkinConfig{
										CollectorCluster: &envoytrace_gloo.ZipkinConfig_ClusterName{
											ClusterName: translator.UpstreamToClusterName(&core.ResourceRef{
												Name:      "zipkin",
												Namespace: writeNamespace,
											}),
										},
										CollectorEndpoint:        "/api/v2/spans",
										CollectorEndpointVersion: envoytrace_gloo.ZipkinConfig_HTTP_JSON,
									},
								},
							},
						},
					}
				},
				testClients.GatewayClient.BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			testRequest := createRequestWithTracingEnabled("127.0.0.1", defaults.HttpPort)
			Eventually(func(g Gomega) {
				g.Eventually(testRequest).Should(BeEmpty())
				g.Eventually(zipkinApiHit).Should(Receive())
			}, time.Second*10, time.Second, "zipkin server should receive trace request").Should(Succeed())
		})

		It("should error with invalid zipkin provider", func() {
			err := gloohelpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Name:      gatewaydefaults.GatewayProxyName,
					Namespace: writeNamespace,
				},
				func(resource resources.Resource) {
					gw := resource.(*gatewayv1.Gateway)
					gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
							Tracing: &tracing.ListenerTracingSettings{
								ProviderConfig: &tracing.ListenerTracingSettings_ZipkinConfig{
									ZipkinConfig: &envoytrace_gloo.ZipkinConfig{
										CollectorCluster: &envoytrace_gloo.ZipkinConfig_CollectorUpstreamRef{
											CollectorUpstreamRef: nil,
										},
										CollectorEndpoint:        "/api/v2/spans",
										CollectorEndpointVersion: envoytrace_gloo.ZipkinConfig_HTTP_JSON,
									},
								},
							},
						},
					}
				},
				testClients.GatewayClient.BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			// ensure the proxy is never updated with the invalid configuration
			Consistently(func(g Gomega) int {
				tracingConfigsFound := 0

				proxy, err := testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				g.Expect(err).NotTo(HaveOccurred())

				for _, l := range proxy.GetListeners() {
					if l.GetHttpListener().GetOptions().GetHttpConnectionManagerSettings().GetTracing() != nil {
						tracingConfigsFound += 1
					}
				}
				return tracingConfigsFound
			}, time.Second*3, time.Second).Should(Equal(0))
		})

	})
})

func startCancellableZipkinServer(serverContext context.Context, address string, handler http.Handler) {
	zipkinServer := &http.Server{
		Addr:    address,
		Handler: handler,
	}

	// Start a goroutine to handle requests
	go func() {
		defer GinkgoRecover()
		if err := zipkinServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		}
	}()

	// Start a goroutine to shutdown the server
	go func(serverCtx context.Context) {
		defer GinkgoRecover()

		<-serverCtx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		ExpectWithOffset(1, zipkinServer.Shutdown(shutdownCtx)).NotTo(HaveOccurred())
	}(serverContext)
}

func createRequestWithTracingEnabled(address string, port uint32) func() (string, error) {
	return func() (string, error) {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/", address, port), nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")

		// Set a random trace ID
		req.Header.Set("x-client-trace-id", "test-trace-id-1234567890")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		body, err := ioutil.ReadAll(res.Body)
		return string(body), err
	}
}
