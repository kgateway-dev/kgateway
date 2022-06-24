package e2e_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Aggregate Listener", func() {

	// An AggregateListener is a type of Listener supported on a Proxy
	// Proxies only contain this type of Listener by configuring the
	// IsolateVirtualHostsBySslConfig property in the Settings CR to true
	// These tests generally perform the following with and without this setting:
	//	1. Produce Gateways and VirtualServices
	//	2. Convert those resources into a Proxy
	//	3. Configure an instance of Envoy with that Proxy configuration
	//	4. Confirm the routing behavior

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		envoyInstance *services.EnvoyInstance
		testClients   services.TestClients
		testUpstream  *v1helpers.TestUpstream

		isolateVirtualHostsBySslConfig bool

		resourcesToCreate *gloosnapshot.ApiSnapshot
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()
		defaults.TcpPort = services.NextBindPort()
		defaults.HybridPort = services.NextBindPort()

		// Initialize Envoy instance
		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		// The set of resources that these tests will generate
		resourcesToCreate = &gloosnapshot.ApiSnapshot{
			Gateways:        v1.GatewayList{},
			VirtualServices: v1.VirtualServiceList{},
			Upstreams: gloov1.UpstreamList{
				testUpstream.Upstream,
			},
			Secrets: gloov1.SecretList{},
		}
	})

	AfterEach(func() {
		cancel()
	})

	JustBeforeEach(func() {
		// Run Gloo
		testClients = services.RunGlooGatewayUdsFds(ctx, &services.RunOptions{
			NsToWrite: defaults.GlooSystem,
			NsToWatch: []string{"default", defaults.GlooSystem},
			WhatToRun: services.What{
				DisableGateway: false,
				DisableFds:     true,
				DisableUds:     true,
			},
			Settings: &gloov1.Settings{
				Gateway: &gloov1.GatewayOptions{
					IsolateVirtualHostsBySslConfig: &wrappers.BoolValue{
						Value: isolateVirtualHostsBySslConfig,
					},
				},
			},
		})

		// Run envoy
		role := fmt.Sprintf("%s~%s", defaults.GlooSystem, gatewaydefaults.GatewayProxyName)
		err := envoyInstance.RunWithRole(role, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		// Create Resources
		err = testClients.WriteSnapshot(ctx, resourcesToCreate)
		Expect(err).NotTo(HaveOccurred())

		// Wait for a proxy to be accepted
		gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
		})
	})

	JustAfterEach(func() {
		// Cleanup Resources
		err := testClients.DeleteSnapshot(ctx, resourcesToCreate)
		Expect(err).NotTo(HaveOccurred())

		// Cleanup the Proxy
		deleteErr := testClients.ProxyClient.Delete(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.DeleteOpts{Ctx: ctx, IgnoreNotExist: true})
		Expect(deleteErr).NotTo(HaveOccurred())

		// Stop Envoy
		envoyInstance.Clean()
	})

	Context("Insecure HttpGateway", func() {

		TestUpstreamReachable := func(host, path string) {
			v1helpers.ExpectCurlWithOffset(
				1,
				v1helpers.CurlRequest{
					RootCA: nil,
					Port:   defaults.HttpPort,
					Host:   host,
					Path:   path,
					Body:   []byte("solo.io test"),
				},
				v1helpers.CurlResponse{
					Status:  http.StatusOK,
					Message: "",
				})
		}

		BeforeEach(func() {
			simpleRouteName := "simple-route"
			vsBuilder := gloohelpers.NewVirtualServiceBuilder().WithNamespace(defaults.GlooSystem)

			vsEast := vsBuilder.
				WithName("vs-east").
				WithDomain("east.com").
				WithRouteActionToUpstream(simpleRouteName, testUpstream.Upstream).
				WithPrefixMatcher(simpleRouteName, "/east").
				Build()

			vsWest := vsBuilder.
				WithName("vs-west").
				WithDomain("west.com").
				WithRouteActionToUpstream(simpleRouteName, testUpstream.Upstream).
				WithPrefixMatcher(simpleRouteName, "/west").
				Build()

			resourcesToCreate.Gateways = v1.GatewayList{
				gatewaydefaults.DefaultGateway(defaults.GlooSystem),
			}
			resourcesToCreate.VirtualServices = v1.VirtualServiceList{
				vsEast, vsWest,
			}
		})

		Context("IsolateVirtualHostsBySslConfig = false", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = false
			})

			It("produces a Proxy with a single HttpListener", func() {
				proxy, err := testClients.ProxyClient.Read(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				Expect(proxy.GetListeners()).To(HaveLen(1))
				Expect(proxy.GetListeners()[0].GetHttpListener()).NotTo(BeNil())
			})

			It("routes requests to all routes on gateway", func() {
				TestUpstreamReachable("east.com", "/east/1")
				TestUpstreamReachable("west.com", "/west/1")
			})

		})

		Context("IsolateVirtualHostsBySslConfig = true", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = true
			})

			It("produces a Proxy with a single AggregateListener", func() {
				proxy, err := testClients.ProxyClient.Read(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				Expect(proxy.GetListeners()).To(HaveLen(1))
				Expect(proxy.GetListeners()[0].GetAggregateListener()).NotTo(BeNil())
			})

			It("routes requests to all routes on gateway", func() {
				TestUpstreamReachable("east.com", "/east/1")
				TestUpstreamReachable("west.com", "/west/1")
			})

		})

	})

	Context("Secure HttpGateway", func() {

		var (
			eastCert, eastPK = gloohelpers.Certificate(), gloohelpers.PrivateKey()
			westCert, westPK = gloohelpers.GetCerts(gloohelpers.Params{
				Hosts: "other-host",
				IsCA:  false,
			})
		)

		TestUpstreamReturns := func(host, path, cert string, responseStatus int) {
			v1helpers.ExpectCurlWithOffset(
				1,
				v1helpers.CurlRequest{
					RootCA: &cert,
					Port:   defaults.HttpsPort,
					Host:   host,
					Path:   path,
					Body:   []byte("solo.io test"),
				},
				v1helpers.CurlResponse{
					Status:  responseStatus,
					Message: "",
				})
		}

		BeforeEach(func() {
			simpleRouteName := "simple-route"
			vsBuilder := gloohelpers.NewVirtualServiceBuilder().WithNamespace(defaults.GlooSystem)

			eastTLSSecret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "east-tls-secret",
					Namespace: defaults.GlooSystem,
				},
				Kind: &gloov1.Secret_Tls{
					Tls: &gloov1.TlsSecret{
						CertChain:  eastCert,
						PrivateKey: eastPK,
					},
				},
			}
			westTLSSecret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "west-tls-secret",
					Namespace: defaults.GlooSystem,
				},
				Kind: &gloov1.Secret_Tls{
					Tls: &gloov1.TlsSecret{
						CertChain:  westCert,
						PrivateKey: westPK,
					},
				},
			}

			vsEast := vsBuilder.
				WithName("vs-east").
				WithDomain("east.com").
				WithRouteActionToUpstream(simpleRouteName, testUpstream.Upstream).
				WithPrefixMatcher(simpleRouteName, "/east").
				WithSslConfig(&gloov1.SslConfig{
					SslSecrets: &gloov1.SslConfig_SecretRef{
						SecretRef: eastTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()

			vsWest := vsBuilder.
				WithName("vs-west").
				WithDomain("west.com").
				WithRouteActionToUpstream(simpleRouteName, testUpstream.Upstream).
				WithPrefixMatcher(simpleRouteName, "/west").
				WithSslConfig(&gloov1.SslConfig{
					OneWayTls: &wrappers.BoolValue{
						Value: false,
					},
					SniDomains: []string{"west.com"},
					SslSecrets: &gloov1.SslConfig_SecretRef{
						SecretRef: westTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()

			resourcesToCreate.Gateways = v1.GatewayList{
				gatewaydefaults.DefaultSslGateway(defaults.GlooSystem),
			}
			resourcesToCreate.VirtualServices = v1.VirtualServiceList{
				vsEast, vsWest,
			}
			resourcesToCreate.Secrets = gloov1.SecretList{
				eastTLSSecret, westTLSSecret,
			}
		})

		Context("IsolateVirtualHostsBySslConfig = false", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = false
			})

			It("produces a Proxy with a single HttpListener", func() {
				proxy, err := testClients.ProxyClient.Read(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				Expect(proxy.GetListeners()).To(HaveLen(1))
				Expect(proxy.GetListeners()[0].GetHttpListener()).NotTo(BeNil())
			})

			It("routes requests to all routes on gateway", func() {
				// This test demonstrates the flaw with HttpListeners:
				//	The West VirtualService should only be exposing routes if the westCert is provided,
				//	but in this test we can successfully execute requests against the west routes,
				//	by providing an east certificate.
				//
				// This is due to the fact that an HttpListener creates an aggregate set of RouteConfiguration
				// and then produces duplicate FilterChains, based on all available SslConfig's from VirtualServices
				TestUpstreamReturns("east.com", "/east/1", eastCert, http.StatusOK)
				TestUpstreamReturns("west.com", "/west/1", eastCert, http.StatusOK)
			})

		})

		Context("IsolateVirtualHostsBySslConfig = true", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = true
			})

			It("produces a Proxy with a single AggregateListener", func() {
				proxy, err := testClients.ProxyClient.Read(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				Expect(proxy.GetListeners()).To(HaveLen(1))
				Expect(proxy.GetListeners()[0].GetAggregateListener()).NotTo(BeNil())
			})

			It("routes requests to all routes on gateway", func() {
				// This test demonstrates the solution with AggregateListeners:
				//	The West VirtualService is no longer routable with the eastCert.
				TestUpstreamReturns("east.com", "/east/1", eastCert, http.StatusOK)
				TestUpstreamReturns("west.com", "/west/1", eastCert, http.StatusNotFound)
			})

		})

	})

	Context("Insecure HybridGateway", func() {

		TestUpstreamReachable := func(host, path string) {
			v1helpers.ExpectCurlWithOffset(
				1,
				v1helpers.CurlRequest{
					RootCA: nil,
					Port:   defaults.HybridPort,
					Host:   host,
					Path:   path,
					Body:   []byte("solo.io test"),
				},
				v1helpers.CurlResponse{
					Status:  http.StatusOK,
					Message: "",
				})
		}

		BeforeEach(func() {
			simpleRouteName := "simple-route"
			vsBuilder := gloohelpers.NewVirtualServiceBuilder().WithNamespace(defaults.GlooSystem)

			vsEast := vsBuilder.
				WithName("vs-east").
				WithDomain("east.com").
				WithRouteActionToUpstream(simpleRouteName, testUpstream.Upstream).
				WithPrefixMatcher(simpleRouteName, "/east").
				Build()

			vsWest := vsBuilder.
				WithName("vs-west").
				WithDomain("west.com").
				WithRouteActionToUpstream(simpleRouteName, testUpstream.Upstream).
				WithPrefixMatcher(simpleRouteName, "/west").
				Build()

			resourcesToCreate.Gateways = v1.GatewayList{
				gatewaydefaults.DefaultHybridGateway(defaults.GlooSystem),
			}
			resourcesToCreate.VirtualServices = v1.VirtualServiceList{
				vsEast, vsWest,
			}
		})

		Context("IsolateVirtualHostsBySslConfig = false", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = false
			})

			It("produces a Proxy with a single HybridListener", func() {
				proxy, err := testClients.ProxyClient.Read(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				Expect(proxy.GetListeners()).To(HaveLen(1))
				Expect(proxy.GetListeners()[0].GetHybridListener()).NotTo(BeNil())
			})

			It("routes requests to all routes on gateway", func() {
				TestUpstreamReachable("east.com", "/east/1")
				TestUpstreamReachable("west.com", "/west/1")
			})

		})

		Context("IsolateVirtualHostsBySslConfig = true", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = true
			})

			It("produces a Proxy with a single AggregateListener", func() {
				proxy, err := testClients.ProxyClient.Read(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				Expect(proxy.GetListeners()).To(HaveLen(1))
				Expect(proxy.GetListeners()[0].GetAggregateListener()).NotTo(BeNil())
			})

			It("routes requests to all routes on gateway", func() {
				TestUpstreamReachable("east.com", "/east/1")
				TestUpstreamReachable("west.com", "/west/1")
			})

		})

	})

	Context("Secure HybridGateway", func() {

		var (
			eastCert, eastPK = gloohelpers.Certificate(), gloohelpers.PrivateKey()
			westCert, westPK = gloohelpers.GetCerts(gloohelpers.Params{
				Hosts: "other-host",
				IsCA:  false,
			})
		)

		TestUpstreamReturns := func(host, path, cert string, responseStatus int) {
			v1helpers.ExpectCurlWithOffset(
				1,
				v1helpers.CurlRequest{
					RootCA: &cert,
					Port:   defaults.HybridPort,
					Host:   host,
					Path:   path,
					Body:   []byte("solo.io test"),
				},
				v1helpers.CurlResponse{
					Status:  responseStatus,
					Message: "",
				})
		}

		BeforeEach(func() {
			simpleRouteName := "simple-route"
			vsBuilder := gloohelpers.NewVirtualServiceBuilder().WithNamespace(defaults.GlooSystem)

			eastTLSSecret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "east-tls-secret",
					Namespace: defaults.GlooSystem,
				},
				Kind: &gloov1.Secret_Tls{
					Tls: &gloov1.TlsSecret{
						CertChain:  eastCert,
						PrivateKey: eastPK,
					},
				},
			}
			westTLSSecret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "west-tls-secret",
					Namespace: defaults.GlooSystem,
				},
				Kind: &gloov1.Secret_Tls{
					Tls: &gloov1.TlsSecret{
						CertChain:  westCert,
						PrivateKey: westPK,
					},
				},
			}

			vsEast := vsBuilder.
				WithName("vs-east").
				WithDomain("east.com").
				WithRouteActionToUpstream(simpleRouteName, testUpstream.Upstream).
				WithPrefixMatcher(simpleRouteName, "/east").
				WithSslConfig(&gloov1.SslConfig{
					SslSecrets: &gloov1.SslConfig_SecretRef{
						SecretRef: eastTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()

			vsWest := vsBuilder.
				WithName("vs-west").
				WithDomain("west.com").
				WithRouteActionToUpstream(simpleRouteName, testUpstream.Upstream).
				WithPrefixMatcher(simpleRouteName, "/west").
				WithSslConfig(&gloov1.SslConfig{
					OneWayTls: &wrappers.BoolValue{
						Value: false,
					},
					SniDomains: []string{"west.com"},
					SslSecrets: &gloov1.SslConfig_SecretRef{
						SecretRef: westTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()

			resourcesToCreate.Gateways = v1.GatewayList{
				gatewaydefaults.DefaultHybridSslGateway(defaults.GlooSystem),
			}
			resourcesToCreate.VirtualServices = v1.VirtualServiceList{
				vsEast, vsWest,
			}
			resourcesToCreate.Secrets = gloov1.SecretList{
				eastTLSSecret, westTLSSecret,
			}
		})

		Context("IsolateVirtualHostsBySslConfig = false", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = false
			})

			It("produces a Proxy with a single HybridListener", func() {
				proxy, err := testClients.ProxyClient.Read(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				Expect(proxy.GetListeners()).To(HaveLen(1))
				Expect(proxy.GetListeners()[0].GetHybridListener()).NotTo(BeNil())
			})

			It("routes requests to all routes on gateway", func() {
				// This test demonstrates the flaw with HttpListeners:
				//	The West VirtualService should only be exposing routes if the westCert is provided,
				//	but in this test we can successfully execute requests against the west routes,
				//	by providing an east certificate.
				//
				// This is due to the fact that an HttpListener creates an aggregate set of RouteConfiguration
				// and then produces duplicate FilterChains, based on all available SslConfig's from VirtualServices
				TestUpstreamReturns("east.com", "/east/1", eastCert, http.StatusOK)
				TestUpstreamReturns("west.com", "/west/1", eastCert, http.StatusOK)
			})

		})

		Context("IsolateVirtualHostsBySslConfig = true", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = true
			})

			It("produces a Proxy with a single AggregateListener", func() {
				proxy, err := testClients.ProxyClient.Read(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())

				Expect(proxy.GetListeners()).To(HaveLen(1))
				Expect(proxy.GetListeners()[0].GetAggregateListener()).NotTo(BeNil())
			})

			It("routes requests to all routes on gateway", func() {
				// This test demonstrates the solution with AggregateListeners:
				//	The West VirtualService is no longer routable with the eastCert.
				TestUpstreamReturns("east.com", "/east/1", eastCert, http.StatusOK)
				TestUpstreamReturns("west.com", "/west/1", eastCert, http.StatusNotFound)
			})

		})

	})

})
