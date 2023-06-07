package e2e_test

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"

	"math/rand"

	"github.com/solo-io/gloo/test/gomega/matchers"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	proxy_protocol "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/proxy_protocol"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/test/e2e"
)

type ClientConnectionProperties struct {
	SrcIp net.IP
	SNI   string
}

type GwTester struct {
	secret      *gloov1.Secret
	testContext *e2e.TestContext
}

const NoMatch = "nothing matched"

// Configure the gateway with the provided `matchers`, then send a request
// against the gateway using the information in ClientConnectionProperties and
// return the matcher that is matched.
func (gt *GwTester) getMatchedMatcher(cp ClientConnectionProperties, matchers map[string]*v1.Matcher) string {
	gt.configureEnvoy(matchers)

	// no need for an Eventually block since envoy is configured at this point
	var stringBody string
	resp, err := gt.makeARequest(gt.testContext, cp.SrcIp, cp.SNI)
	if errors.Is(err, syscall.ECONNRESET) {
		// connection properties does not match any of the matchers
		return NoMatch
	}
	Expect(err).NotTo(HaveOccurred())

	defer resp.Body.Close()
	bBody, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	stringBody = string(bBody)

	return stringBody
}

func (gt *GwTester) configureEnvoy(requestMatchers map[string]*v1.Matcher) {
	// create a magic servername value to ensure that envoy is configured
	// then we send a request against this magic servername to make sure
	// envoy has been fully configured
	magicServerName := fmt.Sprintf("%d", rand.Uint32()) + ".com"
	requestMatchers[magicServerName] = &v1.Matcher{
		SslConfig: &ssl.SslConfig{
			SniDomains: []string{magicServerName},
		},
	}
	vss, gw := gt.getGwWithMatches(magicServerName, requestMatchers)

	writeOptions := clients.WriteOpts{
		Ctx:               gt.testContext.Ctx(),
		OverwriteExisting: true,
	}
	c := gt.testContext.TestClients()

	By("writing snapshot with updated gw config")
	for _, vs := range vss {
		_, err := c.VirtualServiceClient.Write(vs, writeOptions)
		Expect(err).NotTo(HaveOccurred())
	}
	_, err := c.GatewayClient.Write(gw, writeOptions)
	Expect(err).NotTo(HaveOccurred())

	// use magicservername to ensure envoy has latest config
	Eventually(func(g Gomega) {
		resp, err := gt.makeARequest(gt.testContext, net.ParseIP("127.0.0.1"), magicServerName)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(matchers.HaveOkResponse())
	}, "5s", "0.1s").Should(Succeed())
}

func (gt *GwTester) getGwWithMatches(configver string, matches map[string]*v1.Matcher) ([]*v1.VirtualService, *v1.Gateway) {
	gw := gatewaydefaults.DefaultHybridGateway(writeNamespace)
	var virtualServices []*v1.VirtualService
	gw.Options = &gloov1.ListenerOptions{
		ProxyProtocol: &proxy_protocol.ProxyProtocol{},
	}
	vsopts := &gloov1.VirtualHostOptions{
		HeaderManipulation: &headers.HeaderManipulation{
			ResponseHeadersToAdd: []*headers.HeaderValueOption{{
				Header: &headers.HeaderValue{
					Key:   "x-gloo-configver",
					Value: configver,
				},
				Append: &wrappers.BoolValue{
					Value: true,
				},
			}},
		},
	}
	var matchedGw []*v1.MatchedGateway

	i := 0
	for name, m := range matches {
		i++
		curVs := gatewaydefaults.DirectResponseVirtualService(gw.Metadata.Namespace, fmt.Sprintf("vs-%s-%d", configver, i), name)
		curVs.VirtualHost.Options = vsopts
		virtualServices = append(virtualServices, curVs)
		matchedGw = append(matchedGw, &v1.MatchedGateway{
			Matcher: m,
			GatewayType: &v1.MatchedGateway_HttpGateway{
				HttpGateway: &v1.HttpGateway{
					VirtualServices: []*core.ResourceRef{
						curVs.Metadata.Ref(),
					},
				},
			},
		})
	}

	for _, v := range virtualServices {
		v.SslConfig = &ssl.SslConfig{
			SslSecrets: &ssl.SslConfig_SecretRef{
				SecretRef: gt.secret.Metadata.Ref(),
			},
		}
	}

	gw.GetHybridGateway().MatchedGateways = matchedGw
	return virtualServices, gw
}

func (gt *GwTester) makeARequest(testContext *e2e.TestContext, srcip net.IP, sni string) (*http.Response, error) {
	if srcip == nil {
		srcip = net.ParseIP("127.0.0.1")
	}
	requestBuilder := testContext.GetHttpsRequestBuilder().WithPort(testContext.EnvoyInstance().HybridPort)
	proxyProtocolBytes = []byte("PROXY TCP4 " + srcip.String() + " 1.2.3.4 123 123\r\n")
	client := testutils.DefaultClientBuilder().
		WithTLSRootCa(gloohelpers.Certificate()).
		WithProxyProtocolBytes(proxyProtocolBytes).
		WithTLSServerName(sni).
		Build()

	// skip ssl verification as it would not work for test.com
	//       Get "https://localhost:8087/": tls: failed to verify certificate: x509: certificate is valid for gateway-proxy, knative-proxy, ingress-proxy, not test.com
	client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true

	return client.Do(requestBuilder.Build())
}

var _ = Describe("Hybrid Gateway", func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
		testContext.BeforeEach()
		// limit default gateway to the default vs, so it doesn't catch the new vs we generate
		testContext.ResourcesToCreate().Gateways[0].GetHttpGateway().VirtualServices = []*core.ResourceRef{
			testContext.ResourcesToCreate().VirtualServices[0].GetMetadata().Ref(),
		}
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("catchall match for http", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultHybridGateway(writeNamespace)
			gw.GetHybridGateway().MatchedGateways = []*v1.MatchedGateway{
				// HttpGateway gets a catchall matcher
				{
					GatewayType: &v1.MatchedGateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{},
					},
				},

				// TcpGateway gets a matcher our request *will not* hit
				{
					Matcher: &v1.Matcher{
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
						},
					},
					GatewayType: &v1.MatchedGateway_TcpGateway{
						TcpGateway: &v1.TcpGateway{},
					},
				},
			}

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gw,
			}
		})

		It("http request works as expected", func() {
			requestBuilder := testContext.GetHttpRequestBuilder().WithPort(testContext.EnvoyInstance().HybridPort)
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())
			}, "5s", "0.5s").Should(Succeed())
		})

	})

	Context("SourcePrefixRanges match for http", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultHybridGateway(writeNamespace)
			gw.GetHybridGateway().MatchedGateways = []*v1.MatchedGateway{
				// HttpGateway gets a matcher our request will hit
				{
					Matcher: &v1.Matcher{
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "255.0.0.0",
								PrefixLen: &wrappers.UInt32Value{
									Value: 1,
								},
							},
							{
								AddressPrefix: "0.0.0.0",
								PrefixLen: &wrappers.UInt32Value{
									Value: 1,
								},
							},
						},
					},
					GatewayType: &v1.MatchedGateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{},
					},
				},
			}

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gw,
			}
		})

		It("http request works as expected", func() {
			requestBuilder := testContext.GetHttpRequestBuilder().WithPort(testContext.EnvoyInstance().HybridPort)
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())
			}, "5s", "0.5s").Should(Succeed())
		})

	})

	Context("SourcePrefixRanges miss for tcp", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultHybridGateway(writeNamespace)

			gw.GetHybridGateway().MatchedGateways = []*v1.MatchedGateway{
				// HttpGateway gets a filter our request *will not* hit
				{
					Matcher: &v1.Matcher{
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
						},
					},
					GatewayType: &v1.MatchedGateway_HttpGateway{
						HttpGateway: &v1.HttpGateway{},
					},
				},
			}

			testContext.ResourcesToCreate().Gateways = v1.GatewayList{
				gw,
			}
		})

		It("http request fails", func() {
			requestBuilder := testContext.GetHttpRequestBuilder().WithPort(testContext.EnvoyInstance().HybridPort)
			Consistently(func(g Gomega) {
				_, err := testutils.DefaultHttpClient.Do(requestBuilder.Build())
				g.Expect(err).Should(HaveOccurred())
			}, "3s", "0.5s").Should(Succeed())
		})

	})

	Context("permutations of servername and SourcePrefixRanges", func() {
		/*
			Currently, gloo exposes 2 fields that are used in filter chain
			matchers: SNI servername, and SourcePrefixRanges. When these values are
			set, Envoy's behaviour (using the old filter chain match API) is to (1)
			match on the *most specific* servername first, then (2) see if a
			matching value is present for SourcePrefixRanges

			This set of tests tries to comprehensively test all possible
			permutations of the outcomes of these 2 matchers to ensure that our
			implemented use of the new API does not create any regressions.
		*/

		var (
			secret *gloov1.Secret
			tester *GwTester
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

			testContext.ResourcesToCreate().Secrets = gloov1.SecretList{
				secret,
			}
			tester = &GwTester{
				secret:      secret,
				testContext: testContext,
			}
		})

		// Table test:
		// Each entry contains a connection properties struct that is used to
		// create a request and a map of named matchers (name is arbitrary)
		// to configure envoy with.
		// The last argument is the name of the matcher that should match,
		// or `NoMatch` if nothing should match.
		DescribeTable("SetResource[Invalid|Valid] works as expected",
			func(cp ClientConnectionProperties, matches map[string]*v1.Matcher, expected string) {
				// uncomment to dump envoy config
				// defer func() {
				// 	config, _ := testContext.EnvoyInstance().ConfigDump()
				// 	fmt.Println(config)
				// }()
				Expect(tester.getMatchedMatcher(cp, matches)).To(Equal(expected))
			},
			Entry("no match",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.4"),
					SNI:   "test.com",
				},
				map[string]*v1.Matcher{
					"sni-star": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"*.foo.com"},
						},
					},
				}, NoMatch),
			Entry("ip non-match (half ip address) without sni",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("2.2.3.4"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"ip-matcher": {
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 16,
								},
							},
						},
					},
				}, NoMatch),
			// for the next 2 entries, no filter chain match is recorded
			// because the filter chain translator aborts translation if there
			// are no network filters (ie. virtual hosts)
			// https://github.com/solo-io/gloo/blob/d3879f282da00dc0cb6c8c9366a87b48ca1a382b/projects/gloo/pkg/translator/filter_chain.go#L94-L96
			// so even though the ip matches, we expect the request to fail
			// similar to the above test.
			// there is a workaround for this: by setting SniDomains to '*.', a
			// virtual host *will* be created that matches all sni domains -
			// see the tests a bit further below.
			Entry("ip matcher (full ip address) without sni",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.4"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"ip-matcher": {
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
						},
					},
				}, NoMatch),
			Entry("ip matcher (half ip address) without sni",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.5"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"ip-matcher": {
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 16,
								},
							},
						},
					},
				}, NoMatch),
			Entry("ip matcher (half ip address) with full wildcard sni (client SNI empty)",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.5"),
					// SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"ip-matcher": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"*."},
						},
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 16,
								},
							},
						},
					},
				}, NoMatch),
			Entry("ip matcher (half ip address) with full wildcard sni",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.5"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"ip-matcher": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"*."},
						},
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 16,
								},
							},
						},
					},
				}, NoMatch),
			Entry("sni match",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.4"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"sni-star": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"*.test.com"},
						},
					},
				}, "sni-star"),
			Entry("sni and ip match",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.4"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"sni-star": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"*.test.com"},
						},
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
						},
					},
				}, "sni-star"),
			Entry("sni match, ip non-match",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.4"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"sni-star": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"*.test.com"},
						},
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "2.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
						},
					},
				}, NoMatch),
			Entry("sni non-match, ip match",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.4"),
					SNI:   "test.com",
				},
				map[string]*v1.Matcher{
					"sni-star": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"*.foo.com"},
						},
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
						},
					},
				}, NoMatch),
			Entry("most specific sni matcher",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.4"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"less-specific": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"*.test.com"},
						},
					},
					"more-specific": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"foo.test.com", "*.com"},
						},
					},
				}, "more-specific"),
			Entry("most specific sni matcher with invalid source ip",
				// envoy parses sni domain first - it matches 'more-specific',
				// and then ignores all other matchers at the same level (ie.
				// the 'less-specific' matcher). as envoy descends through the
				// 'more-specific' branch, it finds no matching
				// SourcePrefixRanges values, so it returns no filter chain
				// found
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.4"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"less-specific": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"*.test.com"},
						},
					},
					"more-specific": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"foo.test.com"},
						},
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "2.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
						},
					},
				}, NoMatch),
			Entry("sni matcher with multiple source ip",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.4"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"more-specific": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"foo.test.com"},
						},
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "2.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
							{
								AddressPrefix: "1.2.3.4",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
						},
					},
				}, "more-specific"),
			Entry("sni matcher with multiple source ip, less precise CIDR range match",
				ClientConnectionProperties{
					SrcIp: net.ParseIP("1.2.3.4"),
					SNI:   "foo.test.com",
				},
				map[string]*v1.Matcher{
					"more-specific": {
						SslConfig: &ssl.SslConfig{
							SniDomains: []string{"foo.test.com"},
						},
						SourcePrefixRanges: []*v3.CidrRange{
							{
								AddressPrefix: "2.3.4.5",
								PrefixLen: &wrappers.UInt32Value{
									Value: 32,
								},
							},
							{
								AddressPrefix: "1.2.0.0",
								PrefixLen: &wrappers.UInt32Value{
									Value: 16,
								},
							},
						},
					},
				}, "more-specific"),
		)

	})
})
