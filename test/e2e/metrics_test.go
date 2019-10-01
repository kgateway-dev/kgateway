package e2e_test

import (
	"context"
	"sync/atomic"

	v2 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v2"
	"github.com/fgrosse/zaptest"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/metrics/pkg/metricsservice"
	"github.com/solo-io/gloo/projects/metrics/pkg/runner"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

type testMetricsHandler struct {
	channel chan *struct{}
}

func (t *testMetricsHandler) HandleMetrics(context.Context, *v2.StreamMetricsMessage) error {
	// just signal that we did receive metrics from envoy
	t.channel <- &struct {
	}{}
	return nil
}

var _ = Describe("Gateway", func() {

	var (
		gw             *gatewayv2.Gateway
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		settings       runner.Settings
		writeNamespace string

		baseMetricsPort = uint32(27000)
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

		Context("Metrics", func() {

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
					channel     chan *struct{}
					testHandler *testMetricsHandler
				)

				BeforeEach(func() {
					metricsPort := atomic.AddUint32(&baseMetricsPort, 1) + uint32(config.GinkgoConfig.ParallelNode*1000)

					logger := zaptest.LoggerWriter(GinkgoWriter)
					contextutils.SetFallbackLogger(logger.Sugar())

					envoyInstance.MetricsPort = metricsPort
					err := envoyInstance.RunWithRole(writeNamespace+"~gateway-proxy-v2", testClients.GlooPort)
					Expect(err).NotTo(HaveOccurred())

					gatewaycli := testClients.GatewayClient
					gw, err = gatewaycli.Read("gloo-system", gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					Expect(err).NotTo(HaveOccurred())

					settings = runner.Settings{
						ServerPort: int(metricsPort),
					}

					opts := metricsservice.Options{
						Ctx: ctx,
					}

					//usageMerger := metricsservice.NewUsageMerger(time.Now)
					//storage := metricsservice.NewConfigMapStorage(writeNamespace, helpers.MustKubeClient().CoreV1().ConfigMaps(writeNamespace))
					//
					//defaulthandler := metricsservice.NewDefaultMetricsHandler(storage, usageMerger)

					channel = make(chan *struct{}, 1000)
					testHandler = &testMetricsHandler{channel: channel}
					service := metricsservice.NewServer(opts, testHandler)
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
					gw, err = gatewaycli.Read("gloo-system", gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					Expect(err).NotTo(HaveOccurred())
					gw.Plugins = nil
					_, err = gatewaycli.Write(gw, clients.WriteOpts{OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())
				})

				It("can stream metrics", func() {
					vs := getTrivialVirtualServiceForUpstream("default", tu.Upstream.Metadata.Ref())
					_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					TestUpstreamReachable()
					Expect(<-channel).To(Equal(&struct {
					}{}))
				}, 20)
			})
		})
	})
})
