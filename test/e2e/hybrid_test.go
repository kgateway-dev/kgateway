package e2e_test

import (
	"fmt"
	"net/http"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	matchers2 "github.com/solo-io/gloo/test/matchers"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var _ = Describe("Hybrid Gateway", func() {

	var (
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
		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	buildHttpRequestToHybridGateway := func() *http.Request {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/", "localhost", defaults.HybridPort), nil)
		Expect(err).NotTo(HaveOccurred())
		req.Host = "test.com" // to match the vs-test

		return req
	}

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

			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithRoutePrefixMatcher("test", "/").
				WithRouteDirectResponseAction("test", &gloov1.DirectResponseAction{
					Status: http.StatusOK,
				}).
				Build()

			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vs,
			}
		})

		It("http request works as expected", func() {
			client := &http.Client{}
			req := buildHttpRequestToHybridGateway()

			Eventually(func() (*http.Response, error) {
				return client.Do(req)
			}, "5s", "0.5s").Should(matchers2.MatchHttpResponse(&http.Response{
				StatusCode: http.StatusOK,
			}))
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

			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithRoutePrefixMatcher("test", "/").
				WithRouteDirectResponseAction("test", &gloov1.DirectResponseAction{
					Status: http.StatusOK,
				}).
				Build()

			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vs,
			}
		})

		It("http request works as expected", func() {
			client := &http.Client{}
			req := buildHttpRequestToHybridGateway()

			Eventually(func() (*http.Response, error) {
				return client.Do(req)
			}, "5s", "0.5s").Should(matchers2.MatchHttpResponse(&http.Response{
				StatusCode: http.StatusOK,
			}))

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

			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithRoutePrefixMatcher("test", "/").
				WithRouteDirectResponseAction("test", &gloov1.DirectResponseAction{
					Status: http.StatusOK,
				}).
				Build()

			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
				vs,
			}
		})

		It("http request fails", func() {
			client := &http.Client{}
			req := buildHttpRequestToHybridGateway()

			Consistently(func() error {
				_, err := client.Do(req)
				return err
			}, "3s", "0.5s").Should(HaveOccurred())

		})

	})

})
