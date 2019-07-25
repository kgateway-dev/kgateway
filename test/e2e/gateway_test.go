package e2e_test

import (
	"context"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/als"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewayv2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc_web"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = FDescribe("Gateway", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		writeNamespace string
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
					DisableFds: true,
					DisableUds: true,
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

		It("should disable grpc web filter", func() {

			gatewayClient := testClients.GatewayClient
			gw, err := gatewayClient.List(writeNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())

			for _, g := range gw {
				httpGateway := g.GetHttpGateway()
				if httpGateway != nil {
					httpGateway.Plugins = &gloov1.HttpListenerPlugins{
						GrpcWeb: &grpc_web.GrpcWeb{
							Disable: true,
						},
					}
				}

				_, err := gatewayClient.Write(g, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())
			}

			// write a virtual service so we have a proxy
			vs := getTrivialVirtualServiceForUpstream("default", core.ResourceRef{Name: "test", Namespace: "test"})
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// make sure it propagates to proxy
			Eventually(
				func() (int, error) {
					numdisable := 0
					proxy, err := testClients.ProxyClient.Read(writeNamespace, translator.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return 0, err
					}
					for _, l := range proxy.Listeners {
						if h := l.GetHttpListener(); h != nil {
							if p := h.GetPlugins(); p != nil {
								if grpcweb := p.GetGrpcWeb(); grpcweb != nil {
									if grpcweb.Disable {
										numdisable++
									}
								}
							}
						}
					}
					return numdisable, nil
				}, "5s", "0.1s").Should(Equal(2))

		})

		It("should create 2 gateway", func() {
			gatewaycli := testClients.GatewayClient
			gw, err := gatewaycli.List(writeNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())

			numssl := 0
			if gw[0].Ssl {
				numssl += 1
			}
			if gw[1].Ssl {
				numssl += 1
			}
			Expect(numssl).To(Equal(1))
		})

		It("correctly configures gateway for a virtual service which contains a route to a service", func() {

			// Create a service so gloo can generate "fake" upstreams for it
			svc := kubernetes.NewService("default", "my-service")
			svc.Spec = corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 1234}}}
			svc, err := testClients.ServiceClient.Write(svc, clients.WriteOpts{Ctx: ctx})
			Expect(err).NotTo(HaveOccurred())

			// Create a virtual service with a route pointing to the above service
			vs := getTrivialVirtualServiceForService("default", kubeutils.FromKubeMeta(svc.ObjectMeta).Ref(), uint32(svc.Spec.Ports[0].Port))
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// Wait for proxy to be accepted
			var proxy *gloov1.Proxy
			Eventually(func() bool {
				proxy, err = testClients.ProxyClient.Read(writeNamespace, translator.GatewayProxyName, clients.ReadOpts{})
				if err != nil {
					return false
				}
				return proxy.Status.State == core.Status_Accepted
			}, "100s", "0.1s").Should(BeTrue())

			// Verify that the proxy has the expected route
			Expect(proxy.Listeners).To(HaveLen(2))
			var nonSslListener gloov1.Listener
			for _, l := range proxy.Listeners {
				if l.BindPort == defaults.HttpPort {
					nonSslListener = *l
					break
				}
			}
			Expect(nonSslListener.GetHttpListener()).NotTo(BeNil())
			Expect(nonSslListener.GetHttpListener().VirtualHosts).To(HaveLen(1))
			Expect(nonSslListener.GetHttpListener().VirtualHosts[0].Routes).To(HaveLen(1))
			Expect(nonSslListener.GetHttpListener().VirtualHosts[0].Routes[0].GetRouteAction()).NotTo(BeNil())
			Expect(nonSslListener.GetHttpListener().VirtualHosts[0].Routes[0].GetRouteAction().GetSingle()).NotTo(BeNil())
			service := nonSslListener.GetHttpListener().VirtualHosts[0].Routes[0].GetRouteAction().GetSingle().GetKube()
			Expect(service.Ref.Namespace).To(Equal(svc.Namespace))
			Expect(service.Ref.Name).To(Equal(svc.Name))
			Expect(service.Port).To(BeEquivalentTo(svc.Spec.Ports[0].Port))
		})

		Context("traffic", func() {

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

				err = envoyInstance.RunWithRole(writeNamespace+"~gateway-proxy-v2", testClients.GlooPort)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				if envoyInstance != nil {
					_ = envoyInstance.Clean()
				}
			})

			It("should work with no ssl", func() {
				up := tu.Upstream
				vs := getTrivialVirtualServiceForUpstream("default", up.Metadata.Ref())
				_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				TestUpstreamReachable()
			})

			Context("ssl", func() {

				TestUpstreamSslReachable := func() {
					cert := gloohelpers.Certificate()
					v1helpers.TestUpstreamReachable(defaults.HttpsPort, tu, &cert)
				}

				It("should work with ssl", func() {

					secret := &gloov1.Secret{
						Metadata: core.Metadata{
							Name:      "secret",
							Namespace: "default",
						},
						Kind: &gloov1.Secret_Tls{
							Tls: &gloov1.TlsSecret{
								CertChain:  gloohelpers.Certificate(),
								PrivateKey: gloohelpers.PrivateKey(),
							},
						},
					}
					createdSecret, err := testClients.SecretClient.Write(secret, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					up := tu.Upstream
					vscli := testClients.VirtualServiceClient
					vs := getTrivialVirtualServiceForUpstream("default", up.Metadata.Ref())
					vs.SslConfig = &gloov1.SslConfig{
						SslSecrets: &gloov1.SslConfig_SecretRef{
							SecretRef: &core.ResourceRef{
								Name:      createdSecret.Metadata.Name,
								Namespace: createdSecret.Metadata.Namespace,
							},
						},
					}

					_, err = vscli.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					TestUpstreamSslReachable()
				})
			})

			Context("Access logs", func() {
				var (
					gw *gatewayv2.Gateway
				)

				BeforeEach(func() {
					gatewaycli := testClients.GatewayClient
					var err error
					gw, err = gatewaycli.Read("gloo-system", "gateway", clients.ReadOpts{})
					Expect(err).NotTo(HaveOccurred())
				})
				AfterEach(func() {
					gatewaycli := testClients.GatewayClient
					var err error
					gw, err = gatewaycli.Read("gloo-system", "gateway", clients.ReadOpts{})
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

					gatewaycli := testClients.GatewayClient
					_, err := gatewaycli.Write(gw, clients.WriteOpts{OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())
					up := tu.Upstream
					vs := getTrivialVirtualServiceForUpstream("default", up.Metadata.Ref())
					_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())
					TestUpstreamReachable()



					Eventually(func() error {
						logs, err := envoyInstance.LogsCmd()
						if err != nil {
							return err
						}
						if logs == "" {
							return errors.Errorf("logs should not be empty")
						}
						if !strings.Contains(logs, `"POST /1 HTTP/1.1" 200`) {
							return errors.Errorf("no access logs present")
						}
						return nil
					}, time.Second* 30, time.Second/2).ShouldNot(HaveOccurred())
				})
				It("can create json access logs", func() {
					gw.Plugins = &gloov1.ListenerPlugins{
						AccessLoggingService: &als.AccessLoggingService{
							AccessLog: []*als.AccessLog{
								{
									OutputDestination: &als.AccessLog_FileSink{
										FileSink: &als.FileSink{
											Path: "/dev/stdout",
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
						logs, err := envoyInstance.LogsCmd()
						if err != nil {
							return err
						}
						if logs == "" {
							return errors.Errorf("logs should not be empty")
						}
						if !strings.Contains(logs, `{"method":"POST","protocol":"HTTP/1.1"}`) {
							return errors.Errorf("no access logs present")
						}
						return nil
					}, time.Second* 30, time.Second/2).ShouldNot(HaveOccurred())
				})
			})
		})
	})
})

func getTrivialVirtualServiceForUpstream(ns string, upstream core.ResourceRef) *gatewayv1.VirtualService {
	vs := getTrivialVirtualService(ns)
	vs.VirtualHost.Routes[0].GetRouteAction().GetSingle().DestinationType = &gloov1.Destination_Upstream{
		Upstream: utils.ResourceRefPtr(upstream),
	}
	return vs
}

func getTrivialVirtualServiceForService(ns string, service core.ResourceRef, port uint32) *gatewayv1.VirtualService {
	vs := getTrivialVirtualService(ns)
	vs.VirtualHost.Routes[0].GetRouteAction().GetSingle().DestinationType = &gloov1.Destination_Kube{
		Kube: &gloov1.KubernetesServiceDestination{
			Ref:  service,
			Port: port,
		},
	}
	return vs
}

func getTrivialVirtualService(ns string) *gatewayv1.VirtualService {
	return &gatewayv1.VirtualService{
		Metadata: core.Metadata{
			Name:      "vs",
			Namespace: ns,
		},
		VirtualHost: &gloov1.VirtualHost{
			Name:    "virt1",
			Domains: []string{"*"},
			Routes: []*gloov1.Route{{
				Matcher: &gloov1.Matcher{
					PathSpecifier: &gloov1.Matcher_Prefix{
						Prefix: "/",
					},
				},
				Action: &gloov1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Single{
							Single: &gloov1.Destination{
								DestinationType: nil,
							},
						},
					},
				},
			}},
		},
	}
}
