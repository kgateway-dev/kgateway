package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/test/testutils"

	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/golang/protobuf/ptypes/wrappers"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/golang/protobuf/ptypes/duration"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
. "github.com/solo-io/gloo/test/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	glootest "github.com/solo-io/gloo/test/v1helpers/test_grpc_service/glootest/protos"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Health Checks", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		tu            *v1helpers.TestUpstream
	)

	BeforeEach(func() {
		testutils.ValidateRequirementsAndNotifyGinkgo(
			testutils.LinuxOnly("Relies on FDS"),
		)

		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		ro := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableGateway: false,
				DisableUds:     true,
				// test relies on FDS to discover the grpc spec via reflection
				DisableFds: false,
			},
			Settings: &gloov1.Settings{
				Gloo: &gloov1.GlooOptions{
					// https://github.com/solo-io/gloo/issues/7577
					RemoveUnusedFilters: &wrappers.BoolValue{Value: false},
				},
				Discovery: &gloov1.Settings_DiscoveryOptions{
					FdsMode: gloov1.Settings_DiscoveryOptions_BLACKLIST,
				},
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)
		err = envoyInstance.RunWithRole(writeNamespace+"~"+gwdefaults.GatewayProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())
		err = helpers.WriteDefaultGateways(writeNamespace, testClients.GatewayClient)
		Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	basicReq := func(b []byte) func() (string, error) {
		return func() (string, error) {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(b)
			res, err := http.Post(fmt.Sprintf("http://%s:%d/test", "localhost", defaults.HttpPort), "application/json", &buf)
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			return string(body), err
		}
	}

	Context("regression for config", func() {

		BeforeEach(func() {

			tu = v1helpers.NewTestGRPCUpstream(ctx, envoyInstance.LocalAddr(), 1)
			_, err := testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
		})

		tests := []struct {
			Name  string
			Check *envoy_config_core_v3.HealthCheck
		}{
			{
				Name: "http",
				Check: &envoy_config_core_v3.HealthCheck{
					HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
						HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
							Path: "xyz",
						},
					},
				},
			},
			{
				Name: "tcp",
				Check: &envoy_config_core_v3.HealthCheck{
					HealthChecker: &envoy_config_core_v3.HealthCheck_TcpHealthCheck_{
						TcpHealthCheck: &envoy_config_core_v3.HealthCheck_TcpHealthCheck{
							Send: &envoy_config_core_v3.HealthCheck_Payload{
								Payload: &envoy_config_core_v3.HealthCheck_Payload_Text{
									Text: "AAAA",
								},
							},
							Receive: []*envoy_config_core_v3.HealthCheck_Payload{
								{
									Payload: &envoy_config_core_v3.HealthCheck_Payload_Text{
										Text: "AAAA",
									},
								},
							},
						},
					},
				},
			},
		}

		for _, envoyHealthCheckTest := range tests {
			envoyHealthCheckTest := envoyHealthCheckTest

			It(envoyHealthCheckTest.Name, func() {
				// by default we disable panic mode
				// this purpose of this test is to verify panic modes behavior so we need to enable it
				envoyInstance.EnablePanicMode()

				// get the upstream
				us, err := testClients.UpstreamClient.Read(tu.Upstream.Metadata.Namespace, tu.Upstream.Metadata.Name, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				// update the health check configuration
				envoyHealthCheckTest.Check.Timeout = translator.DefaultHealthCheckTimeout
				envoyHealthCheckTest.Check.Interval = translator.DefaultHealthCheckInterval
				envoyHealthCheckTest.Check.HealthyThreshold = translator.DefaultThreshold
				envoyHealthCheckTest.Check.UnhealthyThreshold = translator.DefaultThreshold

				// persist the health check configuration
				us.HealthChecks, err = api_conversion.ToGlooHealthCheckList([]*envoy_config_core_v3.HealthCheck{envoyHealthCheckTest.Check})
				Expect(err).NotTo(HaveOccurred())

				_, err = testClients.UpstreamClient.Write(us, clients.WriteOpts{OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				vs := getGrpcTranscoderVs(writeNamespace, tu.Upstream.Metadata.Ref())
				_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				// ensure that a request fails the health check but is handled by the upstream anyway
				testRequest := basicReq([]byte(`"foo"`))
				Eventually(testRequest, 30, 1).Should(Equal(`{"str":"foo"}`))

				Eventually(tu.C, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
					"GRPCRequest": PointTo(Equal(glootest.TestRequest{Str: "foo"})),
				}))))
			})
		}

		It("outlier detection", func() {
			us, err := testClients.UpstreamClient.Read(tu.Upstream.Metadata.Namespace, tu.Upstream.Metadata.Name, clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())
			us.OutlierDetection = api_conversion.ToGlooOutlierDetection(&envoy_config_cluster_v3.OutlierDetection{
				Interval: &duration.Duration{Seconds: 1},
			})

			_, err = testClients.UpstreamClient.Write(us, clients.WriteOpts{
				OverwriteExisting: true,
			})
			Expect(err).NotTo(HaveOccurred())

			vs := getGrpcTranscoderVs(writeNamespace, tu.Upstream.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			body := []byte(`"foo"`)

			testRequest := basicReq(body)

			Eventually(testRequest, 30, 1).Should(Equal(`{"str":"foo"}`))

			Eventually(tu.C, DefaultEventuallyTimeout, DefaultEventuallyPollingInterval).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
				"GRPCRequest": PointTo(Equal(glootest.TestRequest{Str: "foo"})),
			}))))
		})
	})

	// This test can be run locally by setting INVALID_TEST_REQS=run, to bypass this ValidateRequirements method in the BeforeEach
	Context("translates and persists health checkers", func() {
		var healthCheck *envoy_config_core_v3.HealthCheck

		getUpstreamWithMethod := func(method v3.RequestMethod) *v1helpers.TestUpstream {
			upstream := v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
			healthCheck = &envoy_config_core_v3.HealthCheck{
				Timeout:            translator.DefaultHealthCheckTimeout,
				Interval:           translator.DefaultHealthCheckInterval,
				HealthyThreshold:   translator.DefaultThreshold,
				UnhealthyThreshold: translator.DefaultThreshold,
				HealthChecker: &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
						Path:   "health",
						Method: envoy_config_core_v3.RequestMethod(method),
					},
				},
			}
			var err error
			upstream.Upstream.HealthChecks, err = api_conversion.ToGlooHealthCheckList([]*envoy_config_core_v3.HealthCheck{healthCheck})
			Expect(err).To(Not(HaveOccurred()))
			return upstream
		}

		//Patch the upstream with a given http method then check for expected envoy config
		patchUpstreamAndCheckConfig := func(method v3.RequestMethod, expectedConfig string) {
			err := helpers.PatchResource(ctx, tu.Upstream.Metadata.Ref(), func(resource resources.Resource) resources.Resource {
				upstream := resource.(*gloov1.Upstream)
				upstream.GetHealthChecks()[0].GetHttpHealthCheck().Method = method
				return upstream
			}, testClients.UpstreamClient.BaseClient())
			Expect(err).ToNot(HaveOccurred())

			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return testClients.UpstreamClient.Read(tu.Upstream.Metadata.Namespace, tu.Upstream.Metadata.Name, clients.ReadOpts{})
			})

			Eventually(func(g Gomega) {
				envoyConfig, err := envoyInstance.ConfigDump()
				g.Expect(err).To(Not(HaveOccurred()))

				// Get "http_health_check" and its contents out of the envoy config dump
				http_health_check := regexp.MustCompile(`(?sU)("http_health_check": {).*(})`).FindString(envoyConfig)
				g.Expect(http_health_check).To(ContainSubstring(expectedConfig))
			}, "10s", "1s").ShouldNot(HaveOccurred())
		}
		It("with different methods", func() {
			tu = getUpstreamWithMethod(v3.RequestMethod_METHOD_UNSPECIFIED)

			_, err := testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
			_, err = testClients.VirtualServiceClient.Write(getTrivialVirtualServiceForUpstream(writeNamespace, tu.Upstream.Metadata.Ref()), clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				return testClients.ProxyClient.Read(writeNamespace, gwdefaults.GatewayProxyName, clients.ReadOpts{})
			})

			By("default", func() { patchUpstreamAndCheckConfig(v3.RequestMethod_METHOD_UNSPECIFIED, `"path": "health`) })
			By("POST", func() { patchUpstreamAndCheckConfig(v3.RequestMethod_POST, `"method": "POST"`) })
			By("GET", func() { patchUpstreamAndCheckConfig(v3.RequestMethod_GET, `"method": "GET"`) })

			//We expect a health checker with the CONNECT method to be rejected and the prior health check to be retained
			By("CONNECT", func() { patchUpstreamAndCheckConfig(v3.RequestMethod_CONNECT, `"method": "GET"`) })
		})
	})

	Context("e2e + GRPC", func() {

		BeforeEach(func() {

			tu = v1helpers.NewTestGRPCUpstream(ctx, envoyInstance.LocalAddr(), 5)
			_, err := testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error { return envoyInstance.DisablePanicMode() }, time.Second*5, time.Second/4).Should(BeNil())

			tu = v1helpers.NewTestGRPCUpstream(ctx, envoyInstance.LocalAddr(), 5)
			_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			us, err := testClients.UpstreamClient.Read(tu.Upstream.Metadata.Namespace, tu.Upstream.Metadata.Name, clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			us.HealthChecks, err = api_conversion.ToGlooHealthCheckList([]*envoy_config_core_v3.HealthCheck{
				{
					Timeout:            translator.DefaultHealthCheckTimeout,
					Interval:           translator.DefaultHealthCheckInterval,
					UnhealthyThreshold: translator.DefaultThreshold,
					HealthyThreshold:   translator.DefaultThreshold,
					HealthChecker: &envoy_config_core_v3.HealthCheck_GrpcHealthCheck_{
						GrpcHealthCheck: &envoy_config_core_v3.HealthCheck_GrpcHealthCheck{
							ServiceName: "TestService",
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = testClients.UpstreamClient.Write(us, clients.WriteOpts{
				OverwriteExisting: true,
			})
			Expect(err).NotTo(HaveOccurred())

			vs := getGrpcTranscoderVs(writeNamespace, tu.Upstream.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("Fail all but one GRPC health check", func() {
			liveService := tu.FailGrpcHealthCheck()
			body := []byte(`"foo"`)
			testRequest := basicReq(body)

			numRequests := 5

			for i := 0; i < numRequests; i++ {
				Eventually(testRequest, 30, 1).Should(Equal(`{"str":"foo"}`))
			}

			for i := 0; i < numRequests; i++ {
				select {
				case v := <-tu.C:
					Expect(v.Port).To(Equal(liveService.Port))
				case <-time.After(5 * time.Second):
					Fail("channel did not receive proper response in time")
				}
			}
		})
	})

})

func getGrpcTranscoderVs(writeNamespace string, usRef *core.ResourceRef) *gatewayv1.VirtualService {
	return &gatewayv1.VirtualService{
		Metadata: &core.Metadata{
			Name:      "default",
			Namespace: writeNamespace,
		},
		VirtualHost: &gatewayv1.VirtualHost{
			Routes: []*gatewayv1.Route{
				{
					Matchers: []*matchers.Matcher{{
						PathSpecifier: &matchers.Matcher_Prefix{
							// the grpc_json transcoding filter clears the cache so it no longer would match on /test (this can be configured)
							Prefix: "/",
						},
					}},
					Action: &gatewayv1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: usRef,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
