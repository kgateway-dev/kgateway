package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/translator"

	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"

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

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()
		defaults.TcpPort = services.NextBindPort()
	})

	AfterEach(func() {
		cancel()
	})

	Describe("in memory", func() {

		BeforeEach(func() {
			validationPort := services.AllocateGlooPort()
			writeNamespace = "gloo-system"
			ro := &services.RunOptions{
				NsToWrite: writeNamespace,
				NsToWatch: []string{"default", writeNamespace},
				WhatToRun: services.What{
					DisableFds: true,
					DisableUds: true,
				},
				ValidationPort: validationPort,
				Settings: &gloov1.Settings{
					Gateway: &gloov1.GatewayOptions{
						Validation: &gloov1.GatewayOptions_ValidationOptions{
							ProxyValidationServerAddr: fmt.Sprintf("127.0.0.1:%v", validationPort),
						},
					},
				},
			}

			testClients = services.RunGlooGatewayUdsFds(ctx, ro)
		})

		Context("http gateway", func() {

			BeforeEach(func() {
				err := gloohelpers.WriteDefaultGateways(writeNamespace, testClients.GatewayClient)
				Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")

				// wait for the two gateways to be created.
				Eventually(func() (gatewayv1.GatewayList, error) {
					return testClients.GatewayClient.List(writeNamespace, clients.ListOpts{})
				}, "10s", "0.1s").Should(HaveLen(2), "Gateways should be present")
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
				vs := getTrivialVirtualServiceForUpstream("gloo-system", &core.ResourceRef{Name: "test", Namespace: "test"})
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

			It("should create 2 gateways", func() {
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
					return proxy.GetStatus().GetState() == core.Status_Accepted
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

			It("won't allow a bad authconfig in a virtualservice to block updates to a gateway", func() {
				// Test the following scenario:
				// 1 VS is written with good config
				// A 2nd VS is written with bad config (refers to an authConfig which doesn't exist)
				// A 3rd VS is written with good config
				// Expected Behavior:
				// Final Proxy should have VS 1 & 3, reporting no errors
				// The gateway output by these three VS's should also have no errors
				// The 2nd VS should have an error complaining about the auth config not being found

				// Create a service so gloo can generate "fake" upstreams for it
				svc := kubernetes.NewService("default", "my-service")
				svc.Spec = corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 1234}}}
				svc, err := testClients.ServiceClient.Write(svc, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				// Create a trivial, working service with a route pointing to the above service
				vs1 := getTrivialVirtualServiceForService(defaults.GlooSystem, kubeutils.FromKubeMeta(svc.ObjectMeta).Ref(), uint32(svc.Spec.Ports[0].Port))
				vs1.VirtualHost.Domains = []string{"vs1"}
				vs1.Metadata.Name = "vs1"
				_, err = testClients.VirtualServiceClient.Write(vs1, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				// Wait for proxy to be accepted
				var proxy *gloov1.Proxy
				Eventually(func() bool {
					proxy, err = testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return false
					}
					return proxy.GetStatus().GetState() == core.Status_Accepted
				}, "60s", "2s").Should(BeTrue(), "first virtualservice should be accepted")

				// Create a second vs with a bad authconfig
				vs2 := getTrivialVirtualServiceForService(defaults.GlooSystem, kubeutils.FromKubeMeta(svc.ObjectMeta).Ref(), uint32(svc.Spec.Ports[0].Port))
				vs2.VirtualHost.Domains = []string{"vs2"}
				vs2.Metadata.Name = "vs2"
				vs2.VirtualHost.Options = &gloov1.VirtualHostOptions{
					Extauth: &extauthv1.ExtAuthExtension{
						Spec: &extauthv1.ExtAuthExtension_ConfigRef{
							ConfigRef: &core.ResourceRef{
								Name:      "bad-authconfig-doesnt-exist",
								Namespace: defaults.GlooSystem,
							},
						},
					},
				}

				// Write vs2
				_, err = testClients.VirtualServiceClient.Write(vs2, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				// Check that virtualservice is reporting an error because of missing authconfig:
				Eventually(func() bool {
					vs, err := testClients.VirtualServiceClient.Read(writeNamespace, "vs2", clients.ReadOpts{})
					if err != nil {
						return false
					}

					return vs.GetStatus().GetState() == core.Status_Rejected
				}, "30s", "1s").Should(BeTrue(), fmt.Sprintf("second virtualservice should be rejected due to missing authconfig"))

				Consistently(func() bool {
					gateway, err := testClients.GatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return false
					}
					return gateway.GetStatus().GetState() == core.Status_Accepted
				}, "10s", "0.1s").Should(BeTrue(), "gateway should not have any errors from a bad VS")

				Eventually(func() bool {
					proxy, err = testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return false
					}
					nonSslListener := getNonSSLListener(proxy)

					return proxy.GetStatus().GetState() == core.Status_Accepted && len(nonSslListener.GetHttpListener().VirtualHosts) == 1
				}, "10s", "0.1s").Should(BeTrue(), "second virtualservice should not end up in the proxy (bad config)")

				// Create a third trivial vs with valid config
				vs3 := getTrivialVirtualServiceForService(defaults.GlooSystem, kubeutils.FromKubeMeta(svc.ObjectMeta).Ref(), uint32(svc.Spec.Ports[0].Port))
				vs3.Metadata.Name = "vs3"
				vs3.VirtualHost.Domains = []string{"vs3"}
				_, err = testClients.VirtualServiceClient.Write(vs3, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				// Wait for proxy to be accepted
				Eventually(func() bool {
					proxy, err = testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return false

					}
					nonSslListener := getNonSSLListener(proxy)

					return proxy.GetStatus().GetState() == core.Status_Accepted && len(nonSslListener.GetHttpListener().VirtualHosts) == 2
				}, "10s", "0.1s").Should(BeTrue(), "third virtualservice should end up in the proxy (good config)")

				// Verify that the proxy is as expected (2 functional virtualservices)
				Expect(proxy.Listeners).To(HaveLen(2))
				nonSslListener := getNonSSLListener(proxy)
				httpListener := nonSslListener.GetHttpListener()

				Expect(httpListener).NotTo(BeNil(), "should have a (non-ssl) http listener")
				Expect(httpListener.VirtualHosts).To(HaveLen(2), "should have 2 virtualHosts")
				Expect(httpListener.VirtualHosts[0].Domains[0]).To(Equal("vs1"), "should have vs1")
				Expect(httpListener.VirtualHosts[1].Domains[0]).To(Equal("vs3"), "should have vs3")
				Expect(httpListener.VirtualHosts[0].Routes).To(HaveLen(1), "should have 1 route in each host")
				Expect(httpListener.VirtualHosts[1].Routes).To(HaveLen(1), "should have 1 route in each host")

				// Make sure the routes are as expected:
				allRoutes := []*gloov1.Route{httpListener.VirtualHosts[0].Routes[0], httpListener.VirtualHosts[1].Routes[0]}
				for _, route := range allRoutes {
					Expect(route.GetRouteAction()).NotTo(BeNil())
					Expect(route.GetRouteAction().GetSingle()).NotTo(BeNil())
					service := route.GetRouteAction().GetSingle().GetKube()
					Expect(service.Ref.Namespace).To(Equal(svc.Namespace))
					Expect(service.Ref.Name).To(Equal(svc.Name))
					Expect(service.Port).To(BeEquivalentTo(svc.Spec.Ports[0].Port))
				}
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
					var err error
					envoyInstance, err = envoyFactory.NewEnvoyInstance()
					Expect(err).NotTo(HaveOccurred())

					tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

					_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())
					err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					if envoyInstance != nil {
						_ = envoyInstance.Clean()
					}
				})

				It("works when rapid virtual service creation and deletion causes no race conditions", func() {
					up := tu.Upstream
					vs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())

					// Write the Virtual Service
					_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					// Wait for proxy to be created
					var proxyList gloov1.ProxyList
					Eventually(func() bool {
						proxyList, err = testClients.ProxyClient.List(writeNamespace, clients.ListOpts{})
						if err != nil {
							return false
						}
						return len(proxyList) == 1
					}, "20s", "1s").Should(BeTrue())

					TestUpstreamReachable()

					// Delete the Virtual Service
					err = testClients.VirtualServiceClient.Delete(writeNamespace, vs.GetMetadata().Name, clients.DeleteOpts{})
					Expect(err).NotTo(HaveOccurred())

					// The vs should be deleted
					var vsList gatewayv1.VirtualServiceList
					Eventually(func() bool {
						vsList, err = testClients.VirtualServiceClient.List(writeNamespace, clients.ListOpts{})
						if err != nil {
							return false
						}
						if len(vsList) != 0 {
							testClients.VirtualServiceClient.Delete(writeNamespace, vs.GetMetadata().Name, clients.DeleteOpts{})
							return false
						}
						return true
					}, "10s", "0.5s").Should(BeTrue())
					Consistently(func() bool {
						vsList, err = testClients.VirtualServiceClient.List(writeNamespace, clients.ListOpts{})
						if err != nil {
							return false
						}
						return len(vsList) == 0
					}, "10s", "0.5s").Should(BeTrue())
				})

				It("should work with no ssl and clean up the envoy config when the virtual service is deleted", func() {
					up := tu.Upstream
					vs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
					_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					TestUpstreamReachable()

					// Delete the Virtual Service
					err = testClients.VirtualServiceClient.Delete(writeNamespace, vs.GetMetadata().Name, clients.DeleteOpts{})
					Expect(err).NotTo(HaveOccurred())

					// Wait for proxy to be deleted
					var proxyList gloov1.ProxyList
					Eventually(func() bool {
						proxyList, err = testClients.ProxyClient.List(writeNamespace, clients.ListOpts{})
						if err != nil {
							return false
						}
						return len(proxyList) == 0
					}, "10s", "0.1s").Should(BeTrue())

					// Create a regular request
					request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d", defaults.HttpPort), nil)
					Expect(err).NotTo(HaveOccurred())
					request = request.WithContext(ctx)

					// Check that we can no longer reach the upstream
					client := &http.Client{}
					Eventually(func() int {
						response, err := client.Do(request)
						if err != nil {
							return 503
						}
						return response.StatusCode
					}, 20*time.Second, 500*time.Millisecond).Should(Equal(503))
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
					}, 20*time.Second, 500*time.Millisecond).Should(Equal(200))

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

				It("should direct requests that use cluster_header to the proper upstream", func() {
					// Construct upstream name {{name}}_{{namespace}}
					us := tu.Upstream
					upstreamName := translator.UpstreamToClusterName(us.Metadata.Ref())

					vs := getTrivialVirtualService("gloo-system")
					// Create route that uses cluster header destination
					vs.GetVirtualHost().Routes = []*gatewayv1.Route{{
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_ClusterHeader{
									ClusterHeader: "cluster-header-name",
								},
							},
						}}}

					_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					// Create a regular request
					request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/", defaults.HttpPort), nil)
					Expect(err).NotTo(HaveOccurred())
					request = request.WithContext(context.TODO())
					request.Header.Add("cluster-header-name", upstreamName)

					// Check that we can reach the upstream
					client := &http.Client{}
					Eventually(func() (int, error) {
						response, err := client.Do(request)
						if response == nil {
							return 0, err
						}
						return response.StatusCode, err
					}, 10*time.Second, 500*time.Millisecond).Should(Equal(200))
				})

				Context("ssl", func() {

					TestUpstreamSslReachable := func() {
						cert := gloohelpers.Certificate()
						v1helpers.TestUpstreamReachable(defaults.HttpsPort, tu, &cert)
					}

					It("should work with ssl", func() {
						// Check tls inspector has not been added yet
						Eventually(func() (string, error) {
							envoyConfig := ""
							resp, err := envoyInstance.EnvoyConfig()
							if err != nil {
								return "", err
							}
							p := new(bytes.Buffer)
							if _, err := io.Copy(p, resp.Body); err != nil {
								return "", err
							}
							defer resp.Body.Close()
							envoyConfig = p.String()
							return envoyConfig, nil
						}, "10s", "0.1s").Should(Not(MatchRegexp("type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector")))

						secret := &gloov1.Secret{
							Metadata: &core.Metadata{
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

						Eventually(func() (string, error) {
							envoyConfig := ""
							resp, err := envoyInstance.EnvoyConfig()
							if err != nil {
								return "", err
							}
							p := new(bytes.Buffer)
							if _, err := io.Copy(p, resp.Body); err != nil {
								return "", err
							}
							defer resp.Body.Close()
							envoyConfig = p.String()
							return envoyConfig, nil
						}, "10s", "0.1s").Should(MatchRegexp("type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector"))
					})
				})
			})
		})

		Context("tcp gateway", func() {

			var (
				envoyInstance *services.EnvoyInstance
				tu            *v1helpers.TestUpstream
			)

			BeforeEach(func() {
				// Use tcp gateway instead of default
				defaultGateway := gatewaydefaults.DefaultTcpGateway(writeNamespace)
				defaultSslGateway := gatewaydefaults.DefaultTcpSslGateway(writeNamespace)

				_, err := testClients.GatewayClient.Write(defaultGateway, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")
				_, err = testClients.GatewayClient.Write(defaultSslGateway, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred(), "Should be able to write default ssl gateways")

				// wait for the two gateways to be created.
				Eventually(func() (gatewayv1.GatewayList, error) {
					return testClients.GatewayClient.List(writeNamespace, clients.ListOpts{})
				}, "10s", "0.1s").Should(HaveLen(2), "Gateways should be present")

				envoyInstance, err = envoyFactory.NewEnvoyInstance()
				Expect(err).NotTo(HaveOccurred())

				tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

				_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())
				err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				if envoyInstance != nil {
					_ = envoyInstance.Clean()
				}
			})

			Context("ssl", func() {

				TestUpstreamSslReachableTcp := func() {
					cert := gloohelpers.Certificate()
					v1helpers.TestUpstreamReachable(defaults.HttpsPort, tu, &cert)
				}

				It("should work with ssl", func() {
					// Check tls inspector has not been added yet
					Eventually(func() (string, error) {
						envoyConfig := ""
						resp, err := envoyInstance.EnvoyConfig()
						if err != nil {
							return "", err
						}
						p := new(bytes.Buffer)
						if _, err := io.Copy(p, resp.Body); err != nil {
							return "", err
						}
						defer resp.Body.Close()
						envoyConfig = p.String()
						return envoyConfig, nil
					}, "10s", "0.1s").Should(Not(MatchRegexp("type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector")))

					secret := &gloov1.Secret{
						Metadata: &core.Metadata{
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
					createdSecret, err := testClients.SecretClient.Write(secret, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())

					host := &gloov1.TcpHost{
						Name: "one",
						Destination: &gloov1.TcpHost_TcpAction{
							Destination: &gloov1.TcpHost_TcpAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: tu.Upstream.Metadata.Ref(),
									},
								},
							},
						},
						SslConfig: &gloov1.SslConfig{
							SslSecrets: &gloov1.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      createdSecret.Metadata.Name,
									Namespace: createdSecret.Metadata.Namespace,
								},
							},
							AlpnProtocols: []string{"http/1.1"},
						},
					}

					// Update gateway with tcp hosts
					gatewayClient := testClients.GatewayClient
					gw, err := gatewayClient.List(writeNamespace, clients.ListOpts{})
					Expect(err).NotTo(HaveOccurred())

					for _, g := range gw {
						tcpGateway := g.GetTcpGateway()
						if tcpGateway != nil {
							tcpGateway.TcpHosts = []*gloov1.TcpHost{host}
						}

						_, err := gatewayClient.Write(g, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
						Expect(err).NotTo(HaveOccurred())
					}

					// Check tls inspector is correctly configured
					Eventually(func() (string, error) {
						envoyConfig := ""
						resp, err := envoyInstance.EnvoyConfig()
						if err != nil {
							return "", err
						}
						p := new(bytes.Buffer)
						if _, err := io.Copy(p, resp.Body); err != nil {
							return "", err
						}
						defer resp.Body.Close()
						envoyConfig = p.String()
						return envoyConfig, nil
					}, "10s", "0.1s").Should(MatchRegexp("type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector"))

					TestUpstreamSslReachableTcp()
				})
			})

		})
	})
})

func getTrivialVirtualServiceForUpstream(ns string, upstream *core.ResourceRef) *gatewayv1.VirtualService {
	vs := getTrivialVirtualService(ns)
	vs.VirtualHost.Routes[0].GetRouteAction().GetSingle().DestinationType = &gloov1.Destination_Upstream{
		Upstream: upstream,
	}
	return vs
}

func getTrivialVirtualServiceForService(ns string, service *core.ResourceRef, port uint32) *gatewayv1.VirtualService {
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
		Metadata: &core.Metadata{
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

// Given a proxy, reuturns the non-ssl listener from
// that proxy, or nil if it can't be found
func getNonSSLListener(proxy *gloov1.Proxy) *gloov1.Listener {
	for _, l := range proxy.Listeners {
		if l.BindPort == defaults.HttpPort {
			return l
		}
	}
	return nil
}
