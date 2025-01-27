package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	envoytrace_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/trace/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tracing"
	"github.com/solo-io/gloo/test/e2e"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services/envoy"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/gloo/test/v1helpers"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	coreV1 "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("HeaderManipulation", func() {

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

	Context("Secrets in HeaderManipulation", func() {
		BeforeEach(func() {
			// put a secret in `writeNamespace` so we have it in the snapshot
			// The upstream is in `default` so when we enforce that secrets + upstream namespaces match, it should not be allowed
			forbiddenSecret := &gloov1.Secret{
				Kind: &gloov1.Secret_Header{
					Header: &gloov1.HeaderSecret{
						Headers: map[string]string{
							"Authorization": "basic dXNlcjpwYXNzd29yZA==",
						},
					},
				},
				Metadata: &coreV1.Metadata{
					Name:      "foo",
					Namespace: writeNamespace,
				},
			}
			// Create a secret in the same namespace as the upstream
			allowedSecret := &gloov1.Secret{
				Kind: &gloov1.Secret_Header{
					Header: &gloov1.HeaderSecret{
						Headers: map[string]string{
							"Authorization": "basic dXNlcjpwYXNzd29yZA==",
						},
					},
				},
				Metadata: &coreV1.Metadata{
					Name:      "goodsecret",
					Namespace: testContext.TestUpstream().Upstream.GetMetadata().GetNamespace(),
				},
			}
			headerManipVsBuilder := helpers.NewVirtualServiceBuilder().
				WithNamespace(writeNamespace).
				WithRoutePrefixMatcher(e2e.DefaultRouteName, "/endpoint").
				WithRouteActionToUpstream(e2e.DefaultRouteName, testContext.TestUpstream().Upstream)

			goodVS := headerManipVsBuilder.Clone().
				WithName("good").
				WithDomain("custom-domain.com").
				WithVirtualHostOptions(&gloov1.VirtualHostOptions{HeaderManipulation: &headers.HeaderManipulation{
					RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{HeaderOption: &envoycore_sk.HeaderValueOption_HeaderSecretRef{HeaderSecretRef: allowedSecret.GetMetadata().Ref()},
						Append: &wrappers.BoolValue{Value: true}}},
				}}).
				Build()
			badVS := headerManipVsBuilder.Clone().
				WithName("bad").
				WithDomain("another-domain.com").
				WithVirtualHostOptions(
					&gloov1.VirtualHostOptions{HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{HeaderOption: &envoycore_sk.HeaderValueOption_HeaderSecretRef{HeaderSecretRef: forbiddenSecret.GetMetadata().Ref()},
							Append: &wrappers.BoolValue{Value: true}}},
					}}).
				Build()

			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{goodVS, badVS}
			testContext.ResourcesToCreate().Secrets = gloov1.SecretList{forbiddenSecret, allowedSecret}
		})

		AfterEach(func() {
			os.Unsetenv(api_conversion.MatchingNamespaceEnv)
		})

		Context("With matching not enforced", func() {

			BeforeEach(func() {
				os.Setenv(api_conversion.MatchingNamespaceEnv, "false")
			})

			It("Accepts all virtual services", func() {
				helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					vs, err := testContext.TestClients().VirtualServiceClient.Read(writeNamespace, "bad", clients.ReadOpts{})
					return vs, err
				})
				helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					return testContext.TestClients().VirtualServiceClient.Read(writeNamespace, "good", clients.ReadOpts{})
				})
			})

		})
		Context("With matching enforced", func() {

			BeforeEach(func() {
				os.Setenv(api_conversion.MatchingNamespaceEnv, "true")
			})

			It("rejects the virtual service where the secret is in another namespace and accepts virtual service with a matching namespace", func() {
				helpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
					return testContext.TestClients().VirtualServiceClient.Read(writeNamespace, "bad", clients.ReadOpts{})
				})
				helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
					return testContext.TestClients().VirtualServiceClient.Read(writeNamespace, "good", clients.ReadOpts{})
				})
			})

		})
	})

	Context("Validates forbidden headers", func() {
		var headerManipVsBuilder *helpers.VirtualServiceBuilder

		BeforeEach(func() {
			headerManipVsBuilder = helpers.NewVirtualServiceBuilder().
				WithNamespace(writeNamespace).
				WithRoutePrefixMatcher(e2e.DefaultRouteName, "/endpoint").
				WithRouteActionToUpstream(e2e.DefaultRouteName, testContext.TestUpstream().Upstream)

			allowedHeaderManipulationVS := headerManipVsBuilder.Clone().
				WithName("allowed-header-manipulation").
				WithDomain("another-domain.com").
				WithVirtualHostOptions(
					&gloov1.VirtualHostOptions{HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{
							{HeaderOption: &envoycore_sk.HeaderValueOption_Header{
								Header: &envoycore_sk.HeaderValue{Key: "some-header", Value: "some-value"}},
								Append: &wrappers.BoolValue{Value: true}}},
					}}).
				Build()

			forbiddenHeaderManipulationVS := headerManipVsBuilder.Clone().
				WithName("forbidden-header-manipulation").
				WithDomain("yet-another-domain.com").
				WithVirtualHostOptions(
					&gloov1.VirtualHostOptions{HeaderManipulation: &headers.HeaderManipulation{
						RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{
							{HeaderOption: &envoycore_sk.HeaderValueOption_Header{
								Header: &envoycore_sk.HeaderValue{Key: ":path", Value: "some-value"}},
								Append: &wrappers.BoolValue{Value: true}}},
					}}).
				Build()

			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{allowedHeaderManipulationVS, forbiddenHeaderManipulationVS}
		})

		It("Allows non forbidden headers", func() {
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				vs, err := testContext.TestClients().VirtualServiceClient.Read(writeNamespace, "allowed-header-manipulation", clients.ReadOpts{})
				return vs, err
			})
		})

		It("Does not allow forbidden headers", func() {
			helpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
				vs, err := testContext.TestClients().VirtualServiceClient.Read(writeNamespace, "forbidden-header-manipulation", clients.ReadOpts{})
				return vs, err
			})
		})
	})

	Context("Header mutation order (most_specific_header_mutations_wins)", func() {
		const (
			responseHeader = "Response-Header"
			requestHeader  = "Request-Header"
		)

		rtWithOptions := func(shouldAppend bool) *v1.RouteTable {
			rtName := "append"
			if !shouldAppend {
				rtName = "overwrite"
			}

			opts := &gloov1.RouteOptions{
				HeaderManipulation: &headers.HeaderManipulation{
					RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{
						HeaderOption: &envoycore_sk.HeaderValueOption_Header{
							Header: &envoycore_sk.HeaderValue{Key: requestHeader, Value: "route-header"},
						},
						Append: &wrappers.BoolValue{Value: shouldAppend},
					}},
					ResponseHeadersToAdd: []*headers.HeaderValueOption{{
						Header: &headers.HeaderValue{Key: responseHeader, Value: "route-header"},
						Append: &wrappers.BoolValue{Value: shouldAppend},
					}},
				},
			}
			route := helpers.NewRouteBuilder().
				WithName(fmt.Sprintf("%s-route", rtName)).
				WithPrefixMatcher(fmt.Sprintf("/%s", rtName)).
				WithRouteOptions(opts).
				WithRouteActionToUpstreamRef(testContext.TestUpstream().Upstream.GetMetadata().Ref()).
				Build()

			return helpers.NewRouteTableBuilder().
				WithName(fmt.Sprintf("%s-rt", rtName)).
				WithNamespace(writeNamespace).
				WithRoute("route", route).
				Build()
		}

		vsWithOptions := func(shouldAppend bool, rtRef []*coreV1.ResourceRef) *v1.VirtualService {
			vsName := "append"
			if !shouldAppend {
				vsName = "overwrite"
			}

			opts := &gloov1.VirtualHostOptions{HeaderManipulation: &headers.HeaderManipulation{
				RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{
					HeaderOption: &envoycore_sk.HeaderValueOption_Header{
						Header: &envoycore_sk.HeaderValue{Key: requestHeader, Value: "vs-header"},
					},
					Append: &wrappers.BoolValue{Value: shouldAppend},
				}},
				ResponseHeadersToAdd: []*headers.HeaderValueOption{{
					Header: &headers.HeaderValue{Key: responseHeader, Value: "vs-header"},
					Append: &wrappers.BoolValue{Value: shouldAppend},
				}},
			}}
			vsBuilder := helpers.NewVirtualServiceBuilder().
				WithName(fmt.Sprintf("%s-vs", vsName)).
				WithNamespace(writeNamespace).
				WithDomain(fmt.Sprintf("%s.com", vsName)).
				WithVirtualHostOptions(opts)

			for _, ref := range rtRef {
				routeRef := ref
				vsBuilder.WithRouteDelegateActionRef(fmt.Sprintf("%s-route", ref.GetName()), routeRef)
			}

			return vsBuilder.Build()
		}

		// headerCheck checks that the request and response headers are as expected for a given host and path.
		headerCheck := func(appendVs, appendRoute bool, expectedReqHeaders, expectedResHeaders []string) {
			// The `append.com` host is configured to append headers, while the `overwrite.com` domain is configured to overwrite headers.
			host := "append.com"
			if !appendVs {
				host = "overwrite.com"
			}

			// The `append` path/route is configured to append headers, while the `overwrite` route is configured to overwrite headers.
			path := "append"
			if !appendRoute {
				path = "overwrite"
			}

			Eventually(func(g Gomega) {
				requestBuilder := testContext.GetHttpRequestBuilder().WithHost(host).WithPath(path)
				res, err := testutils.DefaultHttpClient.Do(requestBuilder.Build())
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
					StatusCode: http.StatusOK,
					Custom:     testmatchers.ConsistOfHeaders(http.Header{responseHeader: expectedResHeaders}),
				}))

				select {
				case req := <-testContext.TestUpstream().C:
					g.Expect(req.Headers).To(HaveKeyWithValue(requestHeader, ConsistOf(expectedReqHeaders)))
				case <-time.After(time.Second * 5):
					Fail("request didn't make it upstream")
				}
			}, "5s", "0.5s").Should(Succeed())
		}

		BeforeEach(func() {
			appendRT := rtWithOptions(true)
			overwriteRT := rtWithOptions(false)

			overwriteVS := vsWithOptions(false, []*coreV1.ResourceRef{appendRT.Metadata.Ref(), overwriteRT.Metadata.Ref()})
			appendVS := vsWithOptions(true, []*coreV1.ResourceRef{appendRT.Metadata.Ref(), overwriteRT.Metadata.Ref()})

			testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{overwriteVS, appendVS}
			testContext.ResourcesToCreate().RouteTables = v1.RouteTableList{overwriteRT, appendRT}
		})

		When("most_specific_header_mutations_wins is nil", func() {
			DescribeTable("appends headers correctly", headerCheck,
				Entry("appends route and vhost level headers", true, true, []string{"route-header", "vs-header"}, []string{"vs-header", "route-header"}),
				Entry("appends route and vhost level headers when route is set to overwrite headers", true, false, []string{"route-header", "vs-header"}, []string{"route-header", "vs-header"}),
				Entry("vhost level header overwrites route level header", false, true, []string{"vs-header"}, []string{"vs-header"}),
				Entry("vhost level header overwrites route level header when route is set to overwrite headers", false, false, []string{"vs-header"}, []string{"vs-header"}),
			)
		})

		When("most_specific_header_mutations_wins is false", func() {
			BeforeEach(func() {
				gw := defaults.DefaultGateway(writeNamespace)
				gw.RouteOptions = &gloov1.RouteConfigurationOptions{
					MostSpecificHeaderMutationsWins: &wrappers.BoolValue{Value: false},
				}
				testContext.ResourcesToCreate().Gateways = v1.GatewayList{gw}
			})

			DescribeTable("appends headers correctly", headerCheck,
				Entry("appends route and vhost level headers", true, true, []string{"route-header", "vs-header"}, []string{"route-header", "vs-header"}),
				Entry("appends route and vhost level headers when route is set to overwrite headers", true, false, []string{"route-header", "vs-header"}, []string{"route-header", "vs-header"}),
				Entry("vhost level header overwrites route level header", false, true, []string{"vs-header"}, []string{"vs-header"}),
				Entry("vhost level header overwrites route level header when route is set to overwrite headers", false, false, []string{"vs-header"}, []string{"vs-header"}),
			)
		})

		When("most_specific_header_mutations_wins is true", func() {
			BeforeEach(func() {
				gw := defaults.DefaultGateway(writeNamespace)
				gw.RouteOptions = &gloov1.RouteConfigurationOptions{
					MostSpecificHeaderMutationsWins: &wrappers.BoolValue{Value: true},
				}
				testContext.ResourcesToCreate().Gateways = v1.GatewayList{gw}
			})

			DescribeTable("appends headers correctly", headerCheck,
				Entry("appends route and vhost level headers", true, true, []string{"route-header", "vs-header"}, []string{"route-header", "vs-header"}),
				Entry("appends route and vhost level headers when vhost is set to overwrite headers", false, true, []string{"route-header", "vs-header"}, []string{"route-header", "vs-header"}),
				Entry("route level header overwrites vhost level header", true, false, []string{"route-header"}, []string{"route-header"}),
				Entry("route level header overwrites vhost level header when vhost is set to overwrite headers", false, false, []string{"route-header"}, []string{"route-header"}),
			)
		})
	})

	Describe("Early Header Manipulation", func() {
		var (
			httpClient     *http.Client
			requestBuilder *testutils.HttpRequestBuilder
		)

		type HeadersResponse struct {
			Headers map[string][]string `json:"headers"`
		}

		prepareEHMTestContext := func(testContext *e2e.TestContext, ehm *headers.EarlyHeaderManipulation) {
			getHCMSettings(testContext).EarlyHeaderManipulation = ehm
		}

		makeHeadersRequest := func(inHeaders map[string]string) (*HeadersResponse, error) {
			req := requestBuilder.
				WithHeader("Accept", "application/json").
				WithPath("headers").
				WithHeaders(inHeaders).
				Build()

			res, err := httpClient.Do(req)
			if err != nil {
				return &HeadersResponse{}, err
			}

			if res.StatusCode != http.StatusOK {
				return &HeadersResponse{}, fmt.Errorf("unexpected status code %d", res.StatusCode)
			}

			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			if err != nil {
				return &HeadersResponse{}, err
			}

			var headerResponse HeadersResponse
			err = json.Unmarshal(body, &headerResponse)
			if err != nil {
				return &HeadersResponse{}, err
			}

			return &headerResponse, nil
		}

		Context("No mutations", func() {
			BeforeEach(func() {
				testContext = testContextFactory.NewTestContext()
				testContext.SetUpstreamGenerator(v1helpers.NewTestHttpUpstreamWithHttpbin)
				testContext.BeforeEach()

				prepareEHMTestContext(testContext, nil)

				httpClient = testutils.DefaultClientBuilder().Build()
				requestBuilder = testContext.GetHttpRequestBuilder()
			})

			It("Should do nothing if no mutations are present", func() {
				Eventually(func(g Gomega) {
					headersResponse, err := makeHeadersRequest(map[string]string{
						"X-Keep": "foo",
						"X-Drop": "bar",
					})
					g.Expect(err).NotTo(HaveOccurred())

					// the headers should be unchanged
					g.Expect(headersResponse.Headers).To(HaveKeyWithValue("X-Keep", ConsistOf("foo")),
						"Expected header X-Keep to be present")
					g.Expect(headersResponse.Headers).To(HaveKeyWithValue("X-Drop", ConsistOf("bar")),
						"Expected header X-Drop to be present when no mutations are present")
					g.Expect(headersResponse.Headers).NotTo(HaveKey("X-Add"),
						"Expected header X-Add to not be present when no mutations are present")
				}, "5s", "0.5s").Should(Succeed())
			})
		})

		Context("Add/remove manipulations", func() {
			BeforeEach(func() {
				testContext = testContextFactory.NewTestContext()
				testContext.SetUpstreamGenerator(v1helpers.NewTestHttpUpstreamWithHttpbin)
				testContext.BeforeEach()

				prepareEHMTestContext(testContext, &headers.EarlyHeaderManipulation{
					HeadersToAdd: []*envoycore_sk.HeaderValueOption{
						{
							HeaderOption: &envoycore_sk.HeaderValueOption_Header{
								Header: &envoycore_sk.HeaderValue{
									Key:   "X-Add",
									Value: "baz",
								},
							},
							Append: &wrappers.BoolValue{Value: true},
						},
						{
							HeaderOption: &envoycore_sk.HeaderValueOption_Header{
								Header: &envoycore_sk.HeaderValue{
									Key:   "X-Append",
									Value: "baz",
								},
							},
							Append: &wrappers.BoolValue{Value: true},
						},
						{
							HeaderOption: &envoycore_sk.HeaderValueOption_Header{
								Header: &envoycore_sk.HeaderValue{
									Key:   "X-Overwrite",
									Value: "baz",
								},
							},
							Append: &wrappers.BoolValue{Value: false},
						},
					},
					HeadersToRemove: []string{"X-Drop"},
				})

				httpClient = testutils.DefaultClientBuilder().Build()
				requestBuilder = testContext.GetHttpRequestBuilder()
			})

			It("Should append as expected", func() {
				Eventually(func(g Gomega) {
					headersResponse, err := makeHeadersRequest(map[string]string{
						"X-Append":    "foo",
						"X-Overwrite": "bar",
						"X-Drop":      "baz",
					})
					g.Expect(err).NotTo(HaveOccurred())

					// the headers should be as expected
					g.Expect(headersResponse.Headers).To(HaveKeyWithValue("X-Add", ConsistOf("baz")),
						"Expected header X-Add to be present")
					g.Expect(headersResponse.Headers).To(HaveKeyWithValue("X-Append", ConsistOf("foo", "baz")),
						"Expected header X-Add to be present")
					g.Expect(headersResponse.Headers).To(HaveKeyWithValue("X-Overwrite", ConsistOf("baz")),
						"Expected header X-Exists two have two values")
					g.Expect(headersResponse.Headers).NotTo(HaveKey("X-Drop"),
						"Expected header X-Drop to be removed")
				}, "5s", "0.5s").Should(Succeed())
			})
		})

		Context("Manipulation with secrets", func() {
			BeforeEach(func() {
				testContext = testContextFactory.NewTestContext()
				testContext.SetUpstreamGenerator(v1helpers.NewTestHttpUpstreamWithHttpbin)
				testContext.BeforeEach()

				prepareEHMTestContext(testContext, &headers.EarlyHeaderManipulation{
					HeadersToAdd: []*envoycore_sk.HeaderValueOption{
						{
							HeaderOption: &envoycore_sk.HeaderValueOption_HeaderSecretRef{
								HeaderSecretRef: &coreV1.ResourceRef{
									Name:      "secret",
									Namespace: writeNamespace,
								},
							},
							Append: &wrappers.BoolValue{Value: true},
						},
					},
				})

				secret := &gloov1.Secret{
					Kind: &gloov1.Secret_Header{
						Header: &gloov1.HeaderSecret{
							Headers: map[string]string{
								"X-Secret": "something super secret",
							},
						},
					},
					Metadata: &coreV1.Metadata{
						Name:      "secret",
						Namespace: writeNamespace,
					},
				}

				testContext.ResourcesToCreate().Secrets = gloov1.SecretList{secret}

				httpClient = testutils.DefaultClientBuilder().Build()
				requestBuilder = testContext.GetHttpRequestBuilder()
			})

			It("Should have the secret", func() {
				Eventually(func(g Gomega) {
					headersResponse, err := makeHeadersRequest(map[string]string{
						"X-Keep": "foo",
						"X-Drop": "bar",
					})
					g.Expect(err).NotTo(HaveOccurred())

					// the headers should be as expected
					g.Expect(headersResponse.Headers).To(HaveKeyWithValue("X-Secret",
						ConsistOf("something super secret")),
						"Expected header X-Secret to be present with secret value")
				}, "5s", "0.5s").Should(Succeed())
			})
		})

		// A customer reported that normal header manipulation was happening after the tracing headers were added.
		// They desired that they be able to override the tracing headers with their own headers.
		// Setting the tracing headers earlier allows for the override to happen.
		// This test ensures that the interaction between the two works as expected.
		Context("Interaction with Zipkin tracing", func() {
			var (
				zipkinInstance *envoy.Instance
				//zipkinUpstream *v1helpers.TestUpstream
			)

			prepareZipkinTracingTestContext := func(testContext *e2e.TestContext, upstream *gloov1.Upstream) {
				getHCMSettings(testContext).Tracing = &tracing.ListenerTracingSettings{
					ProviderConfig: &tracing.ListenerTracingSettings_ZipkinConfig{
						ZipkinConfig: &envoytrace_gloo.ZipkinConfig{
							CollectorCluster: &envoytrace_gloo.ZipkinConfig_CollectorUpstreamRef{
								CollectorUpstreamRef: &core.ResourceRef{
									Name:      upstream.GetMetadata().GetName(),
									Namespace: writeNamespace,
								},
							},
							CollectorEndpoint:        zipkinCollectionPath,
							CollectorEndpointVersion: envoytrace_gloo.ZipkinConfig_HTTP_JSON,
						},
					},
				}
			}

			startCancellableTracingServer := func(serverContext context.Context, address string) {
				// Start a dummy server listening on 9411 for tracing requests
				tracingCollectorHandler := http.NewServeMux()
				tracingCollectorHandler.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal(zipkinCollectionPath))
					fmt.Fprintf(w, "Dummy tracing Collector received request on - %q", html.EscapeString(r.URL.Path))
				}))

				tracingServer := &http.Server{
					Addr:    address,
					Handler: tracingCollectorHandler,
				}

				// Start a goroutine to handle requests
				go func() {
					defer GinkgoRecover()
					if err := tracingServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						ExpectWithOffset(1, err).NotTo(HaveOccurred())
					}
				}()

				// Start a goroutine to shutdown the server
				go func(serverCtx context.Context) {
					defer GinkgoRecover()

					<-serverCtx.Done()
					// tracingServer.Shutdown hangs with opentelemetry tests, probably
					// because the agent leaves the connection open. There's no need for a
					// graceful shutdown anyway, so just force it using Close() instead
					tracingServer.Close()
				}(serverContext)
			}

			BeforeEach(func() {
				testContext = testContextFactory.NewTestContext()
				testContext.SetUpstreamGenerator(v1helpers.NewTestHttpUpstreamWithHttpbin)
				testContext.BeforeEach()

				httpClient = testutils.DefaultClientBuilder().Build()
				requestBuilder = testContext.GetHttpRequestBuilder()

				// create Zipkin tracing collector
				// the tracing extension expects the zipkin collector to be on port 9411
				// which makes this whole process a bit more complicated
				zipkinInstance = envoyFactory.NewInstance()
				startCancellableTracingServer(testContext.Ctx(),
					fmt.Sprintf("%s:%d", zipkinInstance.LocalAddr(), tracingCollectorPort))

				// create tracing collector upstream
				zipkinUpstream := &gloov1.Upstream{
					Metadata: &core.Metadata{
						Name:      tracingCollectorUpstreamName,
						Namespace: writeNamespace,
					},
					UpstreamType: &gloov1.Upstream_Static{
						Static: &static_plugin_gloo.UpstreamSpec{
							Hosts: []*static_plugin_gloo.Host{
								{
									Addr: zipkinInstance.LocalAddr(),
									Port: tracingCollectorPort,
								},
							},
						},
					},
				}

				testContext.ResourcesToCreate().Upstreams = gloov1.UpstreamList{
					testContext.TestUpstream().Upstream,
					zipkinUpstream,
				}

				// add the zipkin tracing configuration to the test context
				prepareZipkinTracingTestContext(testContext, zipkinUpstream)
			})

			Context("Zipkin/B3 headers without transforms", func() {
				BeforeEach(func() {
					prepareEHMTestContext(testContext, nil)
				})

				It("Zipkin/B3 headers should make without transforms", func() {
					Eventually(func(g Gomega) {
						headersResponse, err := makeHeadersRequest(map[string]string{})
						g.Expect(err).NotTo(HaveOccurred())

						// the headers should be as expected
						g.Expect(headersResponse.Headers).To(HaveKey("X-B3-Traceid"),
							"Expected header X-B3-Traceid to be present")
						g.Expect(headersResponse.Headers).To(HaveKey("X-B3-Spanid"),
							"Expected header X-B3-Spanid to be present")
					}, "5s", "0.5s").Should(Succeed())
				})
			})

			Context("Zipkin/B3 headers with transforms", func() {
				BeforeEach(func() {
					prepareEHMTestContext(testContext, &headers.EarlyHeaderManipulation{
						HeadersToAdd: []*envoycore_sk.HeaderValueOption{
							{
								HeaderOption: &envoycore_sk.HeaderValueOption_Header{
									Header: &envoycore_sk.HeaderValue{
										Key:   "X-B3-Traceid",
										Value: "%REQ(x-override-traceid)%",
									},
								},
							},
							{
								HeaderOption: &envoycore_sk.HeaderValueOption_Header{
									Header: &envoycore_sk.HeaderValue{
										Key:   "X-B3-Spanid",
										Value: "%REQ(x-override-spanid)%",
									},
								},
							},
						},
					})
				})

				It("Should be able to override Zipkin/B3 headers", func() {
					Eventually(func(g Gomega) {
						headersResponse, err := makeHeadersRequest(map[string]string{
							"x-override-traceid": "traceid",
							"x-override-spanid":  "spanid",
						})
						g.Expect(err).NotTo(HaveOccurred())

						// the headers should be as expected
						g.Expect(headersResponse.Headers).To(HaveKeyWithValue("X-B3-Traceid", ConsistOf("traceid")),
							"Expected header X-B3-Traceid to be present with overridden value")
						g.Expect(headersResponse.Headers).To(HaveKeyWithValue("X-B3-Spanid", ConsistOf("spanid")),
							"Expected header X-B3-Spanid to be present with overridden value")
					}, "5s", "0.5s").Should(Succeed())
				})
			})
		})
	})
})

func getHCMSettings(testContext *e2e.TestContext) *hcm.HttpConnectionManagerSettings {
	gateway := testContext.ResourcesToCreate().Gateways[0]
	Expect(gateway).NotTo(BeNil())
	httpGateway := gateway.GetHttpGateway()
	Expect(httpGateway).NotTo(BeNil())

	listenerOptions := httpGateway.GetOptions()
	if listenerOptions == nil {
		listenerOptions = &gloov1.HttpListenerOptions{}
		httpGateway.Options = listenerOptions
	}

	hcmSettings := listenerOptions.GetHttpConnectionManagerSettings()
	if hcmSettings == nil {
		hcmSettings = &hcm.HttpConnectionManagerSettings{}
		listenerOptions.HttpConnectionManagerSettings = hcmSettings
	}

	return hcmSettings
}
