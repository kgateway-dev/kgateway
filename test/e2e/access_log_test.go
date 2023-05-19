package e2e_test

import (
	"context"
	"net/http"
	"time"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/solo-io/gloo/test/services"

	envoy_data_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/e2e"

	envoyals "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/accesslogger/pkg/loggingservice"
	"github.com/solo-io/gloo/projects/accesslogger/pkg/runner"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	gloo_envoy_v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	alsplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var _ = Describe("Access Log", func() {

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

	Context("Grpc", func() {

		var (
			msgChan <-chan *envoy_data_accesslog_v3.HTTPAccessLogEntry
		)

		BeforeEach(func() {
			accessLogPort := services.NextBindPort()
			msgChan = runAccessLog(testContext.Ctx(), accessLogPort)
			testContext.EnvoyInstance().AccessLogPort = accessLogPort

			gw := gwdefaults.DefaultGateway(writeNamespace)
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

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gw,
			}
		})

		It("can stream access logs", func() {
			requestBuilder := testContext.GetHttpRequestBuilder()
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())

				var entry *envoy_data_accesslog_v3.HTTPAccessLogEntry
				g.Eventually(msgChan, 2*time.Second).Should(Receive(&entry))
				g.Expect(entry.CommonProperties.UpstreamCluster).To(Equal(translator.UpstreamToClusterName(testContext.TestUpstream().Upstream.Metadata.Ref())))
			}, time.Second*21, time.Second*2).Should(Succeed())
		})

	})

	Context("File", func() {

		Context("String Format", func() {
			BeforeEach(func() {
				gw := gwdefaults.DefaultGateway(writeNamespace)
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

				testContext.ResourcesToCreate().Gateways = v1.GatewayList{
					gw,
				}
			})

			It("can create string access logs", func() {
				requestBuilder := testContext.GetHttpRequestBuilder().
					WithPath("1").
					WithPostMethod()
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())

					logs, err := testContext.EnvoyInstance().Logs()
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(logs).To(ContainSubstring(`"POST /1 HTTP/1.1" 200`))
				}, time.Second*30, time.Second/2).Should(Succeed())
			})
		})

		Context("Json Format", func() {

			BeforeEach(func() {
				gw := gwdefaults.DefaultGateway(writeNamespace)
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

				testContext.ResourcesToCreate().Gateways = v1.GatewayList{
					gw,
				}
			})
			It("can create json access logs", func() {
				requestBuilder := testContext.GetHttpRequestBuilder().
					WithPath("1").
					WithPostMethod()
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())

					logs, err := testContext.EnvoyInstance().Logs()
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(logs).To(ContainSubstring(`"method":"POST"`))
					g.Expect(logs).To(ContainSubstring(`"protocol":"HTTP/1.1"`))
				}, time.Second*30, time.Second/2).Should(Succeed())
			})
		})
	})

	Context("Test Filters", func() {
		// The output format doesn't (or at least shouldn't) matter for the filter tests, except in how we examine the access logs
		// We'll use the string output because it's easiest to match against
		BeforeEach(func() {
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
			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gw,
			}
		})

		It("Can filter by status code", func() {
			requestBuilder := testContext.GetHttpRequestBuilder().
				WithPath("1").
				WithPostMethod()
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())

				logs, err := testContext.EnvoyInstance().Logs()
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(logs).To(Not(ContainSubstring(`"POST /1 HTTP/1.1" 200`)))
			}, time.Second*30, time.Second/2).Should(Succeed())

			badHostRequestBuilder := testContext.GetHttpRequestBuilder().
				WithPath("BAD/HOST").
				WithPostMethod().
				WithHost("") // We can get a 404 by not setting the Host header.
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(badHostRequestBuilder.Build())).Should(matchers.HaveStatusCode(http.StatusNotFound))

				logs, err := testContext.EnvoyInstance().Logs()
				g.Expect(err).To(Not(HaveOccurred()))
				g.Expect(logs).To(Not(ContainSubstring(`"POST /1 HTTP/1.1" 200`)))
				g.Expect(logs).To(ContainSubstring(`"POST /BAD/HOST HTTP/1.1" 404`))

			}, time.Second*30, time.Second/2).Should(Succeed())
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
