package e2e_test

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/e2e"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("GRPC to JSON Transcoding Plugin - Discovery", func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		// This test seems to work locally without linux and we don't remember why it used to require linux,
		// but if it starts failing locally, that might be the issue.
		testContext = testContextFactory.NewTestContext()
		testContext.SetUpstreamGenerator(func(ctx context.Context, addr string) *v1helpers.TestUpstream {
			return v1helpers.NewTestGRPCUpstream(ctx, addr, 1)
		})
		testContext.BeforeEach()

		testContext.SetRunServices(services.What{
			DisableGateway: false,
			DisableUds:     true,
			// test relies on FDS to discover the grpc spec via reflection
			DisableFds: false,
		})
		testContext.SetRunSettings(&gloov1.Settings{
			Gloo: &gloov1.GlooOptions{
				// https://github.com/solo-io/gloo/issues/7577
				RemoveUnusedFilters: &wrappers.BoolValue{Value: false},
			},
			Discovery: &gloov1.Settings_DiscoveryOptions{
				FdsMode: gloov1.Settings_DiscoveryOptions_BLACKLIST,
			},
		})
	})
	JustBeforeEach(func() {
		testContext.JustBeforeEach()
	})
	JustAfterEach(func() {
		testContext.JustAfterEach()
	})
	AfterEach(func() {
		testContext.AfterEach()
	})
	basicReq := func(body string, expected string) func(g Gomega) {
		return func(g Gomega) {
			req := testContext.GetHttpRequestBuilder().WithPostBody(body).WithContentType("application/json").WithPath("test")
			g.Expect(testutils.DefaultHttpClient.Do(req.Build())).Should(testmatchers.HaveExactResponseBody(expected))
		}
	}
	Context("New API", func() {
		It("Routes to GRPC Functions", func() {

			body := `"foo"`
			testRequest := basicReq(body, `{"str":"foo"}`)

			Eventually(testRequest, 30, 1).Should(Succeed())

			Eventually(testContext.TestUpstream().C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
				"GRPCRequest": PointTo(MatchFields(IgnoreExtras, Fields{"Str": Equal("foo")})),
			}))))
		})
		//basically `matchIncomingRequestRoute` needs to be set for this to work
		It("Routes to GRPC functions with prefix matcher in VS", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().Routes[0].Matchers = []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/test",
					},
				}}
				return vs
			})
			body := `"foo"`
			testRequest := basicReq(body, `{"str":"foo"}`)

			Eventually(testRequest, 30, 1).Should(Succeed())

			Eventually(testContext.TestUpstream().C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
				"GRPCRequest": PointTo(MatchFields(IgnoreExtras, Fields{"Str": Equal("foo")})),
			}))))
		})
		It("Routes to GRPC Functions with parameters in URL", func() {

			testRequest := func(g Gomega) {
				// GET request with parameters in URL
				req := testContext.GetHttpRequestBuilder().WithPath("t/foo").Build()
				g.Expect(testutils.DefaultHttpClient.Do(req)).Should(testmatchers.HaveExactResponseBody(`{"str":"foo"}`))
			}
			Eventually(testRequest, 30, 1).Should(Succeed())
			Eventually(testContext.TestUpstream().C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
				"GRPCRequest": PointTo(MatchFields(IgnoreExtras, Fields{"Str": Equal("foo")})),
			}))))
		})
	})
	Context("Deprecated API", func() {
		BeforeEach(func() {
			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{getGrpcVs(e2e.WriteNamespace, testContext.TestUpstream().Upstream.GetMetadata().Ref())}
			testContext.ResourcesToCreate().Upstreams = gloov1.UpstreamList{populateDeprecatedApi(testContext.TestUpstream().Upstream).(*gloov1.Upstream)}
		})
		It("Does not overwrite existing upstreams with the deprecated API", func() {

			body := `{"str":"foo"}`

			testRequest := basicReq(body, body)

			Eventually(testRequest, 30, 1).Should(Succeed())

			Eventually(testContext.TestUpstream().C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
				"GRPCRequest": PointTo(MatchFields(IgnoreExtras, Fields{"Str": Equal("foo")})),
			}))))
		})
	})
	Context("mismatched APIs", func() {
		BeforeEach(func() {
			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{getGrpcVs(e2e.WriteNamespace, testContext.TestUpstream().Upstream.GetMetadata().Ref())}
		})
		// The test vs we generate already has a prefix matcher because that was how this API was documented
		It("Routes to GRPC Functions", func() {

			body := `"foo"`
			testRequest := basicReq(body, `{"str":"foo"}`)

			Eventually(testRequest, 30, 1).Should(Succeed())

			Eventually(testContext.TestUpstream().C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
				"GRPCRequest": PointTo(MatchFields(IgnoreExtras, Fields{"Str": Equal("foo")})),
			}))))
		})
		It("Routes to GRPC Functions with parameters in URL", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().Routes = []*gatewayv1.Route{
					{
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/t",
							},
						}},
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: testContext.TestUpstream().Upstream.GetMetadata().Ref(),
										},
										DestinationSpec: &gloov1.DestinationSpec{
											DestinationType: &gloov1.DestinationSpec_Grpc{
												Grpc: &grpc.DestinationSpec{
													Package:  "glootest",
													Function: "TestParameterMethod",
													Service:  "TestService",
												},
											},
										},
									},
								},
							},
						}},
				}
				return vs
			})

			testRequest := func(g Gomega) {
				// GET request with parameters in URL
				req := testContext.GetHttpRequestBuilder().WithPath("t/foo").Build()
				g.Expect(testutils.DefaultHttpClient.Do(req)).Should(testmatchers.HaveExactResponseBody(`{"str":"foo"}`))
			}
			Eventually(testRequest, 30, 1).Should(Succeed())
			Eventually(testContext.TestUpstream().C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
				"GRPCRequest": PointTo(MatchFields(IgnoreExtras, Fields{"Str": Equal("foo")})),
			}))))
		})
	})
})
