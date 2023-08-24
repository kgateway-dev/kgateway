package e2e_test

import (
	"fmt"
	"net/http"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/local_ratelimit"
	local_ratelimit_plugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/local_ratelimit"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/testutils"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Local Rate Limit", func() {

	const (
		defaultLimit = 3
		vsLimit      = 2
		routeLimit   = 1
	)

	var (
		testContext *e2e.TestContext

		httpClient                            *http.Client
		requestBuilder                        *testutils.HttpRequestBuilder
		expectSuccess                         func()
		expectRateLimitedWithXRateLimitHeader func(int)
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
		testContext.BeforeEach()

		httpClient = testutils.DefaultHttpClient
		requestBuilder = testContext.GetHttpRequestBuilder()

		expectSuccess = func() {
			defer GinkgoRecover()
			response, err := httpClient.Do(requestBuilder.Build())
			ExpectWithOffset(1, response).Should(matchers.HaveOkResponse())
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "The connection should not be rate limited")
		}

		expectRateLimitedWithXRateLimitHeader = func(limit int) {
			defer GinkgoRecover()
			response, _ := httpClient.Do(requestBuilder.Build())
			ExpectWithOffset(1, response).To(matchers.ContainHeaders(http.Header{
				"X-Ratelimit-Limit":     []string{fmt.Sprint(limit)},
				"X-Ratelimit-Remaining": []string{"0"},
				"X-Ratelimit-Reset":     []string{"100"},
			}), "X-Ratelimit headers should be present")
			ExpectWithOffset(1, response).To(matchers.HaveHttpResponse(&matchers.HttpResponse{
				StatusCode: http.StatusTooManyRequests,
				Body:       "local_rate_limited",
			}), "should rate limit")
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

	Context("L4 Local Rate Limit", func() {
		Context("Filter not defined", func() {
			It("Should not rate limit", func() {
				// Since the filter is not defined, the stat prefix should not be present, and requests should not be rate limited
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).ToNot(ContainSubstring(local_ratelimit_plugin.NetworkFilterStatPrefix))

				expectSuccess()
				expectSuccess()
			})
		})

		Context("Filter defined", func() {
			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					L4LocalRatelimit: &local_ratelimit.TokenBucket{
						MaxTokens: defaultLimit,
						TokensPerFill: &wrapperspb.UInt32Value{
							Value: defaultLimit,
						},
						FillInterval: &durationpb.Duration{
							Seconds: 100,
						},
					},
				}

				testContext.ResourcesToCreate().Gateways = v1.GatewayList{
					gw,
				}
			})

			// TODO : Investigate this failure - either the test or the filter itself
			It("Should rate limit at the l4 layer", func() {
				expectRateLimited := func() {
					defer GinkgoRecover()
					response, err := httpClient.Do(requestBuilder.Build())
					fmt.Println(response)
					fmt.Println(err)
					ExpectWithOffset(1, response).To(matchers.HaveHttpResponse(&matchers.HttpResponse{
						StatusCode: http.StatusTooManyRequests,
						Body:       "local_rate_limited",
					}))
				}

				// The default rate limit is 3
				cfg, _ := testContext.EnvoyInstance().ConfigDump()
				fmt.Println(cfg)

				expectSuccess()
				expectSuccess()
				expectSuccess()
				expectSuccess()
				expectSuccess()
				expectRateLimited()
			})

		})
	})

	Context("HTTP Local Rate Limit", func() {

		Context("Filter not defined", func() {
			It("Should not rate limit", func() {
				// Since the filter is not defined, the stat prefix should not be present, and requests should not be rate limited
				cfg, err := testContext.EnvoyInstance().ConfigDump()
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).ToNot(ContainSubstring(local_ratelimit_plugin.HTTPFilterStatPrefix))

				expectSuccess()
				expectSuccess()
			})
		})

		Context("Filter defined", func() {
			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					HttpLocalRatelimit: &local_ratelimit.Settings{
						LocalRateLimitPerDownstreamConnection: true,
						EnableXRatelimitHeaders:               true,
						Defaults: &local_ratelimit.TokenBucket{
							MaxTokens: defaultLimit,
							TokensPerFill: &wrapperspb.UInt32Value{
								Value: defaultLimit,
							},
							FillInterval: &durationpb.Duration{
								Seconds: 100,
							},
						},
					},
				}

				testContext.ResourcesToCreate().Gateways = v1.GatewayList{
					gw,
				}
			})

			It("Should rate limit the default value config", func() {
				// The default rate limit is 3
				expectSuccess()
				expectSuccess()
				expectSuccess()
				expectRateLimitedWithXRateLimitHeader(defaultLimit)
			})

			It("Should override the default limit with the virtual service limit", func() {
				testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
					vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{
						RateLimitConfigType: &gloov1.VirtualHostOptions_Ratelimit{
							Ratelimit: &ratelimit.RateLimitVhostExtension{
								LocalRatelimit: &local_ratelimit.TokenBucket{
									MaxTokens: vsLimit,
									TokensPerFill: &wrapperspb.UInt32Value{
										Value: vsLimit,
									},
									FillInterval: &durationpb.Duration{
										Seconds: 100,
									},
								},
							},
						},
					}
					return vs
				})

				Eventually(func(g Gomega) {
					cfg, err := testContext.EnvoyInstance().ConfigDump()
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(cfg).To(ContainSubstring("typed_per_filter_config"))
				}, "5s", ".5s").Should(Succeed())

				// The rate limit of the virtual service is 2
				expectSuccess()
				expectSuccess()
				expectRateLimitedWithXRateLimitHeader(vsLimit)
			})

			It("Should override the default limit with the route limit", func() {
				testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
					vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{
						RateLimitConfigType: &gloov1.VirtualHostOptions_Ratelimit{
							Ratelimit: &ratelimit.RateLimitVhostExtension{
								LocalRatelimit: &local_ratelimit.TokenBucket{
									MaxTokens: vsLimit,
									TokensPerFill: &wrapperspb.UInt32Value{
										Value: vsLimit,
									},
									FillInterval: &durationpb.Duration{
										Seconds: 100,
									},
								},
							},
						},
					}
					vs.GetVirtualHost().GetRoutes()[0].Options = &gloov1.RouteOptions{
						RateLimitConfigType: &gloov1.RouteOptions_Ratelimit{
							Ratelimit: &ratelimit.RateLimitRouteExtension{
								LocalRatelimit: &local_ratelimit.TokenBucket{
									MaxTokens: routeLimit,
									TokensPerFill: &wrapperspb.UInt32Value{
										Value: routeLimit,
									},
									FillInterval: &durationpb.Duration{
										Seconds: 100,
									},
								},
							},
						},
					}
					return vs
				})

				Eventually(func(g Gomega) {
					cfg, err := testContext.EnvoyInstance().ConfigDump()
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(cfg).To(ContainSubstring("typed_per_filter_config"))
				}, "5s", ".5s").Should(Succeed())

				// The rate limit of the route is 1
				expectSuccess()
				expectRateLimitedWithXRateLimitHeader(routeLimit)
			})
		})
	})
})
