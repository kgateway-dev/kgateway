package e2e_test

import (
	"encoding/base64"

	"github.com/solo-io/gloo/test/testutils"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/golang/protobuf/ptypes/wrappers"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	gloov1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/e2e"

	envoytransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
)

var _ = Describe("Staged Transformation", Ordered, func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContext()
		testContext.BeforeEach()

		// This test relies on running the gateway-proxy with debug logging enabled
		testContext.EnvoyInstance().LogLevel = zapcore.DebugLevel.String()
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

	Context("no auth", func() {

		It("should transform response", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						Early: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								Matchers: []*matchers.HeaderMatcher{
									{
										Name:  ":status",
										Value: "200",
									},
								},
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											BodyTransformation: &envoytransformation.TransformationTemplate_Body{
												Body: &envoytransformation.InjaTemplate{
													Text: "early-transformed",
												},
											},
										},
									},
								},
							}},
						},
						// add regular response to see that the early one overrides it
						Regular: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								Matchers: []*matchers.HeaderMatcher{
									{
										Name:  ":status",
										Value: "200",
									},
								},
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											BodyTransformation: &envoytransformation.TransformationTemplate_Body{
												Body: &envoytransformation.InjaTemplate{
													Text: "regular-transformed",
												},
											},
										},
									},
								},
							}},
						},
					},
				})
				return vsBuilder.Build()
			})

			requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody("test")
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       "early-transformed",
				}))
			}, "15s", ".5s").Should(Succeed())
		})

		It("should allow multiple header values for the same header when using HeadersToAppend", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						Regular: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											Headers: map[string]*envoytransformation.InjaTemplate{
												"x-custom-header": {Text: "original header"},
											},
											HeadersToAppend: []*envoytransformation.TransformationTemplate_HeaderToAppend{
												{
													Key:   "x-custom-header",
													Value: &envoytransformation.InjaTemplate{Text: "{{upper(\"appended header 1\")}}"},
												},
												{
													Key:   "x-custom-header",
													Value: &envoytransformation.InjaTemplate{Text: "{{upper(\"appended header 2\")}}"},
												},
											},
										},
									},
								},
							}},
						},
					},
				})
				return vsBuilder.Build()
			})

			requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody("")
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       BeEmpty(),
					// The default Header matcher only works with single headers, so we supply a custom matcher in this case
					Custom: testmatchers.ContainHeaders(http.Header{
						"X-Custom-Header": []string{"original header", "APPENDED HEADER 1", "APPENDED HEADER 2"},
					}),
				}))
			}, "15s", ".5s").Should(Succeed())
		})

		It("Should be able to base64 encode the body", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						Regular: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											BodyTransformation: &envoytransformation.TransformationTemplate_Body{
												Body: &envoytransformation.InjaTemplate{
													Text: "{{base64_encode(body())}}",
												},
											},
										},
									},
								},
							}},
						},
					},
				})
				return vsBuilder.Build()
			})

			// send a request, expect that the response body is base64 encoded
			requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody("test")
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       base64.StdEncoding.EncodeToString([]byte("test")),
				}))
			}, "15s", ".5s").Should(Succeed())
		})

		It("Should be able to base64 decode the body", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						Regular: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											BodyTransformation: &envoytransformation.TransformationTemplate_Body{
												Body: &envoytransformation.InjaTemplate{
													Text: "{{base64_decode(body())}}",
												},
											},
										},
									},
								},
							}},
						},
					},
				})
				return vsBuilder.Build()
			})

			// send a request, expect that the response body is base64 decoded
			body := "test"
			encodedBody := base64.StdEncoding.EncodeToString([]byte(body))
			requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(encodedBody)
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       body,
				}))
			}, "15s", ".5s").Should(Succeed())
		})

		It("Can extract a substring from the body", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						Regular: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											BodyTransformation: &envoytransformation.TransformationTemplate_Body{
												Body: &envoytransformation.InjaTemplate{
													Text: "{{substring(body(), 0, 4)}}",
												},
											},
										},
									},
								},
							}},
						},
					},
				})
				return vsBuilder.Build()
			})

			// send a request, expect that the response body contains only the first 4 characters

			requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody("123456789")
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       "1234",
				}))
			}, "15s", ".5s").Should(Succeed())
		})

		It("Can base64 decode and transform headers", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						Regular: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											Headers: map[string]*envoytransformation.InjaTemplate{
												// decode the x-custom-header header and then extract a substring
												"x-new-custom-header": {Text: `{{substring(base64_decode(request_header("x-custom-header")), 6, 5)}}`},
											},
										},
									},
								},
							}},
						},
					},
				})
				return vsBuilder.Build()
			})

			requestBuilder := testContext.GetHttpRequestBuilder().
				WithPostBody("").
				WithHeader("x-custom-header", base64.StdEncoding.EncodeToString([]byte("test1.test2")))
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponseWithHeaders(map[string]interface{}{
					"X-New-Custom-Header": ContainSubstring("test2"),
				}))
			}, "15s", ".5s").Should(Succeed())
		})

		// helper function for the "can enable enhanced logging" table test
		// this function checks that both a regular stage and early stage response transformation
		// generate the expected logs
		containsAllEnhancedLoggingSubstrings := func(logs string) {
			Expect(logs).To(And(
				ContainSubstring(`body before transformation: test`),
				ContainSubstring(`body after transformation: regular-transformed`),
				ContainSubstring(`body before transformation: regular-transformed`),
				ContainSubstring(`body after transformation: regular-transformed-early-transformed`),
			))
		}

		// helper function for the "can enable enhanced logging" table test
		// this function checks that an early stage transformation and not a regular stage transformation
		// generate the expected logs
		containsEarlyEnhancedLoggingSubstrings := func(logs string) {
			Expect(logs).To(And(
				Not(ContainSubstring(`body before transformation: test`)),
				// we need to escape the newline here because we do expect to see regular-transformed-early-transformed
				Not(ContainSubstring(`body after transformation: regular-transformed\n`)),
				ContainSubstring(`body before transformation: regular-transformed`),
				ContainSubstring(`body after transformation: regular-transformed-early-transformed`),
			))
		}

		// helper function for the "can enable enhanced logging" table test
		// this function checks that neither a regular stage transformation nor an early stage transformation
		// generate enhanced logs
		containsNoEnhancedLoggingSubstrings := func(logs string) {
			Expect(logs).To(And(
				Not(ContainSubstring(`body before transformation: test`)),
				Not(ContainSubstring(`body after transformation: regular-transformed`)),
				Not(ContainSubstring(`body before transformation: regular-transformed`)),
				Not(ContainSubstring(`body after transformation: regular-transformed-early-transformed`)),
			))
		}

		DescribeTable("can enable enhanced logging", func(logRequestResponseInfoStaged bool, logRequestResponseInfoIndividual bool, expectedLogSubstrings func(string)) {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						LogRequestResponseInfo: &wrapperspb.BoolValue{Value: logRequestResponseInfoStaged},
						Early: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{
								{
									Matchers: []*matchers.HeaderMatcher{
										{
											Name:  ":status",
											Value: "200",
										},
									},
									ResponseTransformation: &transformation.Transformation{
										LogRequestResponseInfo: logRequestResponseInfoIndividual,
										TransformationType: &transformation.Transformation_TransformationTemplate{
											TransformationTemplate: &envoytransformation.TransformationTemplate{
												ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
												BodyTransformation: &envoytransformation.TransformationTemplate_Body{
													Body: &envoytransformation.InjaTemplate{
														Text: "{{body()}}-early-transformed",
													},
												},
											},
										},
									},
								}},
						},
						Regular: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{
								{
									Matchers: []*matchers.HeaderMatcher{
										{
											Name:  ":status",
											Value: "200",
										},
									},
									ResponseTransformation: &transformation.Transformation{
										TransformationType: &transformation.Transformation_TransformationTemplate{
											TransformationTemplate: &envoytransformation.TransformationTemplate{
												ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
												BodyTransformation: &envoytransformation.TransformationTemplate_Body{
													Body: &envoytransformation.InjaTemplate{
														Text: "regular-transformed",
													},
												},
											},
										},
									},
								}},
						},
					},
				})
				return vsBuilder.Build()
			})

			requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody("test")
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       "regular-transformed-early-transformed",
				}))
			}, "15s", ".5s").Should(Succeed())

			// get the logs from the gateway-proxy container
			logs, err := testContext.EnvoyInstance().Logs()
			Expect(err).NotTo(HaveOccurred())

			expectedLogSubstrings(logs)
		},
			Entry("staged logging enabled, individual logging enabled", true, true, containsAllEnhancedLoggingSubstrings),
			Entry("staged logging enabled, individual logging disabled", true, false, containsAllEnhancedLoggingSubstrings),
			Entry("staged logging disabled, individual logging enabled", false, true, containsEarlyEnhancedLoggingSubstrings),
			Entry("staged logging disabled, individual logging disabled", false, false, containsNoEnhancedLoggingSubstrings),
		)

		Context("Enhanced logging in global settings", func() {
			BeforeEach(func() {
				testContext.SetRunSettings(&gloov1.Settings{
					Gloo: &gloov1.GlooOptions{
						LogTransformationRequestResponseInfo: &wrapperspb.BoolValue{Value: true},
					},
				})
			})

			It("can enable enhanced logging from global settings object", func() {
				testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
					vsBuilder := helpers.BuilderFromVirtualService(vs)
					vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
						StagedTransformations: &transformation.TransformationStages{
							Early: &transformation.RequestResponseTransformations{
								ResponseTransforms: []*transformation.ResponseMatch{
									{
										Matchers: []*matchers.HeaderMatcher{
											{
												Name:  ":status",
												Value: "200",
											},
										},
										ResponseTransformation: &transformation.Transformation{
											TransformationType: &transformation.Transformation_TransformationTemplate{
												TransformationTemplate: &envoytransformation.TransformationTemplate{
													ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
													BodyTransformation: &envoytransformation.TransformationTemplate_Body{
														Body: &envoytransformation.InjaTemplate{
															Text: "{{body()}}-early-transformed",
														},
													},
												},
											},
										},
									}},
							},
							Regular: &transformation.RequestResponseTransformations{
								ResponseTransforms: []*transformation.ResponseMatch{
									{
										Matchers: []*matchers.HeaderMatcher{
											{
												Name:  ":status",
												Value: "200",
											},
										},
										ResponseTransformation: &transformation.Transformation{
											TransformationType: &transformation.Transformation_TransformationTemplate{
												TransformationTemplate: &envoytransformation.TransformationTemplate{
													ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
													BodyTransformation: &envoytransformation.TransformationTemplate_Body{
														Body: &envoytransformation.InjaTemplate{
															Text: "regular-transformed",
														},
													},
												},
											},
										},
									}},
							},
						},
					})
					return vsBuilder.Build()
				})

				requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody("test")
				Eventually(func(g Gomega) {
					g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
						StatusCode: http.StatusOK,
						Body:       "regular-transformed-early-transformed",
					}))
				}, "15s", ".5s").Should(Succeed())

				// get the logs from the gateway-proxy container
				logs, err := testContext.EnvoyInstance().Logs()
				Expect(err).NotTo(HaveOccurred())

				containsAllEnhancedLoggingSubstrings(logs)
			})
		})

		It("should apply transforms from most specific level only", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						Regular: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											Headers: map[string]*envoytransformation.InjaTemplate{
												"x-solo-1": {Text: "vhost header"},
											},
										},
									},
								},
							}},
						},
					},
				})
				vsBuilder.WithRouteOptions(e2e.DefaultRouteName, &gloov1.RouteOptions{
					StagedTransformations: &transformation.TransformationStages{
						Regular: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											Headers: map[string]*envoytransformation.InjaTemplate{
												"x-solo-2": {Text: "route header"},
											},
										},
									},
								},
							}},
						},
					},
				})
				return vsBuilder.Build()
			})

			requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody("")
			Eventually(func(g Gomega) {
				// Only route level transformations should be applied here due to the nature of envoy choosing
				// the most specific config (weighted cluster > route > vhost)
				// This behaviour can be overridden (in the control plane) by using `inheritableTransformations` to merge
				// transformations down to the route level.
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(testmatchers.HaveOkResponseWithHeaders(map[string]interface{}{
					"x-solo-2": Equal("route header"),
					"x-solo-1": BeEmpty(),
				}))
			}, "15s", ".5s").Should(Succeed())
		})
	})

	Context("with auth", func() {

		BeforeEach(func() {
			// this upstream doesn't need to exist - in fact, we want ext auth to fail.
			extAuthUpstream := &gloov1.Upstream{
				Metadata: &core.Metadata{
					Name:      "extauth-server",
					Namespace: "default",
				},
				UseHttp2: &wrappers.BoolValue{Value: true},
				UpstreamType: &gloov1.Upstream_Static{
					Static: &gloov1static.UpstreamSpec{
						Hosts: []*gloov1static.Host{{
							Addr: "127.2.3.4",
							Port: 1234,
						}},
					},
				},
			}

			testContext.ResourcesToCreate().Upstreams = append(testContext.ResourcesToCreate().Upstreams, extAuthUpstream)

			testContext.SetRunSettings(&gloov1.Settings{Extauth: &extauthv1.Settings{
				ExtauthzServerRef: extAuthUpstream.GetMetadata().Ref(),
			}})
		})

		It("should transform response code details", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						Early: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseCodeDetails: "ext_authz_error",
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &envoytransformation.TransformationTemplate{
											ParseBodyBehavior: envoytransformation.TransformationTemplate_DontParse,
											BodyTransformation: &envoytransformation.TransformationTemplate_Body{
												Body: &envoytransformation.InjaTemplate{
													Text: "early-transformed",
												},
											},
										},
									},
								},
							}},
						},
					},
					Extauth: &extauthv1.ExtAuthExtension{
						Spec: &extauthv1.ExtAuthExtension_CustomAuth{
							CustomAuth: &extauthv1.CustomAuth{},
						},
					},
				})
				return vsBuilder.Build()
			})

			// send a request and expect it transformed!
			requestBuilder := testContext.GetHttpRequestBuilder()
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).To(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusForbidden,
					Body:       "early-transformed",
				}))
			}, "15s", ".5s").Should(Succeed())
		})

	})

})
