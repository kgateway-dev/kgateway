package e2e_test

import (
	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/gomega/matchers"

	defaults2 "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	"net/http"

	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/dynamic_forward_proxy"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("dynamic forward proxy", func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext(
			testutils.LinuxOnly("Relies on using an in-memory pipe to ourselves"),
		)

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

	Context("without transformation", func() {

		BeforeEach(func() {
			gw := defaults2.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				DynamicForwardProxy: &dynamic_forward_proxy.FilterConfig{}, // pick up system defaults to resolve DNS
			}

			vs := helpers.NewVirtualServiceBuilder().
				WithName(e2e.DefaultVirtualServiceName).
				WithNamespace(writeNamespace).
				WithDomain(e2e.DefaultHost).
				WithRoutePrefixMatcher(e2e.DefaultRouteName, "/").
				WithRouteAction(e2e.DefaultRouteName, &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_DynamicForwardProxy{
						DynamicForwardProxy: &dynamic_forward_proxy.PerRouteConfig{
							HostRewriteSpecifier: &dynamic_forward_proxy.PerRouteConfig_AutoHostRewriteHeader{
								AutoHostRewriteHeader: "x-rewrite-me",
							},
						},
					},
				}).
				Build()

			resourceToCreate := testContext.ResourcesToCreate()
			resourceToCreate.Gateways = gatewayv1.GatewayList{
				gw,
			}
			resourceToCreate.VirtualServices = gatewayv1.VirtualServiceList{
				vs,
			}
		})

		// simpler e2e test without transformation to validate basic behavior
		It("should proxy http if dynamic forward proxy header provided on request", func() {
			requestBuilder := testContext.GetHttpRequestBuilder().
				WithPath("get").
				WithHeader("x-rewrite-me", "postman-echo.com")

			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       ContainSubstring(`"host": "postman-echo.com"`),
				}))
			}, "10s", ".1s").Should(Succeed())
		})
	})

	Context("with transformation can set dynamic forward proxy header to rewrite authority", func() {

		BeforeEach(func() {
			gw := defaults2.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				DynamicForwardProxy: &dynamic_forward_proxy.FilterConfig{}, // pick up system defaults to resolve DNS
			}
			vs := helpers.NewVirtualServiceBuilder().
				WithName(e2e.DefaultVirtualServiceName).
				WithNamespace(writeNamespace).
				WithDomain(e2e.DefaultHost).
				WithRoutePrefixMatcher(e2e.DefaultRouteName, "/").
				WithRouteAction(e2e.DefaultRouteName, &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_DynamicForwardProxy{
						DynamicForwardProxy: &dynamic_forward_proxy.PerRouteConfig{
							HostRewriteSpecifier: &dynamic_forward_proxy.PerRouteConfig_AutoHostRewriteHeader{AutoHostRewriteHeader: "x-rewrite-me"},
						},
					},
				}).
				WithRouteOptions(e2e.DefaultRouteName, &gloov1.RouteOptions{
					StagedTransformations: &transformation.TransformationStages{
						Early: &transformation.RequestResponseTransformations{
							RequestTransforms: []*transformation.RequestMatch{{
								RequestTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											Headers: map[string]*transformation.InjaTemplate{
												"x-rewrite-me": {Text: "postman-echo.com"},
											},
										},
									},
								},
							}},
						},
					},
				}).
				Build()

			resourceToCreate := testContext.ResourcesToCreate()
			resourceToCreate.Gateways = gatewayv1.GatewayList{
				gw,
			}
			resourceToCreate.VirtualServices = gatewayv1.VirtualServiceList{
				vs,
			}
		})

		// This is an important test since the most common use case here will be to grab information from the
		// request using a transformation and use that to determine the upstream destination to route to
		It("should proxy http", func() {
			requestBuilder := testContext.GetHttpRequestBuilder().WithPath("get")

			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       ContainSubstring(`"host": "postman-echo.com"`),
				}))
			}, "10s", ".1s").Should(Succeed())
		})
	})

})
