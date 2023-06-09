package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	envoy_data_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"

	envoyals "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/accesslogger/pkg/loggingservice"
	"github.com/solo-io/gloo/projects/accesslogger/pkg/runner"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloo_envoy_v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	alsplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Access Log", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		writeNamespace string
		envoyInstance  *services.EnvoyInstance
		tu             *v1helpers.TestUpstream

		baseAccessLogPort = uint32(27000)
	)

	Describe("in memory", func() {

		BeforeEach(func() {
			var err error

			ctx, cancel = context.WithCancel(context.Background())
			defaults.HttpPort = services.NextBindPort()
			defaults.HttpsPort = services.NextBindPort()

			writeNamespace = defaults.GlooSystem
			ro := &services.RunOptions{
				NsToWrite: writeNamespace,
				NsToWatch: []string{"default", writeNamespace},
				WhatToRun: services.What{
					DisableGateway: false,
					DisableFds:     true,
					DisableUds:     true,
				},
			}
			testClients = services.RunGlooGatewayUdsFds(ctx, ro)

			envoyInstance, err = envoyFactory.NewEnvoyInstance()
			Expect(err).NotTo(HaveOccurred())

			tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

			_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred(), "Should be abel to write test upstream")

			vs := getTrivialVirtualServiceForUpstream(writeNamespace, tu.Upstream.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred(), "Should be abel to write virtual service to test upstream")

			err = helpers.WriteDefaultGateways(writeNamespace, testClients.GatewayClient)
			Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")

			// wait for the two gateways to be created.
			Eventually(func() (gatewayv1.GatewayList, error) {
				return testClients.GatewayClient.List(writeNamespace, clients.ListOpts{Ctx: ctx})
			}, "10s", "0.1s").Should(HaveLen(2))
		})

		AfterEach(func() {
			envoyInstance.Clean()
			cancel()
		})

		TestUpstreamReachable := func() {
			v1helpers.TestUpstreamReachable(defaults.HttpPort, tu, nil)
		}

		Context("Grpc", func() {

			var (
				msgChan <-chan *envoy_data_accesslog_v3.HTTPAccessLogEntry
			)

			BeforeEach(func() {
				accessLogPort := atomic.AddUint32(&baseAccessLogPort, 1) + uint32(config.GinkgoConfig.ParallelNode*1000)

				envoyInstance.AccessLogPort = accessLogPort
				err := envoyInstance.RunWithRole(writeNamespace+"~"+gwdefaults.GatewayProxyName, testClients.GlooPort)
				Expect(err).NotTo(HaveOccurred())

				msgChan = runAccessLog(ctx, accessLogPort)
			})

			It("can stream access logs", func() {
				gw, err := testClients.GatewayClient.Read(writeNamespace, gwdefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				By("Update default gateway to use grpc access log service")
				gw.Options = &gloov1.ListenerOptions{
					AccessLoggingService: &als.AccessLoggingService{
						AccessLog: []*als.AccessLog{
							{
								OutputDestination: &als.AccessLog_GrpcService{
									GrpcService: &als.GrpcService{
										LogName: "test-log",
										ServiceRef: &als.GrpcService_StaticClusterName{
											StaticClusterName: alsplugin.ClusterName,
										},
									},
								},
							},
						},
					},
				}
				_, err = testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func(g Gomega) {
					TestUpstreamReachable()

					var entry *envoy_data_accesslog_v3.HTTPAccessLogEntry
					g.Eventually(msgChan, 2*time.Second).Should(Receive(&entry))
					g.Expect(entry.CommonProperties.UpstreamCluster).To(Equal(translator.UpstreamToClusterName(tu.Upstream.Metadata.Ref())))
				}, time.Second*21, time.Second*2)

			})
		})

		Context("File", func() {

			BeforeEach(func() {
				err := envoyInstance.RunWithRole(writeNamespace+"~"+gwdefaults.GatewayProxyName, testClients.GlooPort)
				Expect(err).NotTo(HaveOccurred())
			})

			It("can create string access logs", func() {
				gw, err := testClients.GatewayClient.Read(writeNamespace, gwdefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				gw.Options = &gloov1.ListenerOptions{
					AccessLoggingService: &als.AccessLoggingService{
						AccessLog: []*als.AccessLog{
							{
								OutputDestination: &als.AccessLog_FileSink{
									FileSink: &als.FileSink{
										Path: "/dev/stdout",
										OutputFormat: &als.FileSink_StringFormat{
											StringFormat: "",
										},
									},
								},
							},
						},
					},
				}
				_, err = testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func(g Gomega) {
					TestUpstreamReachable()

					logs, err := envoyInstance.Logs()
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(logs).To(ContainSubstring(`"POST /1 HTTP/1.1" 200`))
				}, time.Second*30, time.Second/2)
			})

			It("can create json access logs", func() {
				gw, err := testClients.GatewayClient.Read(writeNamespace, gwdefaults.GatewayProxyName, clients.ReadOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				gw.Options = &gloov1.ListenerOptions{
					AccessLoggingService: &als.AccessLoggingService{
						AccessLog: []*als.AccessLog{
							{
								OutputDestination: &als.AccessLog_FileSink{
									FileSink: &als.FileSink{
										Path: "/dev/stdout",
										OutputFormat: &als.FileSink_JsonFormat{
											JsonFormat: &structpb.Struct{
												Fields: map[string]*structpb.Value{
													"protocol": {
														Kind: &structpb.Value_StringValue{
															StringValue: "%PROTOCOL%",
														},
													},
													"method": {
														Kind: &structpb.Value_StringValue{
															StringValue: "%REQ(:METHOD)%",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}

				_, err = testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func(g Gomega) {
					TestUpstreamReachable()

					logs, err := envoyInstance.Logs()
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(logs).To(ContainSubstring(`"method":"POST"`))
					g.Expect(logs).To(ContainSubstring(`"protocol":"HTTP/1.1"`))
				}, time.Second*30, time.Second/2).ShouldNot(HaveOccurred())
			})
		})

		Context("Test Filters", func() {
			// The output format doesn't (or at least shouldn't) matter for the filter tests, except in how we examine the access logs
			// We'll use the string output because it's easiest to match against
			BeforeEach(func() {
				err := envoyInstance.RunWithRole(writeNamespace+"~"+gwdefaults.GatewayProxyName, testClients.GlooPort)
				Expect(err).NotTo(HaveOccurred())
				gw := gwdefaults.DefaultGateway(writeNamespace)
				filter := &als.AccessLogFilter{
					FilterSpecifier: &als.AccessLogFilter_StatusCodeFilter{
						StatusCodeFilter: &als.StatusCodeFilter{
							Comparison: &als.ComparisonFilter{
								Op: als.ComparisonFilter_EQ,
								Value: &gloo_envoy_v3.RuntimeUInt32{
									DefaultValue: 404,
									RuntimeKey:   "404",
								},
							},
						},
					},
				}

				gw.Options = &gloov1.ListenerOptions{
					AccessLoggingService: &als.AccessLoggingService{
						AccessLog: []*als.AccessLog{
							{
								OutputDestination: &als.AccessLog_FileSink{
									FileSink: &als.FileSink{
										Path: "/dev/stdout",
										OutputFormat: &als.FileSink_StringFormat{
											StringFormat: "",
										},
									},
								},
								Filter: filter,
							},
						},
					},
				}

			})

			It("Can filter by status code", func() {
				req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/1", "localhost", defaults.HttpPort), nil)
				Expect(err).NotTo(HaveOccurred())
				req.Host = "test.com"
				Eventually(func(g Gomega) {
					resp, err := http.DefaultClient.Do(req)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(resp.StatusCode).To(Equal(http.StatusOK))

					logs, err := envoyInstance.Logs()
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(logs).To(Not(ContainSubstring(`"POST /1 HTTP/1.1" 200`)))
				}, time.Second*30, time.Second/2).Should(Succeed())

				req, err = http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/BAD/HOST", "localhost", defaults.HttpPort), nil)
				Expect(err).NotTo(HaveOccurred())
				req.Host = "" // We can get a 404 by not setting the Host header.

				Eventually(func(g Gomega) {
					resp, err := http.DefaultClient.Do(req)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

					logs, err := envoyInstance.Logs()
					g.Expect(err).To(Not(HaveOccurred()))
					g.Expect(logs).To(Not(ContainSubstring(`"POST /1 HTTP/1.1" 200`)))
					g.Expect(logs).To(ContainSubstring(`"POST /BAD/HOST HTTP/1.1" 404`))

				}, time.Second*30, time.Second/2).Should(Succeed())
			})
		})

	})
})

func runAccessLog(ctx context.Context, accessLogPort uint32) <-chan *envoy_data_accesslog_v3.HTTPAccessLogEntry {
	msgChan := make(chan *envoy_data_accesslog_v3.HTTPAccessLogEntry, 10)

	opts := loggingservice.Options{
		Ordered: true,
		Callbacks: loggingservice.AlsCallbackList{
			func(ctx context.Context, message *envoyals.StreamAccessLogsMessage) error {
				defer GinkgoRecover()
				httpLogs := message.GetHttpLogs()
				Expect(httpLogs).NotTo(BeNil())
				for _, v := range httpLogs.LogEntry {
					select {
					case msgChan <- v:
						return nil
					case <-time.After(time.Second):
						Fail("unable to send log message on channel")
					}
				}
				return nil
			},
		},
		Ctx: ctx,
	}

	service := loggingservice.NewServer(opts)

	settings := runner.Settings{
		DebugPort:   0,
		ServerPort:  int(accessLogPort),
		ServiceName: "AccessLog",
	}

	go func(testctx context.Context) {
		defer GinkgoRecover()
		err := runner.RunWithSettings(testctx, service, settings)
		if testctx.Err() == nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}(ctx)
	return msgChan
}
