package e2e_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/healthcheck"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
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
			},
			{
				Title: "Rest Eds Disabled",
				RestEdsEnabled: &wrappers.BoolValue{
					Value: false,
				},
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

		Describe(fmt.Sprintf("%s", testCase.Title), func() {

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
					//noinspection GoUnhandledErrorResult
					defer response.Body.Close()

					body, err := ioutil.ReadAll(response.Body)
					Expect(err).NotTo(HaveOccurred())
					statsString := string(body)

					// Verify that stats for the above virtual cluster are present
					Expect(statsString).To(ContainSubstring("vhost.virt1.vcluster.test-vc."))
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
									PrivateKey: gloohelpers.PrivateKey(),
									CertChain:  gloohelpers.Certificate(),
									RootCa:     gloohelpers.Certificate(),
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
						copyUp.SslConfig = &gloov1.UpstreamSslConfig{
							SslSecrets: &gloov1.UpstreamSslConfig_SecretRef{
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
						gloohelpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
							return testClients.ProxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
						})
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
