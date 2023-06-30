package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/solo-io/gloo/test/services/envoy"

	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	corev1 "k8s.io/api/core/v1"
)

const (
	tlsInspectorType = "type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector"
)

var _ = Describe("Gateway", func() {

	var (
		ctx         context.Context
		cancel      context.CancelFunc
		testClients services.TestClients

		vsMetric = metrics.Names[gatewayv1.VirtualServiceGVK]
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	Describe("in memory", func() {

		BeforeEach(func() {
			ro := &services.RunOptions{
				NsToWrite: writeNamespace,
				NsToWatch: []string{"default", writeNamespace},
				WhatToRun: services.What{
					DisableFds: true,
					DisableUds: true,
				},
				Settings: &gloov1.Settings{
					// Record the config status for virtual services. Use the resource name as a
					// label on the metric so that a unique time series is tracked for each VS
					ObservabilityOptions: &gloov1.Settings_ObservabilityOptions{
						ConfigStatusMetricLabels: map[string]*metrics.MetricLabels{
							"VirtualService.v1.gateway.solo.io": {
								LabelToPath: map[string]string{
									"name": "{.metadata.name}",
								},
							},
						},
					},
				},
			}

			testClients = services.RunGlooGatewayUdsFds(ctx, ro)
		})

		Context("http gateway", func() {

			var (
				envoyInstance   *envoy.Instance
				defaultGateways []*gatewayv1.Gateway
			)

			BeforeEach(func() {
				envoyInstance = envoyFactory.NewInstance()

				defaultGateway := gatewaydefaults.DefaultGateway(writeNamespace)
				defaultSslGateway := gatewaydefaults.DefaultSslGateway(writeNamespace)

				defaultGateways = []*gatewayv1.Gateway{
					defaultGateway,
					defaultSslGateway,
				}
			})

			JustBeforeEach(func() {
				for _, gw := range defaultGateways {
					_, err := testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())
				}

				// wait for the two gateways to be created.
				Eventually(func() (gatewayv1.GatewayList, error) {
					return testClients.GatewayClient.List(writeNamespace, clients.ListOpts{Ctx: ctx})
				}, "10s", "0.1s").Should(HaveLen(2), "Gateways should be present")
			})

			JustAfterEach(func() {
				for _, gw := range defaultGateways {
					err := testClients.GatewayClient.Delete(gw.GetMetadata().GetNamespace(), gw.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("should create 2 gateways (1 ssl)", func() {
				gw, err := testClients.GatewayClient.List(writeNamespace, clients.ListOpts{Ctx: ctx})
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
				vs := getTrivialVirtualServiceForService(writeNamespace, kubeutils.FromKubeMeta(svc.ObjectMeta, true).Ref(), uint32(svc.Spec.Ports[0].Port))
				_, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				// Wait for proxy to be accepted
				var proxy *gloov1.Proxy
				gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					var err error
					proxy, err = testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return nil, err
					}
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

					return proxy, err
				})

				// clean up the virtual service that we created
				err = testClients.VirtualServiceClient.Delete(vs.GetMetadata().GetNamespace(), vs.GetMetadata().GetName(), clients.DeleteOpts{})
				Expect(err).NotTo(HaveOccurred())

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

				// Create a trivial, working VS with a route pointing to the above service
				vs1 := getTrivialVirtualServiceForService(writeNamespace, kubeutils.FromKubeMeta(svc.ObjectMeta, true).Ref(), uint32(svc.Spec.Ports[0].Port))
				vs1.VirtualHost.Domains = []string{"vs1"}
				vs1.Metadata.Name = "vs1"
				_, err = testClients.VirtualServiceClient.Write(vs1, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				// Wait for proxy to be accepted
				var proxy *gloov1.Proxy
				gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					proxy, err = testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					return proxy, err
				})

				// Create a second vs with a bad authconfig
				vs2 := getTrivialVirtualServiceForService(defaults.GlooSystem, kubeutils.FromKubeMeta(svc.ObjectMeta, true).Ref(), uint32(svc.Spec.Ports[0].Port))
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
				gloohelpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
					return testClients.VirtualServiceClient.Read(writeNamespace, "vs2", clients.ReadOpts{})
				})

				gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					return testClients.GatewayClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				})

				By("second virtualservice should not end up in the proxy (bad config)")
				gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					proxy, err = testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return nil, err
					}
					nonSslListener := getNonSSLListener(proxy)
					vhostCount := len(nonSslListener.GetHttpListener().VirtualHosts)
					if vhostCount == 1 {
						return proxy, nil
					}

					return nil, errors.Errorf("non-ssl listener virtual hosts: expected 1, found %d ", vhostCount)
				})

				// Create a third trivial vs with valid config
				vs3 := getTrivialVirtualServiceForService(defaults.GlooSystem, kubeutils.FromKubeMeta(svc.ObjectMeta, true).Ref(), uint32(svc.Spec.Ports[0].Port))
				vs3.Metadata.Name = "vs3"
				vs3.VirtualHost.Domains = []string{"vs3"}
				_, err = testClients.VirtualServiceClient.Write(vs3, clients.WriteOpts{})
				Expect(err).NotTo(HaveOccurred())

				// Wait for proxy to be accepted
				By("third virtualservice should end up in the proxy (good config)")
				gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					proxy, err = testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return nil, err
					}
					nonSslListener := getNonSSLListener(proxy)
					vhostCount := len(nonSslListener.GetHttpListener().VirtualHosts)
					if vhostCount == 2 {
						return proxy, nil
					}

					return nil, errors.Errorf("non-ssl listener virtual hosts: expected 2, found %d ", vhostCount)
				})

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

				// Make sure each virtual service's status metric is as expected:
				Expect(gloohelpers.ReadMetricByLabel(vsMetric, "name", "vs1")).To(Equal(0))
				Expect(gloohelpers.ReadMetricByLabel(vsMetric, "name", "vs2")).To(Equal(1))
				Expect(gloohelpers.ReadMetricByLabel(vsMetric, "name", "vs3")).To(Equal(0))
			})

			Context("traffic", func() {

				var (
					testUpstream *v1helpers.TestUpstream
				)

				TestUpstreamReachable := func() {
					v1helpers.TestUpstreamReachable(envoyInstance.HttpPort, testUpstream, nil)
				}

				BeforeEach(func() {
					testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

					err := envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
					Expect(err).NotTo(HaveOccurred())
				})

				JustBeforeEach(func() {
					_, err := testClients.UpstreamClient.Write(testUpstream.Upstream, clients.WriteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())
				})

				JustAfterEach(func() {
					err := testClients.UpstreamClient.Delete(testUpstream.Upstream.GetMetadata().GetNamespace(), testUpstream.Upstream.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					envoyInstance.Clean()
				})

				It("works when rapid virtual service creation and deletion causes no race conditions", func() {
					vs := getTrivialVirtualServiceForUpstream(writeNamespace, testUpstream.Upstream.Metadata.Ref())

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
					vs := getTrivialVirtualServiceForUpstream(writeNamespace, testUpstream.Upstream.Metadata.Ref())
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
					request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d", envoyInstance.HttpPort), nil)
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
					up := testUpstream.Upstream
					vs := getTrivialVirtualServiceForUpstream("gloo-system", up.Metadata.Ref())
					_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					// Create a regular request
					request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/", envoyInstance.HttpPort), nil)
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
					upstreamName := translator.UpstreamToClusterName(testUpstream.Upstream.Metadata.Ref())

					vs := getTrivialVirtualService(writeNamespace)
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
					request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/", envoyInstance.HttpPort), nil)
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

					var secret *gloov1.Secret

					BeforeEach(func() {
						secret = &gloov1.Secret{
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
					})

					JustBeforeEach(func() {
						_, err := testClients.SecretClient.Write(secret, clients.WriteOpts{Ctx: ctx})
						Expect(err).NotTo(HaveOccurred())
					})

					JustAfterEach(func() {
						err := testClients.SecretClient.Delete(secret.GetMetadata().GetNamespace(), secret.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
						Expect(err).NotTo(HaveOccurred())
					})

					TestUpstreamSslReachable := func() {
						cert := gloohelpers.Certificate()
						v1helpers.TestUpstreamReachable(envoyInstance.HttpsPort, testUpstream, &cert)
					}

					It("should work with ssl", func() {
						// Check tls inspector has not been added yet
						Eventually(envoyInstance.ConfigDump, "10s", "0.1s").Should(Not(MatchRegexp(tlsInspectorType)))

						vs := getTrivialVirtualServiceForUpstream(writeNamespace, testUpstream.Upstream.Metadata.Ref())
						vs.SslConfig = &ssl.SslConfig{
							SslSecrets: &ssl.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      secret.GetMetadata().GetName(),
									Namespace: secret.GetMetadata().GetNamespace(),
								},
							},
						}

						_, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{})
						Expect(err).NotTo(HaveOccurred())

						TestUpstreamSslReachable()

						Eventually(envoyInstance.ConfigDump, "10s", "0.1s").Should(MatchRegexp(tlsInspectorType))
					})
				})
			})
		})

		Context("tcp gateway", func() {

			var (
				defaultGateways []*gatewayv1.Gateway
				envoyInstance   *envoy.Instance
				tu              *v1helpers.TestUpstream
			)

			BeforeEach(func() {
				envoyInstance = envoyFactory.NewInstance()

				// Use tcp gateway instead of default
				// Resources need to be created after the Envoy Instance because the port is dynamically allocated
				defaultGateway := gatewaydefaults.DefaultTcpGateway(writeNamespace)
				defaultSslGateway := gatewaydefaults.DefaultTcpSslGateway(writeNamespace)

				defaultGateways = []*gatewayv1.Gateway{
					defaultGateway,
					defaultSslGateway,
				}

				tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

				err := envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
				Expect(err).NotTo(HaveOccurred())
			})

			JustBeforeEach(func() {
				for _, gw := range defaultGateways {
					_, err := testClients.GatewayClient.Write(gw, clients.WriteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")
				}

				_, err := testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())
			})

			JustAfterEach(func() {
				for _, gw := range defaultGateways {
					err := testClients.GatewayClient.Delete(gw.GetMetadata().GetNamespace(), gw.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred(), "Should be able to delete default gateways")
				}
			})

			AfterEach(func() {
				envoyInstance.Clean()
			})

			Context("ssl", func() {

				var secret *gloov1.Secret

				BeforeEach(func() {
					secret = &gloov1.Secret{
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
				})

				JustBeforeEach(func() {
					_, err := testClients.SecretClient.Write(secret, clients.WriteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())
				})

				JustAfterEach(func() {
					err := testClients.SecretClient.Delete(secret.GetMetadata().GetNamespace(), secret.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())
				})

				TestUpstreamSslReachableTcp := func() {
					cert := gloohelpers.Certificate()
					v1helpers.TestUpstreamReachable(envoyInstance.HttpsPort, tu, &cert)
				}

				It("should work with ssl", func() {
					// Check tls inspector has not been added yet
					Eventually(envoyInstance.ConfigDump, "10s", "0.1s").Should(Not(MatchRegexp(tlsInspectorType)))

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
						SslConfig: &ssl.SslConfig{
							SslSecrets: &ssl.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      secret.GetMetadata().GetName(),
									Namespace: secret.GetMetadata().GetNamespace(),
								},
							},
							AlpnProtocols: []string{"http/1.1"},
						},
					}

					// Update gateway with tcp hosts
					tcpGatewayRef := gatewaydefaults.DefaultTcpSslGateway(writeNamespace).GetMetadata().Ref()
					err := gloohelpers.PatchResource(
						ctx,
						tcpGatewayRef,
						func(resource resources.Resource) resources.Resource {
							gw := resource.(*gatewayv1.Gateway)
							gw.GetTcpGateway().TcpHosts = []*gloov1.TcpHost{host}
							return gw
						},
						testClients.GatewayClient.BaseClient(),
					)
					Expect(err).NotTo(HaveOccurred())

					// Check tls inspector is correctly configured
					Eventually(envoyInstance.ConfigDump, "10s", "0.1s").Should(MatchRegexp(tlsInspectorType))

					TestUpstreamSslReachableTcp()
				})
			})

			Context("proxyProtocol", func() {
				var (
					secret *gloov1.Secret
				)

				BeforeEach(func() {

					secret = &gloov1.Secret{
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
				})

				JustBeforeEach(func() {
					_, err := testClients.SecretClient.Write(secret, clients.WriteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())
					tu.Upstream.ProxyProtocolVersion = &wrapperspb.StringValue{Value: "V1"}
				})

				JustAfterEach(func() {
					err := testClients.SecretClient.Delete(secret.GetMetadata().GetNamespace(), secret.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())
					tu.Upstream.ProxyProtocolVersion = nil
				})

				It("should set the transport socket", func() {

					Eventually(envoyInstance.ConfigDump, "10s", "0.1s").Should(Not(MatchRegexp(tlsInspectorType)))

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
						SslConfig: &ssl.SslConfig{
							SslSecrets: &ssl.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      secret.GetMetadata().GetName(),
									Namespace: secret.GetMetadata().GetNamespace(),
								},
							},
							AlpnProtocols: []string{"http/1.1"},
						},
					}

					tcpGatewayRef := gatewaydefaults.DefaultTcpSslGateway(writeNamespace).GetMetadata().Ref()
					err := gloohelpers.PatchResource(
						ctx,
						tcpGatewayRef,
						func(resource resources.Resource) resources.Resource {
							gw := resource.(*gatewayv1.Gateway)
							gw.GetTcpGateway().TcpHosts = []*gloov1.TcpHost{host}
							return gw
						},
						testClients.GatewayClient.BaseClient(),
					)
					Expect(err).NotTo(HaveOccurred())

					// Check tls inspector is correctly configured
					Eventually(envoyInstance.ConfigDump, "10s", "0.1s").Should(MatchRegexp(tlsInspectorType))
					cd, _ := envoyInstance.ConfigDump()
					Expect(cd).To(ContainSubstring("envoy.extensions.transport_sockets.proxy_protocol.v3.ProxyProtocolUpstreamTransport"))
				})

			})

		})

		// These tests are meant to test the hybrid-specific functionality
		// The underlying Http and Tcp logic is tested independently
		Context("hybrid gateway", func() {

			var (
				envoyInstance *envoy.Instance
				testUpstream  *v1helpers.TestUpstream

				virtualService *gatewayv1.VirtualService
				hybridGateway  *gatewayv1.Gateway
			)

			BeforeEach(func() {
				envoyInstance = envoyFactory.NewInstance()

				testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
				virtualService = getTrivialVirtualServiceForUpstream(writeNamespace, testUpstream.Upstream.Metadata.Ref())
				hybridGateway = gatewaydefaults.DefaultHybridGateway(writeNamespace)

				err := envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
				Expect(err).NotTo(HaveOccurred())
			})

			JustBeforeEach(func() {
				var err error

				_, err = testClients.UpstreamClient.Write(testUpstream.Upstream, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred(), "Should be able to write upstream")

				virtualService, err = testClients.VirtualServiceClient.Write(virtualService, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred(), "Should be able to write virtual service")

				hybridGateway, err = testClients.GatewayClient.Write(hybridGateway, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred(), "Should be able to write hybrid gateway")
			})

			JustAfterEach(func() {
				var err error

				err = testClients.GatewayClient.Delete(hybridGateway.GetMetadata().GetNamespace(), hybridGateway.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred(), "Should be able to delete hybrid gateway")

				err = testClients.VirtualServiceClient.Delete(virtualService.GetMetadata().GetNamespace(), virtualService.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx, IgnoreNotExist: true})
				Expect(err).NotTo(HaveOccurred(), "Should be able to delete virtual service")

				err = testClients.UpstreamClient.Delete(testUpstream.Upstream.GetMetadata().GetNamespace(), testUpstream.Upstream.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred(), "Should be able to delete upstream")
			})

			AfterEach(func() {
				envoyInstance.Clean()
			})

			It("should create a hybrid listener with http and tcp matched listeners", func() {
				modifiedHybridGateway, err := testClients.GatewayClient.Read(writeNamespace, hybridGateway.GetMetadata().GetName(), clients.ReadOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				modifiedHybridGateway.GetHybridGateway().MatchedGateways = []*gatewayv1.MatchedGateway{
					{
						Matcher: &gatewayv1.Matcher{
							SourcePrefixRanges: []*v3.CidrRange{
								{
									AddressPrefix: "1.2.3.4",
									PrefixLen: &wrappers.UInt32Value{
										Value: 32,
									},
								},
							},
						},
						GatewayType: &gatewayv1.MatchedGateway_HttpGateway{
							HttpGateway: &gatewayv1.HttpGateway{
								VirtualServiceNamespaces: []string{writeNamespace},
							},
						},
					},
					{
						Matcher: &gatewayv1.Matcher{
							SourcePrefixRanges: []*v3.CidrRange{
								{
									AddressPrefix: "5.6.7.8",
									PrefixLen: &wrappers.UInt32Value{
										Value: 32,
									},
								},
							},
						},
						GatewayType: &gatewayv1.MatchedGateway_TcpGateway{
							TcpGateway: &gatewayv1.TcpGateway{},
						},
					},
				}
				Eventually(func() error {
					current, err := testClients.GatewayClient.Read(modifiedHybridGateway.Metadata.Namespace, modifiedHybridGateway.Metadata.Name, clients.ReadOpts{Ctx: ctx})
					if err != nil {
						return err
					}
					modifiedHybridGateway.Metadata.ResourceVersion = current.Metadata.ResourceVersion
					_, err = testClients.GatewayClient.Write(modifiedHybridGateway, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
					return err
				}, "5s", "0.3s").ShouldNot(HaveOccurred())

				// wait for hybrid listener to propagate to the proxy
				gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					proxy, err := testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return nil, err
					}

					// There should only be a single listener on the proxy and it should be a HybridListener
					hybridListener := proxy.GetListeners()[0].GetHybridListener()
					if hybridListener == nil {
						return nil, eris.New("HybridListener is not present on Proxy")
					}

					matchedListeners := hybridListener.GetMatchedListeners()
					if len(matchedListeners) != 2 {
						return nil, eris.New("HybridListener should have 2 matched listeners")
					}

					if len(matchedListeners[0].GetHttpListener().GetVirtualHosts()) != 1 {
						return nil, eris.New("HybridListener should have HttpListener with 1 Virtual host")
					}

					if matchedListeners[1].GetTcpListener() == nil {
						return nil, eris.New("HybridListener should have non-nil TcpListener")
					}

					// if all conditions are met, return the proxy
					return proxy, nil
				})
			})

			It("correctly configures gateway for a virtual service which contains a route to a service", func() {
				// Create a service so gloo can generate "fake" upstreams for it
				svc := kubernetes.NewService("default", "my-service")
				svc.Spec = corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 1234}}}
				svc, err := testClients.ServiceClient.Write(svc, clients.WriteOpts{Ctx: ctx})
				Expect(err).NotTo(HaveOccurred())

				// Update the existing virtual service with a route pointing to the above service
				virtualService.VirtualHost.Routes[0].GetRouteAction().GetSingle().DestinationType = &gloov1.Destination_Kube{
					Kube: &gloov1.KubernetesServiceDestination{
						Ref:  kubeutils.FromKubeMeta(svc.ObjectMeta, true).Ref(),
						Port: uint32(svc.Spec.Ports[0].Port),
					},
				}
				Eventually(func() error {
					current, err := testClients.VirtualServiceClient.Read(virtualService.Metadata.Namespace, virtualService.Metadata.Name, clients.ReadOpts{Ctx: ctx})
					if err != nil {
						return err
					}
					virtualService.Metadata.ResourceVersion = current.Metadata.ResourceVersion
					_, err = testClients.VirtualServiceClient.Write(virtualService, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
					return err
				}, "5s", "0.3s").ShouldNot(HaveOccurred())

				// Wait for proxy to be accepted
				var proxy *gloov1.Proxy
				gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					proxy, err = testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
					if err != nil {
						return nil, err
					}
					for _, l := range proxy.Listeners {
						if hl := l.GetHybridListener(); hl != nil {
							if len(hl.MatchedListeners) != 1 {
								continue
							}
							return proxy, nil
						}
					}

					// Verify that the proxy has the expected route
					Expect(proxy.Listeners).To(HaveLen(1))
					listener := proxy.Listeners[0]

					Expect(listener.GetHybridListener().GetMatchedListeners()[0].GetHttpListener()).NotTo(BeNil())
					httpListener := listener.GetHybridListener().GetMatchedListeners()[0].GetHttpListener()
					Expect(httpListener.VirtualHosts).To(HaveLen(1))
					Expect(httpListener.VirtualHosts[0].Routes).To(HaveLen(1))
					Expect(httpListener.VirtualHosts[0].Routes[0].GetRouteAction()).NotTo(BeNil())
					Expect(httpListener.VirtualHosts[0].Routes[0].GetRouteAction().GetSingle()).NotTo(BeNil())
					service := httpListener.VirtualHosts[0].Routes[0].GetRouteAction().GetSingle().GetKube()
					Expect(service.GetRef().GetNamespace()).To(Equal(svc.Namespace))
					Expect(service.GetRef().GetName()).To(Equal(svc.Name))
					Expect(service.Port).To(BeEquivalentTo(svc.Spec.Ports[0].Port))
					return nil, nil
				}, "5s", "0.1s")

			})

			Context("http traffic", func() {

				TestUpstreamReachable := func() {
					v1helpers.TestUpstreamReachable(envoyInstance.HybridPort, testUpstream, nil)
				}

				It("works when rapid virtual service creation and deletion causes no race conditions", func() {
					var err error

					TestUpstreamReachable()

					// Delete the Virtual Service
					err = testClients.VirtualServiceClient.Delete(writeNamespace, virtualService.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())

					Eventually(func() (gatewayv1.VirtualServiceList, error) {
						return testClients.VirtualServiceClient.List(writeNamespace, clients.ListOpts{Ctx: ctx})
					}, "10s", "0.5s").Should(HaveLen(0))
					Consistently(func() (gatewayv1.VirtualServiceList, error) {
						return testClients.VirtualServiceClient.List(writeNamespace, clients.ListOpts{Ctx: ctx})
					}, "10s", "0.5s").Should(HaveLen(0))
				})

				It("should work with no ssl and clean up the envoy config when the virtual service is deleted", func() {
					TestUpstreamReachable()

					// Delete the Virtual Service
					err := testClients.VirtualServiceClient.Delete(writeNamespace, virtualService.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
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
					request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d", envoyInstance.HybridPort), nil)
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
					// Create a regular request
					request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/", envoyInstance.HybridPort), nil)
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
					upstreamName := translator.UpstreamToClusterName(testUpstream.Upstream.Metadata.Ref())

					// Create route that uses cluster header destination
					virtualService.GetVirtualHost().Routes = []*gatewayv1.Route{{
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_ClusterHeader{
									ClusterHeader: "cluster-header-name",
								},
							},
						}}}

					Eventually(func() error {
						current, err := testClients.VirtualServiceClient.Read(virtualService.Metadata.Namespace, virtualService.Metadata.Name, clients.ReadOpts{Ctx: ctx})
						if err != nil {
							return err
						}
						virtualService.Metadata.ResourceVersion = current.Metadata.ResourceVersion
						_, err = testClients.VirtualServiceClient.Write(virtualService, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
						return err
					}, "5s", "0.3s").ShouldNot(HaveOccurred())

					// Create a regular request
					request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/", envoyInstance.HybridPort), nil)
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

					var secret *gloov1.Secret

					BeforeEach(func() {
						secret = &gloov1.Secret{
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
					})

					JustBeforeEach(func() {
						_, err := testClients.SecretClient.Write(secret, clients.WriteOpts{Ctx: ctx})
						Expect(err).NotTo(HaveOccurred())
					})

					JustAfterEach(func() {
						err := testClients.SecretClient.Delete(secret.GetMetadata().GetNamespace(), secret.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
						Expect(err).NotTo(HaveOccurred())
					})

					TestUpstreamSslReachable := func() {
						cert := gloohelpers.Certificate()
						v1helpers.TestUpstreamReachable(envoyInstance.HybridPort, testUpstream, &cert)
					}

					It("should work with ssl if ssl config is present in matcher", func() {
						// Check tls inspector has not been added yet
						Eventually(envoyInstance.ConfigDump, "10s", "0.1s").Should(Not(MatchRegexp(tlsInspectorType)))

						sslConfig := &ssl.SslConfig{
							SslSecrets: &ssl.SslConfig_SecretRef{
								SecretRef: secret.GetMetadata().Ref(),
							},
						}

						// Update gateway with ssl config
						gw, err := testClients.GatewayClient.List(writeNamespace, clients.ListOpts{Ctx: ctx})
						Expect(err).NotTo(HaveOccurred())

						for _, g := range gw {
							hybridGateway := g.GetHybridGateway()
							if hybridGateway != nil {
								hybridGateway.MatchedGateways[0].Matcher = &gatewayv1.Matcher{
									SslConfig: sslConfig,
								}
							}
							Eventually(func() error {
								current, err := testClients.GatewayClient.Read(g.Metadata.Namespace, g.Metadata.Name, clients.ReadOpts{Ctx: ctx})
								if err != nil {
									return err
								}
								g.Metadata.ResourceVersion = current.Metadata.ResourceVersion
								_, err = testClients.GatewayClient.Write(g, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
								return err
							}, "5s", "0.3s").ShouldNot(HaveOccurred())
						}

						virtualService.SslConfig = sslConfig
						Eventually(func() error {
							current, err := testClients.VirtualServiceClient.Read(virtualService.Metadata.Namespace, virtualService.Metadata.Name, clients.ReadOpts{Ctx: ctx})
							if err != nil {
								return err
							}
							virtualService.Metadata.ResourceVersion = current.Metadata.ResourceVersion
							_, err = testClients.VirtualServiceClient.Write(virtualService, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
							return err
						}, "5s", "0.3s").ShouldNot(HaveOccurred())

						TestUpstreamSslReachable()

						Eventually(envoyInstance.ConfigDump, "10s", "0.1s").Should(MatchRegexp(tlsInspectorType))
					})
				})
			})

			Context("tcp ssl", func() {

				var secret *gloov1.Secret

				BeforeEach(func() {
					secret = &gloov1.Secret{
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
				})

				JustBeforeEach(func() {
					_, err := testClients.SecretClient.Write(secret, clients.WriteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())
				})

				JustAfterEach(func() {
					err := testClients.SecretClient.Delete(secret.GetMetadata().GetNamespace(), secret.GetMetadata().GetName(), clients.DeleteOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())
				})

				TestUpstreamSslReachableTcp := func() {
					cert := gloohelpers.Certificate()
					v1helpers.TestUpstreamReachable(envoyInstance.HybridPort, testUpstream, &cert)
				}

				It("should work with ssl", func() {
					// Check tls inspector has not been added yet
					Eventually(envoyInstance.ConfigDump, "10s", "0.1s").Should(Not(MatchRegexp(tlsInspectorType)))

					host := &gloov1.TcpHost{
						Name: "tcp-host-one",
						Destination: &gloov1.TcpHost_TcpAction{
							Destination: &gloov1.TcpHost_TcpAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: testUpstream.Upstream.Metadata.Ref(),
									},
								},
							},
						},
						SslConfig: &ssl.SslConfig{
							SslSecrets: &ssl.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      secret.GetMetadata().GetName(),
									Namespace: secret.GetMetadata().GetNamespace(),
								},
							},
							AlpnProtocols: []string{"http/1.1"},
						},
					}

					// Update gateway with tcp hosts
					gw, err := testClients.GatewayClient.List(writeNamespace, clients.ListOpts{Ctx: ctx})
					Expect(err).NotTo(HaveOccurred())

					for _, g := range gw {
						hybridGateway := g.GetHybridGateway()
						if hybridGateway != nil {
							hybridGateway.MatchedGateways = []*gatewayv1.MatchedGateway{
								// Even though this test does not operate on HttpGateways, we intentionally include
								// the configuration to ensure that it does not affect TcpGateway translation
								{
									Matcher: &gatewayv1.Matcher{
										SourcePrefixRanges: []*v3.CidrRange{
											{
												AddressPrefix: "1.2.3.4",
												PrefixLen: &wrappers.UInt32Value{
													Value: 32,
												},
											},
										},
									},
									GatewayType: &gatewayv1.MatchedGateway_HttpGateway{
										HttpGateway: &gatewayv1.HttpGateway{},
									},
								},
								{
									Matcher: &gatewayv1.Matcher{},
									GatewayType: &gatewayv1.MatchedGateway_TcpGateway{
										TcpGateway: &gatewayv1.TcpGateway{
											TcpHosts: []*gloov1.TcpHost{host},
										},
									},
								},
							}
						}
						Eventually(func() error {
							current, err := testClients.GatewayClient.Read(g.Metadata.Namespace, g.Metadata.Name, clients.ReadOpts{Ctx: ctx})
							if err != nil {
								return err
							}
							g.Metadata.ResourceVersion = current.Metadata.ResourceVersion
							_, err = testClients.GatewayClient.Write(g, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
							return err
						}, "5s", "0.3s").ShouldNot(HaveOccurred())
					}

					// Check tls inspector is correctly configured
					Eventually(envoyInstance.ConfigDump, "10s", "0.1s").Should(MatchRegexp(tlsInspectorType))

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
