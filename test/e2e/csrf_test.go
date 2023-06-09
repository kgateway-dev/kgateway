package e2e_test

import (
	"fmt"
	"net/http"
	"time"

	"github.com/solo-io/gloo/test/services/envoy"
	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/types"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloo_config_core "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	csrf "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	gloo_type_matcher "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	glootype "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

const (
	allowedOrigin   = "allowThisOne.solo.io"
	unAllowedOrigin = "doNot.allowThisOne.solo.io"
)

var _ = Describe("CSRF", func() {

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

	Context("no filter defined", func() {

		It("should succeed with allowed origin", func() {
			EventuallyAllowedOriginResponse(buildRequestFromOrigin(allowedOrigin), testContext.EnvoyInstance(), false)
		})

		It("should succeed with un-allowed origin", func() {
			EventuallyAllowedOriginResponse(buildRequestFromOrigin(unAllowedOrigin), testContext.EnvoyInstance(), false)
		})

	})

	Context("defined on listener", func() {

		Context("only on listener", func() {

			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					Csrf: getCsrfPolicyWithFilterEnabled(allowedOrigin),
				}

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gw,
				}
			})

			It("should succeed with allowed origin", func() {
				EventuallyAllowedOriginResponse(buildRequestFromOrigin(allowedOrigin), testContext.EnvoyInstance(), true)
			})

			It("should fail with un-allowed origin", func() {
				EventuallyInvalidOriginResponse(buildRequestFromOrigin(unAllowedOrigin), testContext.EnvoyInstance(), true)
			})
		})

		Context("defined on listener with shadow mode config", func() {

			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					Csrf: getCsrfPolicyWithShadowEnabled(allowedOrigin),
				}

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gw,
				}
			})

			It("should succeed with allowed origin, unsafe request", func() {
				EventuallyAllowedOriginResponse(buildRequestFromOrigin(allowedOrigin), testContext.EnvoyInstance(), false)
			})

			It("should succeed with un-allowed origin and update invalid count", func() {
				EventuallyAllowedOriginResponse(buildRequestFromOrigin(unAllowedOrigin), testContext.EnvoyInstance(), false)
			})
		})

		Context("defined on listener with filter enabled and shadow mode config", func() {

			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					Csrf: getCsrfPolicyWithFilterEnabledAndShadow(allowedOrigin),
				}

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gw,
				}
			})

			It("should succeed with allowed origin, unsafe request", func() {
				EventuallyAllowedOriginResponse(buildRequestFromOrigin(allowedOrigin), testContext.EnvoyInstance(), true)
			})

			It("should fail with un-allowed origin and update invalid count", func() {
				EventuallyInvalidOriginResponse(buildRequestFromOrigin(unAllowedOrigin), testContext.EnvoyInstance(), true)
			})
		})

	})

	Context("enabled on route", func() {

		BeforeEach(func() {
			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithRoutePrefixMatcher("test", "/").
				WithRouteActionToUpstream("test", testContext.TestUpstream().Upstream).
				WithRouteOptions("test", &gloov1.RouteOptions{
					Csrf: getCsrfPolicyWithFilterEnabled(allowedOrigin),
				}).
				Build()

			testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
				vs,
			}
		})

		It("should succeed with allowed origin, unsafe request", func() {
			EventuallyAllowedOriginResponse(buildRequestFromOrigin(allowedOrigin), testContext.EnvoyInstance(), true)
		})

		It("should fail with un-allowed origin", func() {
			EventuallyInvalidOriginResponse(buildRequestFromOrigin(unAllowedOrigin), testContext.EnvoyInstance(), true)
		})

	})

	Context("enabled defined on vhost", func() {

		BeforeEach(func() {
			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					Csrf: getCsrfPolicyWithFilterEnabled(allowedOrigin),
				}).
				WithRoutePrefixMatcher("test", "/").
				WithRouteActionToUpstream("test", testContext.TestUpstream().Upstream).
				Build()

			testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
				vs,
			}
		})

		It("should succeed with allowed origin, unsafe request", func() {
			EventuallyAllowedOriginResponse(buildRequestFromOrigin(allowedOrigin), testContext.EnvoyInstance(), true)
		})

		It("should fail with un-allowed origin", func() {
			EventuallyInvalidOriginResponse(buildRequestFromOrigin(unAllowedOrigin), testContext.EnvoyInstance(), true)
		})

	})

	Context("defined on weighted dest", func() {

		BeforeEach(func() {
			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithRoutePrefixMatcher("test", "/").
				WithRouteActionToMultiDestination("test", &gloov1.MultiDestination{
					Destinations: []*gloov1.WeightedDestination{{
						Weight: &wrappers.UInt32Value{Value: 1},
						Destination: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: testContext.TestUpstream().Upstream.Metadata.Ref(),
							},
						},
						Options: &gloov1.WeightedDestinationOptions{
							Csrf: getCsrfPolicyWithFilterEnabled(allowedOrigin),
						},
					}},
				}).
				Build()

			testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
				vs,
			}
		})

		It("should succeed with allowed origin, unsafe request", func() {
			EventuallyAllowedOriginResponse(buildRequestFromOrigin(allowedOrigin), testContext.EnvoyInstance(), true)
		})

		It("should fail with un-allowed origin", func() {
			EventuallyInvalidOriginResponse(buildRequestFromOrigin(unAllowedOrigin), testContext.EnvoyInstance(), true)
		})

	})

	Context("defined on listener and vhost, should use vhost definition", func() {

		BeforeEach(func() {
			gw := gatewaydefaults.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				Csrf: getCsrfPolicyWithFilterEnabled(unAllowedOrigin),
			}

			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					Csrf: getCsrfPolicyWithFilterEnabled(allowedOrigin),
				}).
				WithRoutePrefixMatcher("test", "/").
				WithRouteActionToUpstream("test", testContext.TestUpstream().Upstream).
				Build()

			testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
				gw,
			}
			testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
				vs,
			}

		})

		It("should succeed with allowed origin, unsafe request", func() {
			EventuallyAllowedOriginResponse(buildRequestFromOrigin(allowedOrigin), testContext.EnvoyInstance(), true)
		})

		It("should fail with un-allowed origin", func() {
			EventuallyInvalidOriginResponse(buildRequestFromOrigin(unAllowedOrigin), testContext.EnvoyInstance(), true)
		})

	})

})

// A safe http method is one that doesn't alter the state of the server (ie read only)
// A CSRF attack targets state changing requests, so the filter only acts on unsafe methods (ones that change state)
// This is used to spoof requests from various origins
func buildRequestFromOrigin(origin string) func() (*http.Response, error) {
	return func() (*http.Response, error) {
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%d/", "localhost", defaults.HttpPort), nil)
		if err != nil {
			return nil, err
		}
		req.Host = e2e.DefaultHost
		req.Header.Set("Origin", origin)

		return testutils.DefaultHttpClient.Do(req)
	}
}

func EventuallyAllowedOriginResponse(request func() (*http.Response, error), envoyInstance *envoy.Instance, validateStatistics bool) {
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(request()).Should(matchers.HaveOkResponse())

		if validateStatistics {
			statistics, err := envoyInstance.Statistics()
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statistics).To(matchInvalidRequestEqualTo(0))
			g.Expect(statistics).To(matchValidRequestEqualTo(1))
		}
	}, time.Second*30).Should(Succeed())
}

func EventuallyInvalidOriginResponse(request func() (*http.Response, error), envoyInstance *envoy.Instance, validateStatistics bool) {
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(request()).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
			StatusCode: http.StatusForbidden,
			Body:       "Invalid origin",
		}))

		if validateStatistics {
			statistics, err := envoyInstance.Statistics()
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statistics).To(matchInvalidRequestEqualTo(1))
			g.Expect(statistics).To(matchValidRequestEqualTo(0))
		}
	}, time.Second*30).Should(Succeed())
}

// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/csrf_filter#statistics
func matchValidRequestEqualTo(count int) types.GomegaMatcher {
	return MatchRegexp("http.http.csrf.request_valid: %d", count)
}

func matchInvalidRequestEqualTo(count int) types.GomegaMatcher {
	return MatchRegexp("http.http.csrf.request_invalid: %d", count)
}

func getCsrfPolicyWithFilterEnabled(origin string) *csrf.CsrfPolicy {
	return &csrf.CsrfPolicy{
		FilterEnabled: &gloo_config_core.RuntimeFractionalPercent{
			DefaultValue: &glootype.FractionalPercent{
				Numerator:   uint32(100),
				Denominator: glootype.FractionalPercent_HUNDRED,
			},
		},
		AdditionalOrigins: []*gloo_type_matcher.StringMatcher{{
			MatchPattern: &gloo_type_matcher.StringMatcher_SafeRegex{
				SafeRegex: &gloo_type_matcher.RegexMatcher{
					EngineType: &gloo_type_matcher.RegexMatcher_GoogleRe2{
						GoogleRe2: &gloo_type_matcher.RegexMatcher_GoogleRE2{},
					},
					Regex: origin,
				},
			},
		}},
	}
}

func getCsrfPolicyWithShadowEnabled(origin string) *csrf.CsrfPolicy {
	return &csrf.CsrfPolicy{
		ShadowEnabled: &gloo_config_core.RuntimeFractionalPercent{
			DefaultValue: &glootype.FractionalPercent{
				Numerator:   uint32(100),
				Denominator: glootype.FractionalPercent_HUNDRED,
			},
		},
		AdditionalOrigins: []*gloo_type_matcher.StringMatcher{{
			MatchPattern: &gloo_type_matcher.StringMatcher_SafeRegex{
				SafeRegex: &gloo_type_matcher.RegexMatcher{
					EngineType: &gloo_type_matcher.RegexMatcher_GoogleRe2{
						GoogleRe2: &gloo_type_matcher.RegexMatcher_GoogleRE2{},
					},
					Regex: origin,
				},
			},
		}},
	}
}

func getCsrfPolicyWithFilterEnabledAndShadow(origin string) *csrf.CsrfPolicy {
	return &csrf.CsrfPolicy{
		FilterEnabled: &gloo_config_core.RuntimeFractionalPercent{
			DefaultValue: &glootype.FractionalPercent{
				Numerator:   uint32(100),
				Denominator: glootype.FractionalPercent_HUNDRED,
			},
		},
		ShadowEnabled: &gloo_config_core.RuntimeFractionalPercent{
			DefaultValue: &glootype.FractionalPercent{
				Numerator:   uint32(100),
				Denominator: glootype.FractionalPercent_HUNDRED,
			},
		},
		AdditionalOrigins: []*gloo_type_matcher.StringMatcher{{
			MatchPattern: &gloo_type_matcher.StringMatcher_SafeRegex{
				SafeRegex: &gloo_type_matcher.RegexMatcher{
					EngineType: &gloo_type_matcher.RegexMatcher_GoogleRe2{
						GoogleRe2: &gloo_type_matcher.RegexMatcher_GoogleRE2{},
					},
					Regex: origin,
				},
			},
		}},
	}
}
