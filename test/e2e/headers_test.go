package e2e_test

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/test/e2e"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/testutils"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
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

		rtWithOptions := func(append bool) *v1.RouteTable {
			rtName := "append"
			if !append {
				rtName = "overwrite"
			}

			opts := &gloov1.RouteOptions{
				HeaderManipulation: &headers.HeaderManipulation{
					RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{
						HeaderOption: &envoycore_sk.HeaderValueOption_Header{
							Header: &envoycore_sk.HeaderValue{Key: requestHeader, Value: "route-header"},
						},
						Append: &wrappers.BoolValue{Value: append},
					}},
					ResponseHeadersToAdd: []*headers.HeaderValueOption{{
						Header: &headers.HeaderValue{Key: responseHeader, Value: "route-header"},
						Append: &wrappers.BoolValue{Value: append},
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

		vsWithOptions := func(append bool, rtRef []*coreV1.ResourceRef) *v1.VirtualService {
			vsName := "append"
			if !append {
				vsName = "overwrite"
			}

			opts := &gloov1.VirtualHostOptions{HeaderManipulation: &headers.HeaderManipulation{
				RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{
					HeaderOption: &envoycore_sk.HeaderValueOption_Header{
						Header: &envoycore_sk.HeaderValue{Key: requestHeader, Value: "vs-header"},
					},
					Append: &wrappers.BoolValue{Value: append},
				}},
				ResponseHeadersToAdd: []*headers.HeaderValueOption{{
					Header: &headers.HeaderValue{Key: responseHeader, Value: "vs-header"},
					Append: &wrappers.BoolValue{Value: append},
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
})
