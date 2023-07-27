package e2e_test

import (
	"fmt"
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

	"github.com/solo-io/gloo/test/testutils"

	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Test", Label(), func() {

	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		var testRequirements []testutils.Requirement
		testContext = testContextFactory.NewTestContext(testRequirements...)
		testContext.BeforeEach()
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
		goodVS := helpers.NewVirtualServiceBuilder().
			WithName("good").
			WithNamespace(writeNamespace).
			WithDomain("custom-domain.com").
			WithRoutePrefixMatcher(e2e.DefaultRouteName, "/endpoint").
			WithRouteActionToUpstream(e2e.DefaultRouteName, testContext.TestUpstream().Upstream).
			Build()
		goodVS.GetVirtualHost().Options = &gloov1.VirtualHostOptions{HeaderManipulation: &headers.HeaderManipulation{
			RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{HeaderOption: &envoycore_sk.HeaderValueOption_HeaderSecretRef{HeaderSecretRef: &coreV1.ResourceRef{Name: allowedSecret.GetMetadata().GetName(), Namespace: allowedSecret.GetMetadata().GetNamespace()}},
				Append: &wrappers.BoolValue{Value: true}}},
			RequestHeadersToRemove: []string{"a"},
		}}
		badVS := helpers.NewVirtualServiceBuilder().
			WithName("bad").
			WithNamespace(writeNamespace).
			WithDomain("another-domain.com").
			WithRoutePrefixMatcher(e2e.DefaultRouteName, "/endpoint").
			WithRouteActionToUpstream(e2e.DefaultRouteName, testContext.TestUpstream().Upstream).
			Build()
		badVS.GetVirtualHost().Options = &gloov1.VirtualHostOptions{HeaderManipulation: &headers.HeaderManipulation{
			RequestHeadersToAdd: []*envoycore_sk.HeaderValueOption{{HeaderOption: &envoycore_sk.HeaderValueOption_HeaderSecretRef{HeaderSecretRef: &coreV1.ResourceRef{Name: forbiddenSecret.GetMetadata().GetName(), Namespace: forbiddenSecret.GetMetadata().GetNamespace()}},
				Append: &wrappers.BoolValue{Value: true}}},
			RequestHeadersToRemove: []string{"a"},
		}}

		testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{goodVS, badVS}
		testContext.ResourcesToCreate().Secrets = gloov1.SecretList{forbiddenSecret, allowedSecret}
	})

	AfterEach(func() {
		os.Setenv(api_conversion.MatchingNamespaceEnv, "")
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("With matching not enforced", func() {

		BeforeEach(func() {
			os.Setenv(api_conversion.MatchingNamespaceEnv, "false")
		})

		It("Accepts all virtual services", func() {
			helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
				vs, err := testContext.TestClients().VirtualServiceClient.Read(writeNamespace, "bad", clients.ReadOpts{})
				fmt.Printf("status: %v", vs.GetNamespacedStatuses())
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
