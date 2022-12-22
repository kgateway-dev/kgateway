package e2e_test

import (
	"fmt"
	"net/http"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/matchers"

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
		req.Host = e2e.DefaultHost

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
		})

		It("http request works as expected", func() {
			req := buildHttpRequestToHybridGateway()

			Eventually(func() (*http.Response, error) {
				return http.DefaultClient.Do(req)
			}, "5s", "0.5s").Should(matchers.HaveOkResponse())
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
			req := buildHttpRequestToHybridGateway()

			Eventually(func() (*http.Response, error) {
				return http.DefaultClient.Do(req)
			}, "5s", "0.5s").Should(matchers.HaveOkResponse())
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
			req := buildHttpRequestToHybridGateway()

			Consistently(func() error {
				_, err := http.DefaultClient.Do(req)
				return err
			}, "3s", "0.5s").Should(HaveOccurred())

		})

	})

})
