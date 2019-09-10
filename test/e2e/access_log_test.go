package e2e_test

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"sync/atomic"
	"time"

	envoy_data_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v2"
	envoyals "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v2"
	"github.com/fgrosse/zaptest"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/accesslogger/pkg/loggingservice"
	"github.com/solo-io/gloo/projects/accesslogger/pkg/runner"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	alsplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Gateway", func() {

	var (
		gw             *gatewayv2.Gateway
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		settings       runner.Settings
		writeNamespace string

		baseAccessLogPort = uint32(27000)
	)

	Describe("in memory", func() {

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			defaults.HttpPort = services.NextBindPort()
			defaults.HttpsPort = services.NextBindPort()

			writeNamespace = "gloo-system"
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

			// wait for the two gateways to be created.
			Eventually(func() (gatewayv2.GatewayList, error) {
				return testClients.GatewayClient.List(writeNamespace, clients.ListOpts{})
			}, "10s", "0.1s").Should(HaveLen(2))
		})

		AfterEach(func() {
			cancel()
		})

		Context("Access Logs", func() {

			var (
				envoyInstance *services.EnvoyInstance
				tu            *v1helpers.TestUpstream
			)

			TestUpstreamReachable := func() {
				v1helpers.TestUpstreamReachable(defaults.HttpPort, tu, nil)
			}

			BeforeEach(func() {
				ctx, cancel = context.WithCancel(context.Background())
				var err error
				envoyInstance, err = envoyFactory.NewEnvoyInstance()
				Expect(err).NotTo(HaveOccurred())

				tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

				_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				if envoyInstance != nil {
					_ = envoyInstance.Clean()
				}
			})

			Context("Grpc", func() {

				var (
					msgChan chan *envoy_data_accesslog_v2.HTTPAccessLogEntry
				)

				BeforeEach(func() {
					msgChan = make(chan *envoy_data_accesslog_v2.HTTPAccessLogEntry, 20)
					accessLogPort := atomic.AddUint32(&baseAccessLogPort, 1) + uint32(config.GinkgoConfig.ParallelNode*1000)

					logger := zaptest.LoggerWriter(GinkgoWriter)
					contextutils.SetFallbackLogger(logger.Sugar())

					envoyInstance.AccessLogPort = accessLogPort
					err := envoyInstance.RunWithRole(writeNamespace+"~gateway-proxy-v2", testClients.GlooPort)
					Expect(err).NotTo(HaveOccurred())

					gatewaycli := testClients.GatewayClient
					gw, err = gatewaycli.Read("gloo-system", "gateway-proxy-v2", clients.ReadOpts{})
					Expect(err).NotTo(HaveOccurred())

					settings = runner.Settings{
						DebugPort:  0,
						ServerPort: int(accessLogPort),
					}

					opts := loggingservice.Options{
						Ordered: true,
						Callbacks: loggingservice.AlsCallbackList{
							func(ctx context.Context, message *envoyals.StreamAccessLogsMessage) error {
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
					go func(testctx context.Context) {
						defer GinkgoRecover()
						err := runner.RunWithSettings(testctx, service, settings)
						if testctx.Err() == nil {
							Expect(err).NotTo(HaveOccurred())
						}
					}(ctx)
				})

				AfterEach(func() {
					gatewaycli := testClients.GatewayClient
					var err error
					gw, err = gatewaycli.Read("gloo-system", "gateway-proxy-v2", clients.ReadOpts{})
					Expect(err).NotTo(HaveOccurred())
					gw.Plugins = nil
					_, err = gatewaycli.Write(gw, clients.WriteOpts{OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())
				})

				It("can stream access logs", func() {
					logName := "test-log"
					gw.Plugins = &gloov1.ListenerPlugins{
						AccessLoggingService: &als.AccessLoggingService{
							AccessLog: []*als.AccessLog{
								{
									OutputDestination: &als.AccessLog_GrpcService{
										GrpcService: &als.GrpcService{
											LogName: logName,
											ServiceRef: &als.GrpcService_StaticClusterName{
												StaticClusterName: alsplugin.ClusterName,
											},
										},
									},
								},
							},
						},
					}

					gatewaycli := testClients.GatewayClient
					_, err := gatewaycli.Write(gw, clients.WriteOpts{OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())

					vs := getTrivialVirtualServiceForUpstream("default", tu.Upstream.Metadata.Ref())
					_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					TestUpstreamReachable()
					var entry *envoy_data_accesslog_v2.HTTPAccessLogEntry
					Eventually(msgChan).Should(Receive(&entry))
					Expect(entry.CommonProperties.UpstreamCluster).To(Equal(translator.UpstreamToClusterName(tu.Upstream.Metadata.Ref())))
				})
			})

			Context("File", func() {
				var (
					path string
				)

				var checkLogs = func(ei *services.EnvoyInstance, logsPresent func(logs string) bool) error {
					var (
						logs string
						err  error
					)

					if ei.UseDocker {
						logs, err = ei.Logs()
						if err != nil {
							return err
						}
					} else {
						file, err := os.OpenFile(ei.AccessLogs, os.O_RDONLY, 0777)
						if err != nil {
							return err
						}
						var byt []byte
						byt, err = ioutil.ReadAll(file)
						if err != nil {
							return err
						}
						logs = string(byt)
					}

					if logs == "" {
						return errors.Errorf("logs should not be empty")
					}
					if !logsPresent(logs) {
						return errors.Errorf("no access logs present")
					}
					return nil
				}

				BeforeEach(func() {
					err := envoyInstance.RunWithRole(writeNamespace+"~gateway-proxy-v2", testClients.GlooPort)
					Expect(err).NotTo(HaveOccurred())

					gatewaycli := testClients.GatewayClient
					gw, err = gatewaycli.Read("gloo-system", "gateway-proxy-v2", clients.ReadOpts{})
					Expect(err).NotTo(HaveOccurred())
					path = "/dev/stdout"
					if !envoyInstance.UseDocker {
						tmpfile, err := ioutil.TempFile("", "")
						Expect(err).NotTo(HaveOccurred())
						path = tmpfile.Name()
						envoyInstance.AccessLogs = path
					}
				})
				AfterEach(func() {
					gatewaycli := testClients.GatewayClient
					var err error
					gw, err = gatewaycli.Read("gloo-system", "gateway-proxy-v2", clients.ReadOpts{})
					Expect(err).NotTo(HaveOccurred())
					gw.Plugins = nil
					_, err = gatewaycli.Write(gw, clients.WriteOpts{OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())
				})
				It("can create string access logs", func() {
					gw.Plugins = &gloov1.ListenerPlugins{
						AccessLoggingService: &als.AccessLoggingService{
							AccessLog: []*als.AccessLog{
								{
									OutputDestination: &als.AccessLog_FileSink{
										FileSink: &als.FileSink{
											Path: path,
											OutputFormat: &als.FileSink_StringFormat{
												StringFormat: "",
											},
										},
									},
								},
							},
						},
					}

					gatewaycli := testClients.GatewayClient
					_, err := gatewaycli.Write(gw, clients.WriteOpts{OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())
					up := tu.Upstream
					vs := getTrivialVirtualServiceForUpstream("default", up.Metadata.Ref())
					_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())
					TestUpstreamReachable()

					Eventually(func() error {
						var logsPresent = func(logs string) bool {
							return strings.Contains(logs, `"POST /1 HTTP/1.1" 200`)
						}
						return checkLogs(envoyInstance, logsPresent)
					}, time.Second*30, time.Second/2).ShouldNot(HaveOccurred())
				})
				It("can create json access logs", func() {
					gw.Plugins = &gloov1.ListenerPlugins{
						AccessLoggingService: &als.AccessLoggingService{
							AccessLog: []*als.AccessLog{
								{
									OutputDestination: &als.AccessLog_FileSink{
										FileSink: &als.FileSink{
											Path: path,
											OutputFormat: &als.FileSink_JsonFormat{
												JsonFormat: &types.Struct{
													Fields: map[string]*types.Value{
														"protocol": {
															Kind: &types.Value_StringValue{
																StringValue: "%PROTOCOL%",
															},
														},
														"method": {
															Kind: &types.Value_StringValue{
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
					gatewaycli := testClients.GatewayClient
					_, err := gatewaycli.Write(gw, clients.WriteOpts{OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())
					up := tu.Upstream
					vs := getTrivialVirtualServiceForUpstream("default", up.Metadata.Ref())
					_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					TestUpstreamReachable()
					Eventually(func() error {
						var logsPresent = func(logs string) bool {
							return strings.Contains(logs, `{"method":"POST","protocol":"HTTP/1.1"}`) ||
								strings.Contains(logs, `{"protocol":"HTTP/1.1","method":"POST"}`)
						}
						return checkLogs(envoyInstance, logsPresent)
					}, time.Second*30, time.Second/2).ShouldNot(HaveOccurred())
				})
			})
		})
	})
})
