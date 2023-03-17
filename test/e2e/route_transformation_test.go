package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/onsi/gomega/gstruct"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/e2e"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/v1helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	envoy_transform "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
)

var _ = Describe("Transformations", func() {

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

	Context("Parsing valid json", func() {

		var transform *transformation.Transformations

		BeforeEach(func() {
			transform = &transformation.Transformations{
				ResponseTransformation: &transformation.Transformation{
					TransformationType: &transformation.Transformation_TransformationTemplate{
						TransformationTemplate: &envoy_transform.TransformationTemplate{
							BodyTransformation: &envoy_transform.TransformationTemplate_Body{
								Body: &envoy_transform.InjaTemplate{
									Text: "{{body}}",
								},
							},
							Headers: map[string]*envoy_transform.InjaTemplate{
								"content-type": {
									Text: "text/html",
								},
							},
						},
					},
				},
			}
		})

		// EventuallyResponseTransformed returns an Asynchronous Assertion which
		// validates that a request with a body will return the requested content.
		// This will only work if the above transformation is applied to the request
		EventuallyResponseTransformed := func() AsyncAssertion {
			return Eventually(func(g Gomega) {
				req, err := http.NewRequest(
					http.MethodPost,
					fmt.Sprintf("http://localhost:%d/1", defaults.HttpPort),
					bytes.NewBufferString("{\"body\":\"test\"}"))
				g.Expect(err).NotTo(HaveOccurred(), "Can create request object")
				req.Host = e2e.DefaultHost

				res, err := http.DefaultClient.Do(req)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).To(testmatchers.HaveExactResponseBody("test"))
			}, "5s", ".5s")
		}

		It("should fail if no transform defined", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{
					Transformations: nil,
				}
				return vs
			})

			EventuallyResponseTransformed().Should(HaveOccurred())
		})

		It("should should transform json to html response on vhost", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{
					Transformations: transform,
				}
				return vs
			})

			EventuallyResponseTransformed().Should(Succeed())
		})

		It("should should transform json to html response on route", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().GetRoutes()[0].Options = &gloov1.RouteOptions{
					Transformations: transform,
				}
				return vs
			})

			EventuallyResponseTransformed().Should(Succeed())
		})

		It("should should transform json to html response on route", func() {
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vsBuilder := helpers.BuilderFromVirtualService(vs)
				vsBuilder.WithRouteActionToMultiDestination(e2e.DefaultRouteName, &gloov1.MultiDestination{
					Destinations: []*gloov1.WeightedDestination{{
						Weight: &wrappers.UInt32Value{Value: 1},
						Options: &gloov1.WeightedDestinationOptions{
							Transformations: transform,
						},
						Destination: &gloov1.Destination{
							DestinationType: &gloov1.Destination_Upstream{
								Upstream: testContext.TestUpstream().Upstream.GetMetadata().Ref(),
							},
						},
					}},
				})

				return vsBuilder.Build()
			})

			EventuallyResponseTransformed().Should(Succeed())

		})

	})

	Context("parsing non-valid JSON", func() {

		var transform *transformation.Transformations

		BeforeEach(func() {
			htmlResponse := "<html></html>"
			htmlEchoUpstream := v1helpers.NewTestHttpUpstreamWithReply(testContext.Ctx(), testContext.EnvoyInstance().LocalAddr(), htmlResponse)

			// This is a bit of a trick
			// We use the Default VirtualService name and then remove all VirtualServices in the ResourcesToCreate
			// This makes the vsToHtmlUpstream the "default" and tests can use PatchVirtualService to modify it
			vsToHtmlUpstream := helpers.NewVirtualServiceBuilder().
				WithName(e2e.DefaultVirtualServiceName).
				WithNamespace(writeNamespace).
				WithDomain(e2e.DefaultHost).
				WithRoutePrefixMatcher(e2e.DefaultRouteName, "/html").
				WithRouteActionToUpstream(e2e.DefaultRouteName, htmlEchoUpstream.Upstream).
				Build()

			testContext.ResourcesToCreate().Upstreams = gloov1.UpstreamList{htmlEchoUpstream.Upstream}
			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{vsToHtmlUpstream}

			transform = &transformation.Transformations{
				ResponseTransformation: &transformation.Transformation{
					TransformationType: &transformation.Transformation_TransformationTemplate{
						TransformationTemplate: &envoy_transform.TransformationTemplate{
							Headers: map[string]*envoy_transform.InjaTemplate{
								"x-solo-resp-hdr1": {
									Text: "{{ request_header(\"x-solo-hdr-1\") }}",
								},
							},
						},
					},
				},
			}
		})

		// EventuallyHtmlResponseTransformed returns an Asynchronous Assertion which
		// validates that a request with a header will return a response header with the same
		// value, and the body of the response is non-json
		// This will only work if the above transformation is applied to the request
		EventuallyHtmlResponseTransformed := func() AsyncAssertion {
			return Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/html", defaults.HttpPort), nil)
				g.Expect(err).NotTo(HaveOccurred())
				req.Host = e2e.DefaultHost
				req.Header.Set("x-solo-hdr-1", "test")

				res, err := http.DefaultClient.Do(req)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).To(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Body: WithTransform(func(b []byte) error {
						var body map[string]interface{}
						return json.Unmarshal(b, &body)
					}, HaveOccurred()), // attempt to read body as json to confirm that it was not parsed
					Headers: map[string]interface{}{
						"x-solo-resp-hdr1": Equal("test"), // inspect response headers to confirm transformation was applied
					},
				}))
			}, "5s", ".5s")
		}

		It("should error on non-json body when ignoreErrorOnParse/parseBodyBehavior/passthrough is disabled", func() {
			transform.ResponseTransformation.GetTransformationTemplate().IgnoreErrorOnParse = false
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{
					Transformations: transform,
				}
				return vs
			})

			Eventually(func(g Gomega) {
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/html", defaults.HttpPort), nil)
				g.Expect(err).NotTo(HaveOccurred())
				req.Host = e2e.DefaultHost
				req.Header.Set("x-solo-hdr-1", "test")

				res, err := http.DefaultClient.Do(req)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).To(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusBadRequest,
					Body:       gstruct.Ignore(), // We don't care about the body, which will contain an error message
				}))
			}, "5s", ".5s").Should(Succeed())
		})

		It("should transform response with non-json body when ignoreErrorOnParse is enabled", func() {
			transform.ResponseTransformation.GetTransformationTemplate().IgnoreErrorOnParse = true
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{
					Transformations: transform,
				}
				return vs
			})

			EventuallyHtmlResponseTransformed().Should(Succeed())
		})

		It("should transform response with non-json body when ParseBodyBehavior is set to DontParse", func() {
			transform.ResponseTransformation.GetTransformationTemplate().ParseBodyBehavior = envoy_transform.TransformationTemplate_DontParse
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{
					Transformations: transform,
				}
				return vs
			})

			EventuallyHtmlResponseTransformed().Should(Succeed())
		})

		It("should transform response with non-json body when passthrough is enabled", func() {
			transform.ResponseTransformation.GetTransformationTemplate().BodyTransformation = &envoy_transform.TransformationTemplate_Passthrough{
				Passthrough: &envoy_transform.Passthrough{},
			}
			testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
				vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{
					Transformations: transform,
				}
				return vs
			})

			EventuallyHtmlResponseTransformed().Should(Succeed())
		})
	})

	Context("requestTransformation", func() {
		var (
			us *gloov1.Upstream
			vh *gloov1.VirtualHost
		)

		extractJsonResponse := func(res *http.Response) map[string]interface{} {
			// read response body
			body, err := io.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())

			// parse the response body as JSON
			var bodyJson map[string]interface{}
			err = json.Unmarshal(body, &bodyJson)
			Expect(err).NotTo(HaveOccurred())
			// the response from the httpbin /anything endpoint is nested under the "json" key
			return bodyJson["json"].(map[string]interface{})

		}

		BeforeEach(func() {
			// create upstream that will return an html body at the /html endpoint
			us = getHttpbinEchoUpstream()
			writeUpstream(us)

			// create a virtual host with a route to the upstream
			vh = getTrivialVirtualHostWithUpstreamRef(us.Metadata.Ref())

			// add a transformation to the virtual host
			transform = &transformation.Transformations{
				RequestTransformation: &transformation.Transformation{
					TransformationType: &transformation.Transformation_HeaderBodyTransform{
						HeaderBodyTransform: &envoy_transform.HeaderBodyTransform{
							AddRequestMetadata: true,
						},
					},
				},
			}

			vh.Options = &gloov1.VirtualHostOptions{
				Transformations: transform,
			}
		})

		It("should handle queryStringParameters and multiValueQueryStringParameters", func() {
			writeVhost(vh)

			// execute request -- expect a 200 response
			url := fmt.Sprintf("http://%s:%d/anything?foo=bar&multiple=1&multiple=2", "localhost", envoyPort)
			headers := map[string][]string{}
			req := formRequestWithUrlAndHeaders(url, headers)
			res := getSuccessfulResponse(req)

			bodyJson := extractJsonResponse(res)

			// inspect the response body to confirm that the queryStringParameters were added to the metadata
			Expect(bodyJson["queryStringParameters"].(map[string]interface{})["foo"]).To(Equal("bar"))
			// we expect the value of a multi-value query string parameter to be the last defined
			Expect(bodyJson["queryStringParameters"].(map[string]interface{})["multiple"]).To(Equal("2"))

			// inspect the response body to confirm that the multiValueQueryStringParameters were added to the metadata
			Expect(bodyJson["multiValueQueryStringParameters"].(map[string]interface{})["multiple"].([]interface{})[0]).To(Equal("1"))
			Expect(bodyJson["multiValueQueryStringParameters"].(map[string]interface{})["multiple"].([]interface{})[1]).To(Equal("2"))
		})

		It("should handle headers and multiValueHeaders", func() {
			writeVhost(vh)

			// execute request -- expect a 200 response
			url := fmt.Sprintf("http://%s:%d/anything", "localhost", envoyPort)
			headers := map[string][]string{
				"x-solo-test-header": {"test"},
				"foo":                {"bar", "baz"},
			}
			req := formRequestWithUrlAndHeaders(url, headers)
			res := getSuccessfulResponse(req)

			bodyJson := extractJsonResponse(res)

			// inspect the response body to confirm that the headers were added to the metadata
			Expect(bodyJson["headers"].(map[string]interface{})["x-solo-test-header"]).To(Equal("test"))
			Expect(bodyJson["headers"].(map[string]interface{})["foo"]).To(Equal("baz"))

			// inspect the response body to confirm that the multiValueHeaders were added to the metadata
			Expect(bodyJson["multiValueHeaders"].(map[string]interface{})["foo"].([]interface{})[0]).To(Equal("bar"))
			Expect(bodyJson["multiValueHeaders"].(map[string]interface{})["foo"].([]interface{})[1]).To(Equal("baz"))
		})
	})
})
