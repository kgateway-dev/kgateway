package e2e_test

import (
	"net/http"

	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/e2e"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/selectors"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/v1helpers"

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
		isolateVirtualHostsBySslConfig bool

		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
		testContext.BeforeEach()
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.SetRunSettings(&gloov1.Settings{
			Gateway: &gloov1.GatewayOptions{
				IsolateVirtualHostsBySslConfig: &wrappers.BoolValue{
					Value: isolateVirtualHostsBySslConfig,
				},
			},
		})

		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("Insecure HttpGateway", func() {

		BeforeEach(func() {
			vsBuilder := gloohelpers.NewVirtualServiceBuilder().WithNamespace(writeNamespace)

			vsEast := vsBuilder.Clone().
				WithName("vs-east").
				WithDomain("east.com").
				WithRouteActionToUpstream(e2e.DefaultRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(e2e.DefaultRouteName, "/east").
				Build()

			vsWest := vsBuilder.Clone().
				WithName("vs-west").
				WithDomain("west.com").
				WithRouteActionToUpstream(e2e.DefaultRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(e2e.DefaultRouteName, "/west").
				Build()

			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vsEast, vsWest,
			}
		})

		Context("IsolateVirtualHostsBySslConfig = false", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = false
			})

			It("produces a Proxy with a single HttpListener", func() {
				proxy, err := testContext.ReadDefaultProxy()
				Expect(err).NotTo(HaveOccurred())

				Expect(proxy.GetListeners()).To(HaveLen(1))
				Expect(proxy.GetListeners()[0].GetHttpListener()).NotTo(BeNil())
			})

			DescribeTable("routes requests",
				func(host, path string, responseMatcher types.GomegaMatcher) {
					httpRequestBuilder := testContext.GetHttpRequestBuilder().
						WithHost(host).
						WithPath(path)

					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(httpRequestBuilder.Build())).To(responseMatcher)
					}, "10s", "1s").Should(Succeed())
				},
				Entry("east host",
					"east.com",
					"east/1",
					matchers.HaveOkResponse()),
				Entry("west host",
					"west.com",
					"west/1",
					matchers.HaveOkResponse()),
			)
		})

		Context("IsolateVirtualHostsBySslConfig = true", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = true
			})

			It("produces a Proxy with a single AggregateListener", func() {
				proxy, err := testContext.ReadDefaultProxy()
				Expect(err).NotTo(HaveOccurred())

				Expect(proxy.GetListeners()).To(HaveLen(1))
				Expect(proxy.GetListeners()[0].GetAggregateListener()).NotTo(BeNil())
			})

			DescribeTable("routes requests",
				func(host, path string, responseMatcher types.GomegaMatcher) {
					httpRequestBuilder := testContext.GetHttpRequestBuilder().
						WithHost(host).
						WithPath(path)

					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(httpRequestBuilder.Build())).To(responseMatcher)
					}, "10s", "1s").Should(Succeed())
				},
				Entry("east host",
					"east.com",
					"east/1",
					matchers.HaveOkResponse()),
				Entry("west host",
					"west.com",
					"west/1",
					matchers.HaveOkResponse()),
			)
		})

	})

	Context("Secure HttpGateway", func() {

		var (
			eastCert, eastPK = gloohelpers.GetCerts(gloohelpers.Params{
				Hosts: "east.com",
				IsCA:  false,
			})
			westCert, westPK = gloohelpers.GetCerts(gloohelpers.Params{
				Hosts: "west.com,northwest.com,southwest.com",
				IsCA:  false,
			})
		)

		BeforeEach(func() {
			simpleRouteName := "simple-route"
			vsBuilder := gloohelpers.NewVirtualServiceBuilder().WithNamespace(writeNamespace)

			eastTLSSecret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "east-tls-secret",
					Namespace: writeNamespace,
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
					Namespace: writeNamespace,
				},
				Kind: &gloov1.Secret_Tls{
					Tls: &gloov1.TlsSecret{
						CertChain:  westCert,
						PrivateKey: westPK,
					},
				},
			}

			vsEast := vsBuilder.Clone().
				WithName("vs-east").
				WithDomain("east.com").
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/east").
				WithSslConfig(&ssl.SslConfig{
					SniDomains: []string{"east.com"},
					SslSecrets: &ssl.SslConfig_SecretRef{
						SecretRef: eastTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()
			vsNorthWest := vsBuilder.Clone().
				WithName("vs-northwest").
				WithDomain("northwest.com").
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/northwest").
				WithSslConfig(&ssl.SslConfig{
					OneWayTls: &wrappers.BoolValue{
						Value: false,
					},
					SniDomains: []string{"northwest.com"},
					SslSecrets: &ssl.SslConfig_SecretRef{
						SecretRef: westTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()
			vsSouthWest := vsBuilder.Clone().
				WithName("vs-southwest").
				WithDomain("southwest.com").
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/southwest").
				WithSslConfig(&ssl.SslConfig{
					OneWayTls: &wrappers.BoolValue{
						Value: false,
					},
					SniDomains: []string{"southwest.com"},
					SslSecrets: &ssl.SslConfig_SecretRef{
						SecretRef: westTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gatewaydefaults.DefaultSslGateway(writeNamespace),
			}
			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vsEast, vsNorthWest, vsSouthWest,
			}
			testContext.ResourcesToCreate().Secrets = gloov1.SecretList{
				eastTLSSecret, westTLSSecret,
			}
		})

		Context("IsolateVirtualHostsBySslConfig = false", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = false
			})

			It("produces a Proxy with a single HttpListener", func() {
				Eventually(func(g Gomega) {
					proxy, err := testContext.ReadDefaultProxy()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(proxy.GetListeners()).To(HaveLen(1))
					g.Expect(proxy.GetListeners()[0].GetHttpListener()).NotTo(BeNil())
				}).Should(Succeed())
			})

			DescribeTable("routes requests",
				func(serverName, host, path, cert string, responseMatcher types.GomegaMatcher) {
					httpClient := testutils.DefaultClientBuilder().
						WithTLSRootCa(cert).
						WithTLSServerName(serverName).
						Build()

					httpRequestBuilder := testContext.GetHttpsRequestBuilder().
						WithHost(host).
						WithPath(path)

					Eventually(func(g Gomega) {
						g.Expect(httpClient.Do(httpRequestBuilder.Build())).To(responseMatcher)
					}, "10s", "1s").Should(Succeed())
				},
				// This test demonstrates the flaw with HttpListeners:
				//	The West VirtualServices should only be exposing routes if the westCert is provided,
				//	but in this test we can successfully execute requests against the west routes,
				//	by providing an east certificate.
				//
				// This is due to the fact that an HttpListener creates an aggregate set of RouteConfiguration
				// and then produces duplicate FilterChains, based on all available SslConfig's from VirtualServices
				Entry("east host with east cert",
					"east.com",
					"east.com",
					"east/1",
					eastCert,
					matchers.HaveOkResponse()),
				Entry("northwest host with east cert",
					"east.com",
					"northwest.com",
					"northwest/1",
					eastCert,
					matchers.HaveOkResponse()),
				Entry("southwest host with east cert",
					"east.com",
					"southwest.com",
					"southwest/1",
					eastCert,
					matchers.HaveOkResponse()),
			)

		})

		Context("IsolateVirtualHostsBySslConfig = true", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = true
			})

			It("produces a Proxy with a single AggregateListener", func() {
				Eventually(func(g Gomega) {
					proxy, err := testContext.ReadDefaultProxy()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(proxy.GetListeners()).To(HaveLen(1))
					g.Expect(proxy.GetListeners()[0].GetAggregateListener()).NotTo(BeNil())
				}).Should(Succeed())
			})

			DescribeTable("routes requests",
				func(serverName, host, path, cert string, responseMatcher types.GomegaMatcher) {
					httpClient := testutils.DefaultClientBuilder().
						WithTLSRootCa(cert).
						WithTLSServerName(serverName).
						Build()

					httpRequestBuilder := testContext.GetHttpsRequestBuilder().
						WithHost(host).
						WithPath(path)

					Eventually(func(g Gomega) {
						g.Expect(httpClient.Do(httpRequestBuilder.Build())).To(responseMatcher)
					}, "10s", "1s").Should(Succeed())
				},
				// This test demonstrates the solution with AggregateListeners:
				//	The West VirtualService is no longer routable with the eastCert.
				Entry("east host with east cert",
					"east.com",
					"east.com",
					"east/1",
					eastCert,
					matchers.HaveOkResponse()),
				Entry("northwest host with east cert",
					"east.com",
					"northwest.com",
					"northwest/1",
					eastCert,
					matchers.HaveStatusCode(http.StatusNotFound)),
				Entry("southwest host with east cert",
					"east.com",
					"southwest.com",
					"southwest/1",
					eastCert,
					matchers.HaveStatusCode(http.StatusNotFound)),
				Entry("northwest host with west cert",
					"northwest.com",
					"northwest.com",
					"northwest/1",
					westCert,
					matchers.HaveOkResponse()),
				Entry("southwest host with west cert",
					"southwest.com",
					"southwest.com",
					"southwest/1",
					westCert,
					matchers.HaveOkResponse()),
			)
		})

	})

	Context("Insecure HybridGateway (Matched)", func() {

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
					Message: "solo.io test",
				})
		}

		BeforeEach(func() {
			simpleRouteName := "simple-route"
			vsBuilder := gloohelpers.NewVirtualServiceBuilder().WithNamespace(writeNamespace)

			vsEast := vsBuilder.
				WithName("vs-east").
				WithDomain("east.com").
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/east").
				Build()

			vsWest := vsBuilder.
				WithName("vs-west").
				WithDomain("west.com").
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/west").
				Build()

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gatewaydefaults.DefaultHybridGateway(writeNamespace),
			}
			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vsEast, vsWest,
			}
		})

		Context("IsolateVirtualHostsBySslConfig = false", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = false
			})

			It("produces a Proxy with a single HybridListener", func() {
				Eventually(func(g Gomega) {
					proxy, err := testContext.ReadDefaultProxy()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(proxy.GetListeners()).To(HaveLen(1))
					g.Expect(proxy.GetListeners()[0].GetHybridListener()).NotTo(BeNil())
				}).Should(Succeed())
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
				proxy, err := testContext.ReadDefaultProxy()
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

	Context("Secure HybridGateway (Matched)", func() {

		var (
			eastCert, eastPK = gloohelpers.Certificate(), gloohelpers.PrivateKey()
			westCert, westPK = gloohelpers.GetCerts(gloohelpers.Params{
				Hosts: "other-host",
				IsCA:  false,
			})
		)

		TestUpstreamReturns := func(host, path, cert string, responseStatus int) {
			expectedResponseBody := ""
			if responseStatus == http.StatusOK {
				expectedResponseBody = "solo.io test"
			}

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
					Message: expectedResponseBody,
				})
		}

		BeforeEach(func() {
			simpleRouteName := "simple-route"
			vsBuilder := gloohelpers.NewVirtualServiceBuilder().WithNamespace(writeNamespace)

			eastTLSSecret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "east-tls-secret",
					Namespace: writeNamespace,
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
					Namespace: writeNamespace,
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
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/east").
				WithSslConfig(&ssl.SslConfig{
					SslSecrets: &ssl.SslConfig_SecretRef{
						SecretRef: eastTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()

			vsWest := vsBuilder.
				WithName("vs-west").
				WithDomain("west.com").
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/west").
				WithSslConfig(&ssl.SslConfig{
					OneWayTls: &wrappers.BoolValue{
						Value: false,
					},
					SniDomains: []string{"west.com"},
					SslSecrets: &ssl.SslConfig_SecretRef{
						SecretRef: westTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gatewaydefaults.DefaultHybridSslGateway(writeNamespace),
			}
			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vsEast, vsWest,
			}
			testContext.ResourcesToCreate().Secrets = gloov1.SecretList{
				eastTLSSecret, westTLSSecret,
			}
		})

		Context("IsolateVirtualHostsBySslConfig = false", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = false
			})

			It("produces a Proxy with a single HybridListener", func() {
				Eventually(func(g Gomega) {
					proxy, err := testContext.ReadDefaultProxy()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(proxy.GetListeners()).To(HaveLen(1))
					g.Expect(proxy.GetListeners()[0].GetHybridListener()).NotTo(BeNil())
				}).Should(Succeed())
			})

			It("routes requests to all routes on gateway", func() {
				// This test demonstrates the flaw with HybridListeners:
				//	The West VirtualService should only be exposing routes if the westCert is provided,
				//	but in this test we can successfully execute requests against the west routes,
				//	by providing an east certificate.
				//
				// This is due to the fact that a HybridListener creates an aggregate set of RouteConfiguration
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
				Eventually(func(g Gomega) {
					proxy, err := testContext.ReadDefaultProxy()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(proxy.GetListeners()).To(HaveLen(1))
					g.Expect(proxy.GetListeners()[0].GetAggregateListener()).NotTo(BeNil())
				}).Should(Succeed())
			})

			It("routes requests to all routes on gateway", func() {
				// This test demonstrates the solution with AggregateListeners:
				//	The West VirtualService is no longer routable with the eastCert.
				TestUpstreamReturns("east.com", "/east/1", eastCert, http.StatusOK)
				TestUpstreamReturns("west.com", "/west/1", eastCert, http.StatusNotFound)
			})

		})

	})

	Context("Insecure HybridGateway (Delegated)", func() {

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
					Message: "solo.io test",
				})
		}

		BeforeEach(func() {
			simpleRouteName := "simple-route"
			vsBuilder := gloohelpers.NewVirtualServiceBuilder().WithNamespace(writeNamespace)

			vsEast := vsBuilder.
				WithName("vs-east").
				WithDomain("east.com").
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/east").
				Build()

			vsWest := vsBuilder.
				WithName("vs-west").
				WithDomain("west.com").
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/west").
				Build()

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				buildInsecureHybridGatewayWithDelegation(writeNamespace),
			}
			testContext.ResourcesToCreate().HttpGateways = v1.MatchableHttpGatewayList{
				gatewaydefaults.DefaultMatchableHttpGateway(writeNamespace, nil),
			}
			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vsEast, vsWest,
			}
		})

		Context("IsolateVirtualHostsBySslConfig = false", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = false
			})

			It("produces a Proxy with a single HybridListener", func() {
				proxy, err := testContext.ReadDefaultProxy()
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
				proxy, err := testContext.ReadDefaultProxy()
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

	Context("Secure HybridGateway (Delegated)", func() {

		var (
			eastCert, eastPK = gloohelpers.Certificate(), gloohelpers.PrivateKey()
			westCert, westPK = gloohelpers.GetCerts(gloohelpers.Params{
				Hosts: "other-host",
				IsCA:  false,
			})
		)

		TestUpstreamReturns := func(host, path, cert string, responseStatus int) {
			expectedResponseBody := ""
			if responseStatus == http.StatusOK {
				expectedResponseBody = "solo.io test"
			}

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
					Message: expectedResponseBody,
				})
		}

		BeforeEach(func() {
			simpleRouteName := "simple-route"
			vsBuilder := gloohelpers.NewVirtualServiceBuilder().WithNamespace(writeNamespace)

			eastTLSSecret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "east-tls-secret",
					Namespace: writeNamespace,
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
					Namespace: writeNamespace,
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
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/east").
				WithSslConfig(&ssl.SslConfig{
					SslSecrets: &ssl.SslConfig_SecretRef{
						SecretRef: eastTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()

			vsWest := vsBuilder.
				WithName("vs-west").
				WithDomain("west.com").
				WithRouteActionToUpstream(simpleRouteName, testContext.TestUpstream().Upstream).
				WithRoutePrefixMatcher(simpleRouteName, "/west").
				WithSslConfig(&ssl.SslConfig{
					OneWayTls: &wrappers.BoolValue{
						Value: false,
					},
					SniDomains: []string{"west.com"},
					SslSecrets: &ssl.SslConfig_SecretRef{
						SecretRef: westTLSSecret.GetMetadata().Ref(),
					},
				}).
				Build()

			nonEmptySslConfig := &ssl.SslConfig{
				TransportSocketConnectTimeout: &duration.Duration{
					Seconds: 30,
				},
			}

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				buildHybridGatewayWithDelegation(writeNamespace, nonEmptySslConfig),
			}
			testContext.ResourcesToCreate().HttpGateways = v1.MatchableHttpGatewayList{
				gatewaydefaults.DefaultMatchableHttpGateway(writeNamespace, nonEmptySslConfig),
			}
			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vsEast, vsWest,
			}
			testContext.ResourcesToCreate().Secrets = gloov1.SecretList{
				eastTLSSecret, westTLSSecret,
			}
		})

		Context("IsolateVirtualHostsBySslConfig = false", func() {

			BeforeEach(func() {
				isolateVirtualHostsBySslConfig = false
			})

			It("produces a Proxy with a single HybridListener", func() {
				Eventually(func(g Gomega) {
					proxy, err := testContext.ReadDefaultProxy()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(proxy.GetListeners()).To(HaveLen(1))
					g.Expect(proxy.GetListeners()[0].GetHybridListener()).NotTo(BeNil())
				}).Should(Succeed())
			})

			It("routes requests to all routes on gateway", func() {
				// This test demonstrates the flaw with HybridListeners:
				//	The West VirtualService should only be exposing routes if the westCert is provided,
				//	but in this test we can successfully execute requests against the west routes,
				//	by providing an east certificate.
				//
				// This is due to the fact that a HybridListener creates an aggregate set of RouteConfiguration
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
				Eventually(func(g Gomega) {
					proxy, err := testContext.ReadDefaultProxy()
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(proxy.GetListeners()).To(HaveLen(1))
					g.Expect(proxy.GetListeners()[0].GetAggregateListener()).NotTo(BeNil())
				}).Should(Succeed())
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

func buildInsecureHybridGatewayWithDelegation(writeNamespace string) *v1.Gateway {
	return buildHybridGatewayWithDelegation(writeNamespace, nil)
}

func buildHybridGatewayWithDelegation(writeNamespace string, sslConfig *ssl.SslConfig) *v1.Gateway {
	gw := gatewaydefaults.DefaultHybridGateway(writeNamespace)
	gw.GatewayType = &v1.Gateway_HybridGateway{
		HybridGateway: &v1.HybridGateway{
			DelegatedHttpGateways: &v1.DelegatedHttpGateway{
				SslConfig: sslConfig,
				SelectionType: &v1.DelegatedHttpGateway_Selector{
					Selector: &selectors.Selector{
						// select all MatchableHttpGateways in the same namespace
						Namespaces: []string{writeNamespace},
					},
				},
			},
		},
	}
	return gw
}
