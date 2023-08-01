package e2e_test

import (
	"os"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/api_conversion"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	envoycore_sk "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	coreV1 "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"
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
})
