package e2e_test

import (
	"fmt"

	"github.com/solo-io/gloo/test/gomega/matchers"

	defaults2 "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	"net/http"

	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/dynamic_forward_proxy"

	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var _ = Describe("dynamic forward proxy", func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext(
			helpers.LinuxOnly("Relies on using an in-memory pipe to ourselves"),
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

	eventuallyRequestMatches := func(dest string, updateReq func(r *http.Request), expectedBody interface{}) {
		By("Prepare request")
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/get", "localhost", defaults.HttpPort), nil)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		req.Host = e2e.DefaultHost
		updateReq(req)

		By("Make request")
		EventuallyWithOffset(1, func(g Gomega) {
			g.Expect(http.DefaultClient.Do(req)).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
				StatusCode: http.StatusOK,
				Body:       expectedBody,
			}))
		}, "10s", ".1s").Should(Succeed())
	}

	Context("without transformation", func() {

		BeforeEach(func() {
			gw := defaults2.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				DynamicForwardProxy: &dynamic_forward_proxy.FilterConfig{}, // pick up system defaults to resolve DNS
			}

			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithRoutePrefixMatcher("test", "/").
				WithRouteAction("test", &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_DynamicForwardProxy{
						DynamicForwardProxy: &dynamic_forward_proxy.PerRouteConfig{
							HostRewriteSpecifier: &dynamic_forward_proxy.PerRouteConfig_AutoHostRewriteHeader{AutoHostRewriteHeader: "x-rewrite-me"},
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
			destEcho := `postman-echo.com`
			eventuallyRequestMatches(destEcho, func(r *http.Request) {
				r.Header.Set("x-rewrite-me", destEcho)
			}, ContainSubstring(`"host": "postman-echo.com"`))
		})
	})

	Context("with transformation can set dynamic forward proxy header to rewrite authority", func() {

		BeforeEach(func() {
			gw := defaults2.DefaultGateway(writeNamespace)
			gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
				DynamicForwardProxy: &dynamic_forward_proxy.FilterConfig{}, // pick up system defaults to resolve DNS
			}
			vs := helpers.NewVirtualServiceBuilder().
				WithName("vs-test").
				WithNamespace(writeNamespace).
				WithDomain("test.com").
				WithRoutePrefixMatcher("test", "/").
				WithRouteAction("test", &gloov1.RouteAction{
					Destination: &gloov1.RouteAction_DynamicForwardProxy{
						DynamicForwardProxy: &dynamic_forward_proxy.PerRouteConfig{
							HostRewriteSpecifier: &dynamic_forward_proxy.PerRouteConfig_AutoHostRewriteHeader{AutoHostRewriteHeader: "x-rewrite-me"},
						},
					},
				}).
				WithRouteOptions("test", &gloov1.RouteOptions{
					StagedTransformations: &transformation.TransformationStages{
						Early: &transformation.RequestResponseTransformations{
							RequestTransforms: []*transformation.RequestMatch{{
								RequestTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											Headers: map[string]*envoytransformation.InjaTemplate{
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
			destEcho := `postman-echo.com`
			eventuallyRequestMatches(destEcho, func(r *http.Request) {
				// nothing to modify
			}, ContainSubstring(`"host": "postman-echo.com"`))
		})
	})

})
