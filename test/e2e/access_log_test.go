package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/solo-io/gloo/test/services"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/matchers"

	envoy_data_accesslog_v3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"

	envoyals "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/accesslogger/pkg/loggingservice"
	"github.com/solo-io/gloo/projects/accesslogger/pkg/runner"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
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
			Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/1", "localhost", defaults.HttpPort), nil)
				g.Expect(err).NotTo(HaveOccurred())
				req.Host = e2e.DefaultHost
				g.Expect(http.DefaultClient.Do(req)).Should(matchers.HaveOkResponse())

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
				Eventually(func(g Gomega) {
					req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/1", "localhost", defaults.HttpPort), nil)
					g.Expect(err).NotTo(HaveOccurred())
					req.Host = e2e.DefaultHost
					g.Expect(http.DefaultClient.Do(req)).Should(matchers.HaveOkResponse())

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
				Eventually(func(g Gomega) {
					req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/1", "localhost", defaults.HttpPort), nil)
					g.Expect(err).NotTo(HaveOccurred())
					req.Host = e2e.DefaultHost
					g.Expect(http.DefaultClient.Do(req)).Should(matchers.HaveOkResponse())

					logs, err := testContext.EnvoyInstance().Logs()
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(logs).To(ContainSubstring(`"method":"POST"`))
					g.Expect(logs).To(ContainSubstring(`"protocol":"HTTP/1.1"`))
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
