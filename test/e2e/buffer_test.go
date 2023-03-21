package e2e_test

import (
	"net/http"
	"time"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	buffer "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("buffer", func() {

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

	Context("filter defined on listener", func() {

		Context("Large buffer ", func() {

			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					Buffer: &buffer.Buffer{
						MaxRequestBytes: &wrappers.UInt32Value{
							Value: 4098, // max size
						},
					},
				}

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gw,
				}
			})

			It("valid buffer size should succeed", func() {
				requestBuilder := testContext.GetHttpRequestBuilder().
					WithPostBody(`{"value":"test"}`).
					WithContentType("application/json")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveExactResponseBody("{\"value\":\"test\"}"))
				}, 10*time.Second, 1*time.Second).Should(Succeed())
			})

		})

		Context("Small buffer ", func() {

			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					Buffer: &buffer.Buffer{
						MaxRequestBytes: &wrappers.UInt32Value{
							Value: 1,
						},
					},
				}

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gw,
				}
			})

			It("empty buffer should fail", func() {
				requestBuilder := testContext.GetHttpRequestBuilder().
					WithPostBody(`{"value":"test"}`).
					WithContentType("application/json")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
						Body:       "Payload Too Large",
						StatusCode: http.StatusRequestEntityTooLarge,
					}))
				}, 10*time.Second, 1*time.Second).Should(Succeed())
			})
		})
	})

	Context("filter defined on listener and vhost", func() {

		Context("Large buffer ", func() {
			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					Buffer: &buffer.Buffer{
						MaxRequestBytes: &wrappers.UInt32Value{
							Value: 1,
						},
					},
				}
				vsToTestUpstream := helpers.NewVirtualServiceBuilder().
					WithName("vs-test").
					WithNamespace(writeNamespace).
					WithDomain("test.com").
					WithVirtualHostOptions(&gloov1.VirtualHostOptions{
						BufferPerRoute: &buffer.BufferPerRoute{
							Override: &buffer.BufferPerRoute_Buffer{
								Buffer: &buffer.Buffer{
									MaxRequestBytes: &wrappers.UInt32Value{
										Value: 4098, // max size
									},
								},
							},
						},
					}).
					WithRoutePrefixMatcher("test", "/").
					WithRouteActionToUpstream("test", testContext.TestUpstream().Upstream).
					Build()

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gw,
				}
				testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
					vsToTestUpstream,
				}
			})

			It("valid buffer size should succeed", func() {
				requestBuilder := testContext.GetHttpRequestBuilder().
					WithPostBody(`{"value":"test"}`).
					WithContentType("application/json")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveExactResponseBody("{\"value\":\"test\"}"))
				}, 10*time.Second, 1*time.Second).Should(Succeed())
			})

		})

		Context("Small buffer ", func() {

			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					Buffer: &buffer.Buffer{
						MaxRequestBytes: &wrappers.UInt32Value{
							Value: 4098,
						},
					},
				}
				vsToTestUpstream := helpers.NewVirtualServiceBuilder().
					WithName("vs-test").
					WithNamespace(writeNamespace).
					WithDomain("test.com").
					WithVirtualHostOptions(&gloov1.VirtualHostOptions{
						BufferPerRoute: &buffer.BufferPerRoute{
							Override: &buffer.BufferPerRoute_Buffer{
								Buffer: &buffer.Buffer{
									MaxRequestBytes: &wrappers.UInt32Value{
										Value: 1,
									},
								},
							},
						},
					}).
					WithRoutePrefixMatcher("test", "/").
					WithRouteActionToUpstream("test", testContext.TestUpstream().Upstream).
					Build()

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gw,
				}
				testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
					vsToTestUpstream,
				}
			})

			It("empty buffer should fail", func() {
				requestBuilder := testContext.GetHttpRequestBuilder().
					WithPostBody(`{"value":"test"}`).
					WithContentType("application/json")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
						Body:       "Payload Too Large",
						StatusCode: http.StatusRequestEntityTooLarge,
					}))
				}, 10*time.Second, 1*time.Second).Should(Succeed())
			})
		})
	})

	Context("filter defined on listener and route", func() {

		Context("Large buffer ", func() {

			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					Buffer: &buffer.Buffer{
						MaxRequestBytes: &wrappers.UInt32Value{
							Value: 1,
						},
					},
				}
				vsToTestUpstream := helpers.NewVirtualServiceBuilder().
					WithName("vs-test").
					WithNamespace(writeNamespace).
					WithDomain("test.com").
					WithRoutePrefixMatcher("test", "/").
					WithRouteActionToUpstream("test", testContext.TestUpstream().Upstream).
					WithRouteOptions("test", &gloov1.RouteOptions{
						BufferPerRoute: &buffer.BufferPerRoute{
							Override: &buffer.BufferPerRoute_Buffer{
								Buffer: &buffer.Buffer{
									MaxRequestBytes: &wrappers.UInt32Value{
										Value: 4098, // max size
									},
								},
							},
						},
					}).
					Build()

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gw,
				}
				testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
					vsToTestUpstream,
				}
			})

			It("valid buffer size should succeed", func() {
				requestBuilder := testContext.GetHttpRequestBuilder().
					WithPostBody(`{"value":"test"}`).
					WithContentType("application/json")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveExactResponseBody("{\"value\":\"test\"}"))
				}, 10*time.Second, 1*time.Second).Should(Succeed())
			})

		})

		Context("Small buffer ", func() {

			BeforeEach(func() {
				gw := gatewaydefaults.DefaultGateway(writeNamespace)
				gw.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
					Buffer: &buffer.Buffer{
						MaxRequestBytes: &wrappers.UInt32Value{
							Value: 4098,
						},
					},
				}
				vsToTestUpstream := helpers.NewVirtualServiceBuilder().
					WithName("vs-test").
					WithNamespace(writeNamespace).
					WithDomain("test.com").
					WithRoutePrefixMatcher("test", "/").
					WithRouteActionToUpstream("test", testContext.TestUpstream().Upstream).
					WithRouteOptions("test", &gloov1.RouteOptions{
						BufferPerRoute: &buffer.BufferPerRoute{
							Override: &buffer.BufferPerRoute_Buffer{
								Buffer: &buffer.Buffer{
									MaxRequestBytes: &wrappers.UInt32Value{
										Value: 1,
									},
								},
							},
						},
					}).
					Build()

				testContext.ResourcesToCreate().Gateways = gatewayv1.GatewayList{
					gw,
				}
				testContext.ResourcesToCreate().VirtualServices = gatewayv1.VirtualServiceList{
					vsToTestUpstream,
				}
			})

			It("empty buffer should fail", func() {
				requestBuilder := testContext.GetHttpRequestBuilder().
					WithPostBody(`{"value":"test"}`).
					WithContentType("application/json")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
						Body:       "Payload Too Large",
						StatusCode: http.StatusRequestEntityTooLarge,
					}))
				}, 10*time.Second, 1*time.Second).Should(Succeed())
			})
		})
	})

})
