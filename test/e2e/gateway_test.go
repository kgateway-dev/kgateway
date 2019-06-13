package e2e_test

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc_web"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Gateway", func() {

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
		})

		AfterEach(func() {
			cancel()
		})

		BeforeEach(func() {
			// wait for the two gateways to be created.
			Eventually(func() (gatewayv1.GatewayList, error) {
				return testClients.GatewayClient.List(writeNamespace, clients.ListOpts{})
			}, "10s", "0.1s").Should(HaveLen(2))
		})

		It("should should disable grpc web filter", func() {

			gatewaycli := testClients.GatewayClient
			gw, err := gatewaycli.List(writeNamespace, clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())

			for _, g := range gw {
				g.Plugins = &gloov1.ListenerPlugins{
					GrpcWeb: &grpc_web.GrpcWeb{
						Disable: true,
					},
				}
				_, err := gatewaycli.Write(g, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())
			}

			// write a virtual service so we have a proxy
			vscli := testClients.VirtualServiceClient
			vs := getTrivialVirtualServiceForUpstream("default", core.ResourceRef{Name: "test", Namespace: "test"})
			_, err = vscli.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// make sure it propagates to proxy
			Eventually(
				func() (int, error) {
					numdisable := 0
					proxy, err := testClients.ProxyClient.Read(writeNamespace, "gateway-proxy", clients.ReadOpts{})
					if err != nil {
						return 0, err
					}
					for _, l := range proxy.Listeners {
						if h := l.GetHttpListener(); h != nil {
							if p := h.GetListenerPlugins(); p != nil {
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

		Context("traffic", func() {

			var (
				envoyInstance *services.EnvoyInstance
				tu            *v1helpers.TestUpstream
				envoyPort     uint32
			)

			TestUpstreamReachable := func() {
				v1helpers.TestUpstreamReachable(envoyPort, tu, nil)
			}

			BeforeEach(func() {
				ctx, cancel = context.WithCancel(context.Background())
				var err error
				envoyInstance, err = envoyFactory.NewEnvoyInstance()
				Expect(err).NotTo(HaveOccurred())

				tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

				_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				envoyPort = uint32(defaults.HttpPort)

				err = envoyInstance.RunWithRole(writeNamespace+"~gateway-proxy", testClients.GlooPort)
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
				BeforeEach(func() {
					envoyPort = uint32(defaults.HttpsPort)
				})

				TestUpstremSslReachable := func() {
					cert := gloohelpers.Certificate()
					v1helpers.TestUpstreamReachable(envoyPort, tu, &cert)
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

					TestUpstremSslReachable()
				})
			})
		})
	})
})

func getTrivialVirtualServiceForUpstream(ns string, upstream core.ResourceRef) *gatewayv1.VirtualService {
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
								DestinationType: &gloov1.Destination_Upstream{
									Upstream: utils.ResourceRefPtr(upstream),
								},
							},
						},
					},
				},
			}},
		},
	}

}
