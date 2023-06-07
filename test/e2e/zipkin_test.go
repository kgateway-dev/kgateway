package e2e_test

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"time"

	"github.com/solo-io/gloo/test/services/envoy"

	"github.com/solo-io/gloo/test/testutils"

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/test/gomega"

	"github.com/solo-io/gloo/test/services"
)

const (
	tracingCollectorPort         = 9411
	tracingCollectorUpstreamName = "tracing-collector"
	openTelemetryCollectionPath  = "/opentelemetry.proto.collector.trace.v1.TraceService/Export"
	zipkinCollectionPath         = "/api/v2/spans"
)

var _ = Describe("Tracing config loading", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		envoyInstance *envoy.Instance
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		envoy.AdvanceRequestPorts()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	Context("Tracing defined on Envoy bootstrap", func() {

		BeforeEach(func() {
			testutils.ValidateRequirementsAndNotifyGinkgo(
				testutils.LinuxOnly("Uses 127.0.0.1"),
			)
		})

		It("should send trace msgs to the zipkin server", func() {
			err := envoyInstance.RunWithConfigFile(int(envoy.HttpPort), "./envoyconfigs/zipkin-envoy-conf.yaml")
			Expect(err).NotTo(HaveOccurred())

			// Start a dummy server listening on 9411 for Zipkin requests
			apiHit := make(chan bool, 1)
			zipkinHandler := http.NewServeMux()
			zipkinHandler.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal(zipkinCollectionPath))
				fmt.Fprintf(w, "Dummy Zipkin Collector received request on - %q", html.EscapeString(r.URL.Path))
				apiHit <- true
			}))
			startCancellableTracingServer(ctx, fmt.Sprintf("%s:%d", envoyInstance.LocalAddr(), tracingCollectorPort), zipkinHandler)

			// Execute a request against the admin endpoint, as this should result in a trace
			testRequest := createRequestWithTracingEnabled("127.0.0.1", 11082)
			Eventually(func(g Gomega) {
				res, err := testRequest()
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).To(ContainSubstring(`<title>Envoy Admin</title>`))

				g.Eventually(apiHit, 1*time.Second).Should(Receive(BeTrue()))
			}, "10s", ".5s").Should(Succeed(), "Admin endpoint request should result in trace")

		})

	})

	Context("Tracing defined on Gloo resources", func() {

		var (
			testClients  services.TestClients
			testUpstream *v1helpers.TestUpstream

			resourcesToCreate *gloosnapshot.ApiSnapshot
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

			// create tracing collector upstream
			tracingCollectorUs := &gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      tracingCollectorUpstreamName,
					Namespace: writeNamespace,
				},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &static_plugin_gloo.UpstreamSpec{
						Hosts: []*static_plugin_gloo.Host{
							{
								Addr: envoyInstance.LocalAddr(),
								Port: tracingCollectorPort,
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
					tracingCollectorUs,
					testUpstream.Upstream,
				},
			}
		})

		AfterEach(func() {
			envoyInstance.Clean()
			cancel()
		})

		startTracingCollectionServer := func(collectorApiChannel chan bool, collectionURLPath string) {
			// Start a dummy server listening on 9411 for tracing requests
			tracingCollectorHandler := http.NewServeMux()
			tracingCollectorHandler.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal(collectionURLPath))
				fmt.Fprintf(w, "Dummy tracing Collector received request on - %q", html.EscapeString(r.URL.Path))
				collectorApiChannel <- true
			}))
			startCancellableTracingServer(ctx, fmt.Sprintf("%s:%d", envoyInstance.LocalAddr(), tracingCollectorPort), tracingCollectorHandler)

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
					Port:   envoy.HttpPort,
					Host:   "test.com", // to match the vs-test
					Path:   "/",
					Body:   []byte("solo.io test"),
				},
				v1helpers.CurlResponse{
					Status:  http.StatusOK,
					Message: "solo.io test",
				},
			)
		}

		It("should send trace msgs with valid opentelemetry provider (collector_ref)", func() {
			collectorApiHit := make(chan bool, 1)
			startTracingCollectionServer(collectorApiHit, openTelemetryCollectionPath)

			err := gloohelpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Name:      gatewaydefaults.GatewayProxyName,
					Namespace: writeNamespace,
				},
				func(resource resources.Resource) resources.Resource {
					gw := resource.(*gatewayv1.Gateway)
					gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
							Tracing: &tracing.ListenerTracingSettings{
								ProviderConfig: &tracing.ListenerTracingSettings_OpenTelemetryConfig{
									OpenTelemetryConfig: &envoytrace_gloo.OpenTelemetryConfig{
										CollectorCluster: &envoytrace_gloo.OpenTelemetryConfig_CollectorUpstreamRef{
											CollectorUpstreamRef: &core.ResourceRef{
												Name:      tracingCollectorUpstreamName,
												Namespace: writeNamespace,
											},
										},
									},
								},
							},
						},
					}
					return gw
				},
				testClients.GatewayClient.BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			testRequest := createRequestWithTracingEnabled("localhost", envoy.HttpPort)
			Eventually(func(g Gomega) {
				g.Eventually(testRequest, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(BeEmpty())
				g.Eventually(collectorApiHit, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Receive())
			}, time.Second*10, time.Second).Should(Succeed(), "tracing server should receive trace request")
		})

		It("should send trace msgs with valid opentelemetry provider (cluster_name)", func() {
			collectorApiHit := make(chan bool, 1)
			startTracingCollectionServer(collectorApiHit, openTelemetryCollectionPath)

			err := gloohelpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Name:      gatewaydefaults.GatewayProxyName,
					Namespace: writeNamespace,
				},
				func(resource resources.Resource) resources.Resource {
					gw := resource.(*gatewayv1.Gateway)
					gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
							Tracing: &tracing.ListenerTracingSettings{
								ProviderConfig: &tracing.ListenerTracingSettings_OpenTelemetryConfig{
									OpenTelemetryConfig: &envoytrace_gloo.OpenTelemetryConfig{
										CollectorCluster: &envoytrace_gloo.OpenTelemetryConfig_ClusterName{
											ClusterName: translator.UpstreamToClusterName(&core.ResourceRef{
												Name:      tracingCollectorUpstreamName,
												Namespace: writeNamespace,
											}),
										},
									},
								},
							},
						},
					}
					return gw
				},
				testClients.GatewayClient.BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			testRequest := createRequestWithTracingEnabled("localhost", envoy.HttpPort)
			Eventually(func(g Gomega) {
				g.Eventually(testRequest, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(BeEmpty())
				g.Eventually(collectorApiHit, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Receive())
			}, time.Second*10, time.Second).Should(Succeed(), "tracing server should receive trace request")
		})

		It("should not send trace msgs with nil provider", func() {
			collectorApiHit := make(chan bool, 1)
			startTracingCollectionServer(collectorApiHit, zipkinCollectionPath)

			err := gloohelpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Name:      gatewaydefaults.GatewayProxyName,
					Namespace: writeNamespace,
				},
				func(resource resources.Resource) resources.Resource {
					gw := resource.(*gatewayv1.Gateway)
					gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
							Tracing: nil,
						},
					}
					return gw
				},
				testClients.GatewayClient.BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			testRequest := createRequestWithTracingEnabled("localhost", envoy.HttpPort)
			Eventually(func(g Gomega) {
				g.Eventually(testRequest, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(BeEmpty())
				g.Eventually(collectorApiHit, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Not(Receive()))
			}, time.Second*5, time.Millisecond*250).Should(Succeed(), "zipkin server should not receive trace request")
		})

		It("should send trace msgs with valid zipkin provider (collector_ref)", func() {
			collectorApiHit := make(chan bool, 1)
			startTracingCollectionServer(collectorApiHit, zipkinCollectionPath)

			err := gloohelpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Name:      gatewaydefaults.GatewayProxyName,
					Namespace: writeNamespace,
				},
				func(resource resources.Resource) resources.Resource {
					gw := resource.(*gatewayv1.Gateway)
					gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
							Tracing: &tracing.ListenerTracingSettings{
								ProviderConfig: &tracing.ListenerTracingSettings_ZipkinConfig{
									ZipkinConfig: &envoytrace_gloo.ZipkinConfig{
										CollectorCluster: &envoytrace_gloo.ZipkinConfig_CollectorUpstreamRef{
											CollectorUpstreamRef: &core.ResourceRef{
												Name:      tracingCollectorUpstreamName,
												Namespace: writeNamespace,
											},
										},
										CollectorEndpoint:        zipkinCollectionPath,
										CollectorEndpointVersion: envoytrace_gloo.ZipkinConfig_HTTP_JSON,
									},
								},
							},
						},
					}
					return gw
				},
				testClients.GatewayClient.BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			testRequest := createRequestWithTracingEnabled("localhost", envoy.HttpPort)
			Eventually(func(g Gomega) {
				g.Eventually(testRequest, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(BeEmpty())
				g.Eventually(collectorApiHit, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Receive())
			}, time.Second*10, time.Second).Should(Succeed(), "tracing server should receive trace request")
		})

		It("should send trace msgs with valid zipkin provider (cluster_name)", func() {
			collectorApiHit := make(chan bool, 1)
			startTracingCollectionServer(collectorApiHit, zipkinCollectionPath)

			err := gloohelpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Name:      gatewaydefaults.GatewayProxyName,
					Namespace: writeNamespace,
				},
				func(resource resources.Resource) resources.Resource {
					gw := resource.(*gatewayv1.Gateway)
					gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
							Tracing: &tracing.ListenerTracingSettings{
								ProviderConfig: &tracing.ListenerTracingSettings_ZipkinConfig{
									ZipkinConfig: &envoytrace_gloo.ZipkinConfig{
										CollectorCluster: &envoytrace_gloo.ZipkinConfig_ClusterName{
											ClusterName: translator.UpstreamToClusterName(&core.ResourceRef{
												Name:      tracingCollectorUpstreamName,
												Namespace: writeNamespace,
											}),
										},
										CollectorEndpoint:        zipkinCollectionPath,
										CollectorEndpointVersion: envoytrace_gloo.ZipkinConfig_HTTP_JSON,
									},
								},
							},
						},
					}
					return gw
				},
				testClients.GatewayClient.BaseClient(),
			)
			Expect(err).NotTo(HaveOccurred())

			testRequest := createRequestWithTracingEnabled("localhost", envoy.HttpPort)
			Eventually(func(g Gomega) {
				g.Eventually(testRequest, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(BeEmpty())
				g.Eventually(collectorApiHit, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Receive())
			}, time.Second*10, time.Second).Should(Succeed(), "zipkin server should receive trace request")
		})

		It("should error with invalid zipkin provider", func() {
			collectorApiHit := make(chan bool, 1)
			startTracingCollectionServer(collectorApiHit, zipkinCollectionPath)

			err := gloohelpers.PatchResource(
				ctx,
				&core.ResourceRef{
					Name:      gatewaydefaults.GatewayProxyName,
					Namespace: writeNamespace,
				},
				func(resource resources.Resource) resources.Resource {
					gw := resource.(*gatewayv1.Gateway)
					gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
							Tracing: &tracing.ListenerTracingSettings{
								ProviderConfig: &tracing.ListenerTracingSettings_ZipkinConfig{
									ZipkinConfig: &envoytrace_gloo.ZipkinConfig{
										CollectorCluster: &envoytrace_gloo.ZipkinConfig_CollectorUpstreamRef{
											CollectorUpstreamRef: nil,
										},
										CollectorEndpoint:        zipkinCollectionPath,
										CollectorEndpointVersion: envoytrace_gloo.ZipkinConfig_HTTP_JSON,
									},
								},
							},
						},
					}
					return gw
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

func startCancellableTracingServer(serverContext context.Context, address string, handler http.Handler) {
	tracingServer := &http.Server{
		Addr:    address,
		Handler: handler,
	}

	// Start a goroutine to handle requests
	go func() {
		defer GinkgoRecover()
		if err := tracingServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		}
	}()

	// Start a goroutine to shutdown the server
	go func(serverCtx context.Context) {
		defer GinkgoRecover()

		<-serverCtx.Done()
		// tracingServer.Shutdown hangs with opentelemetry tests, probably
		// because the agent leaves the connection open. There's no need for a
		// graceful shutdown anyway, so just force it using Close() instead
		tracingServer.Close()
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
		body, err := io.ReadAll(res.Body)
		return string(body), err
	}
}
