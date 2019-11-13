package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"

	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_web"
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

			// wait for the two gateways to be created.
			Eventually(func() (gatewayv1.GatewayList, error) {
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
					httpGateway.Options = &gloov1.HttpListenerOptions{
						GrpcWeb: &grpc_web.GrpcWeb{
							Disable: true,
						},
					}
				}

				_, err := gatewayClient.Write(g, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())
			}

			// write a virtual service so we have a proxy
			vs := getTrivialVirtualServiceForUpstream("gloo-system", core.ResourceRef{Name: "test", Namespace: "test"})
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// make sure it propagates to proxy
			Eventually(
				func() (int, error) {
					numdisable := 0
					proxy, err := testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return 0, err
					}
					for _, l := range proxy.Listeners {
						if h := l.GetHttpListener(); h != nil {
							if p := h.GetOptions(); p != nil {
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
			vs := getTrivialVirtualServiceForService("gloo-system", kubeutils.FromKubeMeta(svc.ObjectMeta).Ref(), uint32(svc.Spec.Ports[0].Port))
			_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			// Wait for proxy to be accepted
			var proxy *gloov1.Proxy
			Eventually(func() bool {
				proxy, err = testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
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

				err = envoyInstance.RunWithRole(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				if envoyInstance != nil {
					_ = envoyInstance.Clean()
				}
			})

			It("should work with no ssl", func() {
				up := tu.Upstream
				vs := getTrivialVirtualServiceForUpstream("gloo-system", up.Metadata.Ref())
				_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				TestUpstreamReachable()
			})

			It("should not match requests that contain a header that is excluded from match", func() {
				up := tu.Upstream
				vs := getTrivialVirtualServiceForUpstream("gloo-system", up.Metadata.Ref())
				_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				// Create a regular request
				request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/", defaults.HttpPort), nil)
				Expect(err).NotTo(HaveOccurred())
				request = request.WithContext(ctx)

				// Check that we can reach the upstream
				client := &http.Client{}
				Eventually(func() int {
					response, err := client.Do(request)
					if err != nil {
						return 0
					}
					return response.StatusCode
				}, 5*time.Second, 500*time.Millisecond).Should(Equal(200))

				// Add the header that we are explicitly excluding from the match
				request.Header = map[string][]string{"this-header-must-not-be-present": {"some-value"}}

				// We should get a 404
				Consistently(func() int {
					response, err := client.Do(request)
					if err != nil {
						return 0
					}
					return response.StatusCode
				}, time.Second, 200*time.Millisecond).Should(Equal(404))
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
					vs := getTrivialVirtualServiceForUpstream("gloo-system", up.Metadata.Ref())
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
		VirtualHost: &gatewayv1.VirtualHost{
			Domains: []string{"*"},
			Routes: []*gatewayv1.Route{{
				Action: &gatewayv1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Single{
							Single: &gloov1.Destination{},
						},
					},
				},
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/",
						},
						Headers: []*matchers.HeaderMatcher{
							{
								Name:        "this-header-must-not-be-present",
								InvertMatch: true,
							},
						},
					},
				},
			}},
		},
	}
}
