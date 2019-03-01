package e2e_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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

		It("should create 2 gateway", func() {

			gatewaycli := testClients.GatewayClient

			Eventually(func() (gatewayv1.GatewayList, error) { return gatewaycli.List(writeNamespace, clients.ListOpts{}) }, "10s", "0.1s").Should(HaveLen(2))
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

			TestUpstremReachable := func() {
				v1helpers.TestUpstremReachable(envoyPort, tu, nil)
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
					envoyInstance.Clean()
				}
			})

			It("should work with no ssl", func() {
				up := tu.Upstream
				vscli := testClients.VirtualServiceClient
				vs := getTrivialVirtualServiceForUpstream("default", up.Metadata.Ref())
				_, err := vscli.Write(vs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				TestUpstremReachable()
			})
			Context("ssl", func() {
				BeforeEach(func() {
					envoyPort = uint32(defaults.HttpsPort)
				})

				TestUpstremSslReachable := func() {
					cert := gloohelpers.Certificate()
					v1helpers.TestUpstremReachable(envoyPort, tu, &cert)
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
								Upstream: upstream,
							},
						},
					},
				},
			}},
		},
	}

}
