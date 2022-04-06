package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/golang/protobuf/ptypes/duration"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	glootest "github.com/solo-io/gloo/test/v1helpers/test_grpc_service/glootest/protos"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Health Checks", func() {
	var (
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		envoyInstance  *services.EnvoyInstance
		tu             *v1helpers.TestUpstream
		writeNamespace string
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		writeNamespace = defaults.GlooSystem
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
				Discovery: &gloov1.Settings_DiscoveryOptions{
					FdsMode: gloov1.Settings_DiscoveryOptions_BLACKLIST,
				},
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)
		err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gwdefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
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
			body, err := ioutil.ReadAll(res.Body)
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

				vs := getGrpcVs(writeNamespace, tu.Upstream.Metadata.Ref())
				_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				// ensure that a request fails the health check but is handled by the upstream anyway
				testRequest := basicReq([]byte(`{"str": "foo"}`))
				Eventually(testRequest, 30, 1).Should(Equal(`{"str":"foo"}`))

				Eventually(tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
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

			vs := getGrpcVs(writeNamespace, tu.Upstream.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			body := []byte(`{"str": "foo"}`)

			testRequest := basicReq(body)

			Eventually(testRequest, 30, 1).Should(Equal(`{"str":"foo"}`))

			Eventually(tu.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
				"GRPCRequest": PointTo(Equal(glootest.TestRequest{Str: "foo"})),
			}))))
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

			vs := getGrpcVs(writeNamespace, tu.Upstream.Metadata.Ref())
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("Fail all but one GRPC health check", func() {
			liveService := tu.FailGrpcHealthCheck()
			body := []byte(`{"str": "foo"}`)
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
