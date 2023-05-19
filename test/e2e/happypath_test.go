package e2e_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/healthcheck"
	routerV1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/router"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	testhelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skkubeutils "github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"github.com/solo-io/solo-kit/test/helpers"
	"github.com/solo-io/solo-kit/test/setup"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var _ = Describe("Happy path", func() {

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		tu            *v1helpers.TestUpstream
		envoyPort     uint32

		testCases = []struct {
			Title               string
			RestEdsEnabled      *wrappers.BoolValue
			TransportApiVersion envoy_config_core_v3.ApiVersion
		}{
			{
				Title: "Rest Eds Enabled",
				RestEdsEnabled: &wrappers.BoolValue{
					Value: true,
				},
				TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
			},
			{
				Title: "Rest Eds Disabled",
				RestEdsEnabled: &wrappers.BoolValue{
					Value: false,
				},
				TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
			},
		}
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		tu = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		envoyPort = defaults.HttpPort
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	TestUpstreamReachable := func() {
		v1helpers.TestUpstreamReachableWithOffset(3, envoyPort, tu, nil)
	}

	for _, testCase := range testCases {

		Describe(fmt.Sprintf("%s: (%s)", testCase.Title, testCase.TransportApiVersion.String()), func() {

			Describe("in memory", func() {

				var up *gloov1.Upstream

				BeforeEach(func() {
					ns := defaults.GlooSystem
					ro := &services.RunOptions{
						NsToWrite: ns,
						NsToWatch: []string{"default", ns},
						WhatToRun: services.What{
							DisableGateway: true,
							DisableUds:     true,
							DisableFds:     true,
						},
						Settings: &gloov1.Settings{
							Gloo: &gloov1.GlooOptions{
								EnableRestEds: testCase.RestEdsEnabled,
							},
						},
					}
					testClients = services.RunGlooGatewayUdsFds(ctx, ro)
					envoyInstance.ApiVersion = testCase.TransportApiVersion.String()
					err := envoyInstance.RunWithRoleAndRestXds(ns+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
					Expect(err).NotTo(HaveOccurred())

					up = tu.Upstream
					_, err = testClients.UpstreamClient.Write(up, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())
				})

				It("should not crash", func() {
					proxy := getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, up.Metadata.Ref())
					_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					TestUpstreamReachable()
				})

				It("should not crash multiple methods", func() {
					proxy := getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, up.Metadata.Ref())
					proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener.
						VirtualHosts[0].Routes[0].Matchers = []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/"},
							Methods:       []string{"GET", "POST"},
						},
					}

					_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					TestUpstreamReachable()
				})

				It("correctly configures envoy to emit virtual cluster statistics", func() {
					proxy := getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, up.Metadata.Ref())

					// Set a virtual cluster matching everything
					proxy.Listeners[0].GetHttpListener().VirtualHosts[0].Options = &gloov1.VirtualHostOptions{
						Stats: &stats.Stats{
							VirtualClusters: []*stats.VirtualCluster{{
								Name:    "test-vc",
								Pattern: ".*",
							}},
						},
					}

					_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					// This will hit the virtual host with the above virtual cluster config
					TestUpstreamReachable()

					response, err := http.Get(fmt.Sprintf("http://localhost:%d/stats", envoyInstance.AdminPort))
					Expect(err).NotTo(HaveOccurred())
					Expect(response).NotTo(BeNil())
					//goland:noinspection GoUnhandledErrorResult
					defer response.Body.Close()

					body, err := io.ReadAll(response.Body)
					Expect(err).NotTo(HaveOccurred())
					statsString := string(body)

					// Verify that stats for the above virtual cluster are present
					Expect(statsString).To(ContainSubstring("vhost.virt1.vcluster.test-vc."))
				})

				It("it correctly passes the suppress envoy headers config", func() {
					proxy := getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, up.Metadata.Ref())

					// configuring an http listener option to set suppressEnvoyHeaders to true
					//projects/gloo/api/v1/options/router/router.proto
					proxy.Listeners[0].GetHttpListener().Options = &gloov1.HttpListenerOptions{
						Router: &routerV1.Router{
							SuppressEnvoyHeaders: wrapperspb.Bool(true),
						},
					}

					_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{
						Ctx: ctx,
					})
					Expect(err).NotTo(HaveOccurred())

					TestUpstreamReachable()

					// This will hit the virtual host with the above virtual cluster config
					response, err := http.Get(fmt.Sprintf("http://%s:%d/", "localhost", defaults.HttpPort))
					Expect(err).NotTo(HaveOccurred())
					Expect(response.Header).NotTo(HaveKey("X-Envoy-Upstream-Service-Time"))

					cfg, err := envoyInstance.ConfigDump()
					Expect(err).NotTo(HaveOccurred())

					// We expect the envoy configuration to contain these properties in the configuration dump
					Expect(cfg).To(MatchRegexp("\"suppress_envoy_headers\": true"))

				})

				It("it correctly DID NOT pass the suppress envoy headers config", func() {
					proxy := getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, up.Metadata.Ref())

					// Set a virtual cluster listener that is blank and has no options
					proxy.Listeners[0].GetHttpListener().Options = &gloov1.HttpListenerOptions{}

					_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{
						Ctx: ctx,
					})
					Expect(err).NotTo(HaveOccurred())
					TestUpstreamReachable()

					// This will hit the virtual host with the above virtual cluster config
					response, err := http.Get(fmt.Sprintf("http://%s:%d/", "localhost", defaults.HttpPort))
					Expect(err).NotTo(HaveOccurred())
					Expect(response.Header).To(HaveKey("X-Envoy-Upstream-Service-Time"))

					cfg, err := envoyInstance.ConfigDump()
					Expect(err).NotTo(HaveOccurred())

					// We expect the envoy configuration to NOT contain these properties in the configuration dump
					Expect(cfg).To(Not(MatchRegexp("\"suppress_envoy_headers\": true")))
				})

				It("passes a health check", func() {
					proxy := getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, up.Metadata.Ref())

					// Set a virtual cluster matching everything
					proxy.Listeners[0].GetHttpListener().Options = &gloov1.HttpListenerOptions{
						HealthCheck: &healthcheck.HealthCheck{
							Path: "/healthy",
						},
					}

					proxy.Listeners[0].GetHttpListener().VirtualHosts[0].Routes[0].Action = &gloov1.Route_DirectResponseAction{
						DirectResponseAction: &gloov1.DirectResponseAction{
							Status: 400,
							Body:   "only health checks work on me. sorry!",
						},
					}
					_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
					Expect(err).NotTo(HaveOccurred())

					Eventually(func() error {
						res, err := http.Get(fmt.Sprintf("http://%s:%d/healthy", "localhost", envoyPort))
						if err != nil {
							return err
						}
						if res.StatusCode != 200 {
							return errors.Errorf("bad status code: %v", res.StatusCode)
						}
						return nil
					}, time.Second*10, time.Second/2).ShouldNot(HaveOccurred())

					res, err := http.Post(fmt.Sprintf("http://localhost:%v/healthcheck/fail", envoyInstance.AdminPort), "", nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(res.StatusCode).To(Equal(200))

					Eventually(func() error {
						res, err := http.Get(fmt.Sprintf("http://%s:%d/healthy", "localhost", envoyPort))
						if err != nil {
							return err
						}
						if res.StatusCode != 503 {
							return errors.Errorf("bad status code: %v", res.StatusCode)
						}
						return nil
					}, time.Second*10, time.Second/2).ShouldNot(HaveOccurred())
				})

				Context("ssl", func() {
					type sslConn struct {
						sni  string
						port uint32
					}
					var (
						upSsl    *gloov1.Upstream
						hellos   chan sslConn
						sslport1 uint32
						sslport2 uint32
					)
					BeforeEach(func() {
						hellos = make(chan sslConn, 100)
						sslSecret := &gloov1.Secret{
							Metadata: &core.Metadata{
								Name:      "secret",
								Namespace: "default",
							},
							Kind: &gloov1.Secret_Tls{
								Tls: &gloov1.TlsSecret{
									PrivateKey: testhelpers.PrivateKey(),
									CertChain:  testhelpers.Certificate(),
									RootCa:     testhelpers.Certificate(),
								},
							},
						}
						_, err := testClients.SecretClient.Write(sslSecret, clients.WriteOpts{})
						Expect(err).NotTo(HaveOccurred())
						// create ssl proxy
						copyUp := *tu.Upstream
						copyUp.Metadata.Name = copyUp.Metadata.Name + "-ssl"
						port := tu.Upstream.UpstreamType.(*gloov1.Upstream_Static).Static.Hosts[0].Port
						addr := tu.Upstream.UpstreamType.(*gloov1.Upstream_Static).Static.Hosts[0].Addr
						sslport1 = v1helpers.StartSslProxyWithHelloCB(ctx, port, func(chi *tls.ClientHelloInfo) {
							hellos <- sslConn{sni: chi.ServerName, port: sslport1}
						})
						sslport2 = v1helpers.StartSslProxyWithHelloCB(ctx, port, func(chi *tls.ClientHelloInfo) {
							hellos <- sslConn{sni: chi.ServerName, port: sslport2}
						})
						ref := sslSecret.Metadata.Ref()

						copyUp.UpstreamType = &gloov1.Upstream_Static{
							Static: &static_plugin_gloo.UpstreamSpec{
								Hosts: []*static_plugin_gloo.Host{{
									Addr: addr,
									Port: sslport1,
								}, {
									Addr: addr,
									Port: sslport2,
								}},
							},
						}
						copyUp.SslConfig = &ssl.UpstreamSslConfig{
							SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
								SecretRef: ref,
							},
						}
						upSsl = &copyUp
					})

					Context("simple ssl", func() {
						BeforeEach(func() {
							_, err := testClients.UpstreamClient.Write(upSsl, clients.WriteOpts{})
							Expect(err).NotTo(HaveOccurred())
						})
						It("should work with ssl", func() {
							proxycli := testClients.ProxyClient
							proxy := getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, upSsl.Metadata.Ref())
							_, err := proxycli.Write(proxy, clients.WriteOpts{})
							Expect(err).NotTo(HaveOccurred())

							TestUpstreamReachable()
						})
					})

					Context("sni", func() {

						BeforeEach(func() {
							upSsl.GetStatic().GetHosts()[0].SniAddr = "solo-sni-test"
							upSsl.GetStatic().GetHosts()[1].SniAddr = "solo-sni-test2"

							_, err := testClients.UpstreamClient.Write(upSsl, clients.WriteOpts{})
							Expect(err).NotTo(HaveOccurred())
						})

						It("should work with ssl", func() {
							proxycli := testClients.ProxyClient
							proxy := getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, upSsl.Metadata.Ref())
							_, err := proxycli.Write(proxy, clients.WriteOpts{})
							Expect(err).NotTo(HaveOccurred())

							match1 := sslConn{sni: "solo-sni-test", port: sslport1}
							match2 := sslConn{sni: "solo-sni-test2", port: sslport2}

							matched1 := false
							matched2 := false

							timeout := time.After(5 * time.Second)
							for {
								TestUpstreamReachable()
								select {
								case <-timeout:
									Fail("timedout waiting for sni")
								case clienthello := <-hellos:
									Expect(clienthello).To(SatisfyAny(Equal(match1), Equal(match2)))
									if clienthello == match1 {
										matched1 = true
									} else {
										matched2 = true
									}
								}
								if matched1 && matched2 {
									break
								}
							}

						})
					})
				})

				Context("sad path", func() {
					It("should error the proxy with two listeners with the same bind address", func() {
						// create a proxy with two identical listeners to see errors come up
						proxy := getTrivialProxyForUpstream(defaults.GlooSystem, envoyPort, up.Metadata.Ref())
						proxy.Listeners = append(proxy.Listeners, proxy.Listeners[0])

						// persist the proxy
						_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
						Expect(err).NotTo(HaveOccurred())

						// eventually the proxy is rejected
						testhelpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
							return testClients.ProxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
						})
					})
				})
			})

			Describe("kubernetes happy path", func() {
				BeforeEach(func() {
					testutils.ValidateRequirementsAndNotifyGinkgo(
						testutils.Kubernetes("Uses a Kubernetes cluster"),
					)
				})

				var (
					namespace      string
					writeNamespace string
					cfg            *rest.Config
					kubeClient     kubernetes.Interface
					svc            *kubev1.Service
				)

				BeforeEach(func() {
					namespace = ""
					writeNamespace = ""
					var err error
					svc = nil
					cfg, err = kubeutils.GetConfig("", "")
					Expect(err).NotTo(HaveOccurred())
					kubeClient, err = kubernetes.NewForConfig(cfg)
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					if namespace != "" {
						_ = setup.TeardownKube(namespace)
					}
				})

				prepNamespace := func() {
					if namespace == "" {
						namespace = "gloo-e2e-" + helpers.RandString(8)
					}

					_, err := kubeClient.CoreV1().Namespaces().Create(ctx, &kubev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: namespace,
						},
					}, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())

					svc, err = kubeClient.CoreV1().Services(namespace).Create(ctx, &kubev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      "headlessservice",
						},
						Spec: kubev1.ServiceSpec{
							Ports: []kubev1.ServicePort{
								{
									Name: "foo",
									Port: int32(tu.Port),
								},
							},
						},
					}, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())

					_, err = kubeClient.CoreV1().Endpoints(namespace).Create(ctx, &kubev1.Endpoints{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      svc.Name,
						},
						Subsets: []kubev1.EndpointSubset{{
							Addresses: []kubev1.EndpointAddress{{
								IP:       getNonSpecialIP(envoyInstance),
								Hostname: "localhost",
							}},
							Ports: []kubev1.EndpointPort{{
								Port: int32(tu.Port),
							}},
						}},
					}, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())
				}

				getUpstream := func() (*gloov1.Upstream, error) {
					l, err := testClients.UpstreamClient.List(writeNamespace, clients.ListOpts{})
					if err != nil {
						return nil, err
					}
					for _, u := range l {
						if strings.Contains(u.Metadata.Name, svc.Name) && strings.Contains(u.Metadata.Name, svc.Namespace) {
							return u, nil
						}
					}
					return nil, fmt.Errorf("not found")
				}

				Context("specific namespace", func() {

					BeforeEach(func() {
						prepNamespace()
						writeNamespace = namespace
						ro := &services.RunOptions{
							NsToWrite: writeNamespace,
							NsToWatch: []string{namespace},
							WhatToRun: services.What{
								DisableGateway: true,
							},
							KubeClient: kubeClient,
							Settings: &gloov1.Settings{
								Gloo: &gloov1.GlooOptions{
									EnableRestEds: testCase.RestEdsEnabled,
								},
							},
						}

						testClients = services.RunGlooGatewayUdsFds(ctx, ro)
						role := namespace + "~" + gatewaydefaults.GatewayProxyName
						err := envoyInstance.RunWithRole(role, testClients.GlooPort)
						Expect(err).NotTo(HaveOccurred())

						testhelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
							return getUpstream()
						}, "20s", ".5s")
					})

					It("should discover service", func() {
						up, err := getUpstream()
						Expect(err).NotTo(HaveOccurred())

						proxycli := testClients.ProxyClient
						proxy := getTrivialProxyForUpstream(namespace, envoyPort, up.Metadata.Ref())
						var opts clients.WriteOpts
						_, err = proxycli.Write(proxy, opts)
						Expect(err).NotTo(HaveOccurred())

						TestUpstreamReachable()
					})

					It("should create appropriate config for discovered service", func() {
						up, err := getUpstream()
						Expect(err).NotTo(HaveOccurred())

						svc.Annotations = map[string]string{
							"gloo.solo.io/upstream_config": "{\"initial_stream_window_size\": 2048}",
						}
						svc, err = kubeClient.CoreV1().Services(namespace).Update(ctx, svc, metav1.UpdateOptions{})
						Expect(err).NotTo(HaveOccurred())

						proxycli := testClients.ProxyClient
						proxy := getTrivialProxyForUpstream(namespace, envoyPort, up.Metadata.Ref())
						var opts clients.WriteOpts
						_, err = proxycli.Write(proxy, opts)
						Expect(err).NotTo(HaveOccurred())

						TestUpstreamReachable()
						upstream, err := getUpstream()
						Expect(err).NotTo(HaveOccurred())
						Expect(int(upstream.GetInitialStreamWindowSize().GetValue())).To(Equal(2048))
					})

					It("correctly routes requests to a service destination", func() {
						svcRef := skkubeutils.FromKubeMeta(svc.ObjectMeta, true).Ref()
						svcPort := svc.Spec.Ports[0].Port
						proxy := getTrivialProxyForService(namespace, envoyPort, svcRef, uint32(svcPort))

						_, err := testClients.ProxyClient.Write(proxy, clients.WriteOpts{})
						Expect(err).NotTo(HaveOccurred())

						TestUpstreamReachable()
					})
				})

				Context("all namespaces", func() {
					BeforeEach(func() {
						namespace = "gloo-e2e-" + helpers.RandString(8)
						prepNamespace()

						writeNamespace = namespace
						ro := &services.RunOptions{
							NsToWrite: writeNamespace,
							NsToWatch: []string{},
							WhatToRun: services.What{
								DisableGateway: true,
							},
							KubeClient: kubeClient,
							Settings: &gloov1.Settings{
								Gloo: &gloov1.GlooOptions{
									EnableRestEds: testCase.RestEdsEnabled,
								},
							},
						}

						testClients = services.RunGlooGatewayUdsFds(ctx, ro)
						role := namespace + "~" + gatewaydefaults.GatewayProxyName
						err := envoyInstance.RunWithRole(role, testClients.GlooPort)
						Expect(err).NotTo(HaveOccurred())

					})

					It("watch all namespaces", func() {
						testhelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
							return getUpstream()
						})

						up, err := getUpstream()
						Expect(err).NotTo(HaveOccurred())

						proxycli := testClients.ProxyClient
						proxy := getTrivialProxyForUpstream(namespace, envoyPort, up.Metadata.Ref())
						var opts clients.WriteOpts
						_, err = proxycli.Write(proxy, opts)
						Expect(err).NotTo(HaveOccurred())

						TestUpstreamReachable()
					})
				})
			})
		})

	}

})

func getTrivialProxyForUpstream(ns string, bindPort uint32, upstream *core.ResourceRef) *gloov1.Proxy {
	proxy := getTrivialProxy(ns, bindPort)
	proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener.
		VirtualHosts[0].Routes[0].Action.(*gloov1.Route_RouteAction).RouteAction.
		Destination.(*gloov1.RouteAction_Single).Single.DestinationType =
		&gloov1.Destination_Upstream{Upstream: upstream}
	return proxy
}

func getTrivialProxyForService(ns string, bindPort uint32, service *core.ResourceRef, svcPort uint32) *gloov1.Proxy {
	proxy := getTrivialProxy(ns, bindPort)
	proxy.Listeners[0].ListenerType.(*gloov1.Listener_HttpListener).HttpListener.
		VirtualHosts[0].Routes[0].Action.(*gloov1.Route_RouteAction).RouteAction.
		Destination.(*gloov1.RouteAction_Single).Single.DestinationType =
		&gloov1.Destination_Kube{
			Kube: &gloov1.KubernetesServiceDestination{
				Ref:  service,
				Port: svcPort,
			},
		}
	return proxy
}

func getTrivialProxy(ns string, bindPort uint32) *gloov1.Proxy {
	return &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:      gatewaydefaults.GatewayProxyName,
			Namespace: ns,
		},
		Listeners: []*gloov1.Listener{{
			Name:        "listener",
			BindAddress: "::",
			BindPort:    bindPort,
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					VirtualHosts: []*gloov1.VirtualHost{{
						Name:    "virt1",
						Domains: []string{"*"},
						Routes: []*gloov1.Route{{
							Action: &gloov1.Route_RouteAction{
								RouteAction: &gloov1.RouteAction{
									Destination: &gloov1.RouteAction_Single{
										Single: &gloov1.Destination{},
									},
								},
							},
						}},
					}},
				},
			},
		}},
	}
}

// getNonSpecialIP returns a non-special IP that Kubernetes will allow in an endpoint.
func getNonSpecialIP(instance *services.EnvoyInstance) string {
	if instance.UseDocker {
		return instance.LocalAddr()
	}

	ifaces, err := net.Interfaces()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}
			if isNonSpecialIP(ip) {
				return ip.String()
			}
		}
	}
	Fail("no ip address available", 1)
	return ""
}

// isNonSpecialIP is adapted from ValidateNonSpecialIP in k8s.io/kubernetes/pkg/apis/core/validation/validation.go
//
// Specifically disallowed are unspecified, loopback addresses, and link-local addresses
// which tend to be used for node-centric purposes (e.g. metadata service).
func isNonSpecialIP(ip net.IP) bool {
	if ip == nil {
		return false // must be a valid IP address
	}
	if ip.IsUnspecified() {
		return false // may not be unspecified
	}
	if ip.IsLoopback() {
		return false // may not be in the loopback range (127.0.0.0/8, ::1/128)
	}
	if ip.IsLinkLocalUnicast() {
		return false // may not be in the link-local range (169.254.0.0/16, fe80::/10)
	}
	if ip.IsLinkLocalMulticast() {
		return false // may not be in the link-local multicast range (224.0.0.0/24, ff02::/10)
	}
	return true
}
