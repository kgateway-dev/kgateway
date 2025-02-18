//go:build ignore

package e2e_test

import (
	"encoding/base64"
	"encoding/json"

	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/test/testutils"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	v1 "github.com/kgateway-dev/kgateway/v2/internal/gateway/pkg/api/v1"
	extauthv1 "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	gloov1static "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1/options/static"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"

	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"

	gloov1 "github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1"
	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1/core/matchers"
	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1/options/transformation"
)

var _ = Describe("Staged Transformation", FlakeAttempts(3), func() {
	// We added the FlakeAttempts decorator to try to reduce the impact of the flakes outlined in:
	// https://github.com/kgateway-dev/kgateway/issues/9292

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
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											BodyTransformation: &transformation.TransformationTemplate_Body{
												Body: &transformation.InjaTemplate{
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
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											BodyTransformation: &transformation.TransformationTemplate_Body{
												Body: &transformation.InjaTemplate{
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
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											Headers: map[string]*transformation.InjaTemplate{
												"x-custom-header": {Text: "original header"},
											},
											HeadersToAppend: []*transformation.TransformationTemplate_HeaderToAppend{
												{
													Key:   "x-custom-header",
													Value: &transformation.InjaTemplate{Text: "{{upper(\"appended header 1\")}}"},
												},
												{
													Key:   "x-custom-header",
													Value: &transformation.InjaTemplate{Text: "{{upper(\"appended header 2\")}}"},
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
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											BodyTransformation: &transformation.TransformationTemplate_Body{
												Body: &transformation.InjaTemplate{
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
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											BodyTransformation: &transformation.TransformationTemplate_Body{
												Body: &transformation.InjaTemplate{
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
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											BodyTransformation: &transformation.TransformationTemplate_Body{
												Body: &transformation.InjaTemplate{
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

		It("Can add endpoint metadata to headers using postRouting transformation", func() {
			testContext.PatchDefaultUpstream(func(u *gloov1.Upstream) *gloov1.Upstream {
				static := u.GetStatic()
				if static == nil {
					return u
				}
				// Set a metadata key in the transformation namespace
				static.Hosts[0].Metadata = map[string]*structpb.Struct{
					"io.solo.transformation": {
						Fields: map[string]*structpb.Value{
							"key": {
								Kind: &structpb.Value_StringValue{
									StringValue: "value",
								},
							},
						},
					},
				}
				return u
			})
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						PostRouting: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											BodyTransformation: &transformation.TransformationTemplate_Body{
												Body: &transformation.InjaTemplate{Text: "{{host_metadata(\"key\")}}"},
											},
											HeadersToAppend: []*transformation.TransformationTemplate_HeaderToAppend{
												{
													Key:   "x-custom-header",
													Value: &transformation.InjaTemplate{Text: "{{host_metadata(\"key\")}}"},
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

			// send a request, expect that:
			// 1. The body will contain the metadata value
			// 2. The header `x-custom-header` will contain the metadata value

			requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody("123456789")
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       "value",
					Headers: map[string]interface{}{
						"x-custom-header": "value",
					},
				}))
			}, "15s", ".5s").Should(Succeed())
		})

		It("Can modify specific body keys using MergeJsonKeys", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithVirtualHostOptions(&gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						PostRouting: &transformation.RequestResponseTransformations{
							ResponseTransforms: []*transformation.ResponseMatch{{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &transformation.TransformationTemplate{
											BodyTransformation: &transformation.TransformationTemplate_MergeJsonKeys{
												MergeJsonKeys: &transformation.MergeJsonKeys{
													JsonKeys: map[string]*transformation.MergeJsonKeys_OverridableTemplate{
														"key2": {
															Tmpl: &transformation.InjaTemplate{Text: "\"new value\""},
														},
														"key3": {
															Tmpl: &transformation.InjaTemplate{Text: "\"value3\""},
														},
													},
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

			// send a request, expect that:
			// 1. The body will contain the metadata value
			// 2. The header `x-custom-header` will contain the metadata value
			jsnBody := map[string]any{
				"key1": "value1",
				"key2": "value2",
			}
			bdyByt, err := json.Marshal(jsnBody)
			Expect(err).NotTo(HaveOccurred())

			requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(string(bdyByt))
			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body:       "{\"key1\":\"value1\",\"key2\":\"new value\",\"key3\":\"value3\"}",
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
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											Headers: map[string]*transformation.InjaTemplate{
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

		Context("Extractors", func() {
			var (
				extraction             *transformation.Extraction
				transformationTemplate *transformation.TransformationTemplate
				vHostOpts              *gloov1.VirtualHostOptions
			)

			BeforeEach(func() {
				extraction = &transformation.Extraction{
					Source: &transformation.Extraction_Body{},
					Regex:  ".*",
				}

				transformationTemplate = &transformation.TransformationTemplate{
					ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
					BodyTransformation: &transformation.TransformationTemplate_Body{
						Body: &transformation.InjaTemplate{
							Text: "{{ foo }}",
						},
					},
					Extractors: map[string]*transformation.Extraction{"foo": extraction},
				}

				vHostOpts = &gloov1.VirtualHostOptions{
					StagedTransformations: &transformation.TransformationStages{
						Regular: &transformation.RequestResponseTransformations{
							RequestTransforms: []*transformation.RequestMatch{{
								RequestTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: transformationTemplate,
									},
								},
							}},
						},
					},
				}
			})

			Describe("Extract mode", func() {
				It("Can extract text from a subset of the input", func() {
					extraction.Mode = transformation.Extraction_EXTRACT
					extraction.Regex = ".*(test).*"
					extraction.Subgroup = 1

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					body := "this is a test"
					requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(body)
					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
							StatusCode: http.StatusOK,
							Body:       "test",
						}))
					}, "5s", ".5s").Should(Succeed())
				})

				It("Defaults to extract mode", func() {
					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					body := "test"
					requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(body)
					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
							StatusCode: http.StatusOK,
							Body:       body,
						}))
					}, "5s", ".5s").Should(Succeed())
				})

				It("Doesn't extract if regex doesn't match", func() {
					extraction.Mode = transformation.Extraction_EXTRACT
					extraction.Regex = "will not match"
					extraction.Subgroup = 0

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					body := "this is a test"
					requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(body)
					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
							StatusCode: http.StatusOK,
							Body:       "",
						}))
					}, "5s", ".5s").Should(Succeed())
				})

				It("Doesn't extract if regex doesn't match entire input", func() {
					extraction.Mode = transformation.Extraction_EXTRACT
					extraction.Regex = "is a test"
					extraction.Subgroup = 0

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					body := "this is a test"
					requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(body)
					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
							StatusCode: http.StatusOK,
							Body:       "",
						}))
					}, "5s", ".5s").Should(Succeed())
				})

				It("Rejects config if replacement_text is set", func() {
					extraction.Mode = transformation.Extraction_EXTRACT
					extraction.ReplacementText = &wrapperspb.StringValue{Value: "test"}

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					helpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
						vs, err := testContext.TestClients().VirtualServiceClient.Read(writeNamespace, e2e.DefaultVirtualServiceName, clients.ReadOpts{})
						return vs, err
					})
				})

			})
			Describe("Single Replace mode", func() {
				It("Can extract a substring from the body and replace it in the response", func() {
					extraction.Mode = transformation.Extraction_SINGLE_REPLACE
					extraction.Regex = ".*(test).*"
					extraction.Subgroup = 1
					extraction.ReplacementText = &wrapperspb.StringValue{Value: "replaced"}

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					body := "this is a test"
					requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(body)
					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
							StatusCode: http.StatusOK,
							Body:       "this is a replaced",
						}))
					}, "5s", ".5s").Should(Succeed())
				})

				It("Can replace from subgroup 0 when regex matches entire body", func() {
					extraction.Mode = transformation.Extraction_SINGLE_REPLACE
					extraction.Regex = "this is a (test)"
					extraction.Subgroup = 0
					extraction.ReplacementText = &wrapperspb.StringValue{Value: "replaced"}

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					body := "this is a test"
					requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(body)
					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
							StatusCode: http.StatusOK,
							Body:       "replaced",
						}))
					}, "5s", ".5s").Should(Succeed())
				})

				It("Returns input if regex doesn't match", func() {
					extraction.Mode = transformation.Extraction_SINGLE_REPLACE
					extraction.Regex = "will not match"
					extraction.Subgroup = 0
					extraction.ReplacementText = &wrapperspb.StringValue{Value: "replaced"}

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)

						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					body := "this is a test"
					requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(body)
					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
							StatusCode: http.StatusOK,
							Body:       body,
						}))
					}, "5s", ".5s").Should(Succeed())
				})

				It("Returns input if regex doesn't match entire input", func() {
					extraction.Mode = transformation.Extraction_SINGLE_REPLACE
					extraction.Regex = "is a test"
					extraction.Subgroup = 0
					extraction.ReplacementText = &wrapperspb.StringValue{Value: "replaced"}

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)

						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					body := "this is a test"
					requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(body)
					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
							StatusCode: http.StatusOK,
							Body:       body,
						}))
					}, "5s", ".5s").Should(Succeed())
				})

				It("Rejects config if replacement_text is not set", func() {
					extraction.Mode = transformation.Extraction_SINGLE_REPLACE
					extraction.Regex = ".*(test).*"
					extraction.Subgroup = 1

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					helpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
						vs, err := testContext.TestClients().VirtualServiceClient.Read(writeNamespace, e2e.DefaultVirtualServiceName, clients.ReadOpts{})
						return vs, err
					})
				})
			})
			Describe("Replace ALL mode", func() {
				It("Can replace multiple instances of the regex in the body", func() {
					extraction.Mode = transformation.Extraction_REPLACE_ALL
					extraction.Regex = "test"
					extraction.ReplacementText = &wrapperspb.StringValue{Value: "replaced"}

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					body := "test test test"
					requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(body)
					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
							StatusCode: http.StatusOK,
							Body:       "replaced replaced replaced",
						}))
					}, "5s", ".5s").Should(Succeed())
				})

				It("Returns input if regex doesn't match", func() {
					extraction.Mode = transformation.Extraction_REPLACE_ALL
					extraction.Regex = "will not match"
					extraction.ReplacementText = &wrapperspb.StringValue{Value: "replaced"}

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)

						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					body := "test test test"
					requestBuilder := testContext.GetHttpRequestBuilder().WithPostBody(body)
					Eventually(func(g Gomega) {
						g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
							StatusCode: http.StatusOK,
							Body:       body,
						}))
					}, "5s", ".5s").Should(Succeed())
				})

				It("Rejects config if replacement_text is not set", func() {
					extraction.Mode = transformation.Extraction_REPLACE_ALL
					extraction.Regex = "test"

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					helpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
						vs, err := testContext.TestClients().VirtualServiceClient.Read(writeNamespace, e2e.DefaultVirtualServiceName, clients.ReadOpts{})
						return vs, err
					})
				})

				It("Rejects config if subgroup is set", func() {
					extraction.Mode = transformation.Extraction_REPLACE_ALL
					extraction.Regex = "test"
					extraction.Subgroup = 1
					extraction.ReplacementText = &wrapperspb.StringValue{Value: "replaced"}

					testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
						vsBuilder := helpers.BuilderFromVirtualService(vs)
						vsBuilder.WithVirtualHostOptions(vHostOpts)
						return vsBuilder.Build()
					})

					helpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
						vs, err := testContext.TestClients().VirtualServiceClient.Read(writeNamespace, e2e.DefaultVirtualServiceName, clients.ReadOpts{})
						return vs, err
					})
				})
			})
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
											TransformationTemplate: &transformation.TransformationTemplate{
												ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
												BodyTransformation: &transformation.TransformationTemplate_Body{
													Body: &transformation.InjaTemplate{
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
											TransformationTemplate: &transformation.TransformationTemplate{
												ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
												BodyTransformation: &transformation.TransformationTemplate_Body{
													Body: &transformation.InjaTemplate{
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
												TransformationTemplate: &transformation.TransformationTemplate{
													ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
													BodyTransformation: &transformation.TransformationTemplate_Body{
														Body: &transformation.InjaTemplate{
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
												TransformationTemplate: &transformation.TransformationTemplate{
													ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
													BodyTransformation: &transformation.TransformationTemplate_Body{
														Body: &transformation.InjaTemplate{
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
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											Headers: map[string]*transformation.InjaTemplate{
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
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											Headers: map[string]*transformation.InjaTemplate{
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
										TransformationTemplate: &transformation.TransformationTemplate{
											ParseBodyBehavior: transformation.TransformationTemplate_DontParse,
											BodyTransformation: &transformation.TransformationTemplate_Body{
												Body: &transformation.InjaTemplate{
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
