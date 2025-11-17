package gwapiutils_test

import (
	"context"
	"fmt"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	testing2 "k8s.io/client-go/testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/gwapiutils"
)

func TestGwApiUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GwApiUtils Suite")
}

// setupCRDReactor adds a reactor to return the given CRD when requested
func setupCRDReactor(fakeClient *fakeapiextensions.FakeApiextensionsV1, crd *apiextensionsv1.CustomResourceDefinition) {
	fakeClient.Fake.AddReactor("get", "customresourcedefinitions", func(action testing2.Action) (handled bool, ret runtime.Object, err error) {
		getAction := action.(testing2.GetAction)
		if getAction.GetName() == "gateways.gateway.networking.k8s.io" {
			if crd != nil {
				return true, crd, nil
			}
			return true, nil, fmt.Errorf("customresourcedefinition.apiextensions.k8s.io %q not found", getAction.GetName())
		}
		return false, nil, fmt.Errorf("not found")
	})
}

var _ = Describe("DetectGatewayAPIVersionWithClient", func() {
	var (
		ctx        context.Context
		crdClient  apiextensionsv1client.CustomResourceDefinitionInterface
		fakeClient *fakeapiextensions.FakeApiextensionsV1
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = &fakeapiextensions.FakeApiextensionsV1{Fake: &testing2.Fake{}}
		crdClient = fakeClient.CustomResourceDefinitions()
	})

	Context("with valid Gateway CRD", func() {
		It("should detect standard channel and version", func() {
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateways.gateway.networking.k8s.io",
					Annotations: map[string]string{
						"gateway.networking.k8s.io/channel":        "standard",
						"gateway.networking.k8s.io/bundle-version": "v1.2.0",
					},
				},
			}
			setupCRDReactor(fakeClient, crd)

			info, err := gwapiutils.DetectGatewayAPIVersionWithClient(ctx, crdClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.Channel).To(Equal(gwapiutils.GwApiChannelStandard))
			Expect(info.Channel.IsStandard()).To(BeTrue())
			Expect(info.Channel.IsExperimental()).To(BeFalse())
			Expect(info.Version.String()).To(Equal("1.2.0"))
		})

		It("should detect experimental channel and version", func() {
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateways.gateway.networking.k8s.io",
					Annotations: map[string]string{
						"gateway.networking.k8s.io/channel":        "experimental",
						"gateway.networking.k8s.io/bundle-version": "v1.4.0",
					},
				},
			}
			setupCRDReactor(fakeClient, crd)

			info, err := gwapiutils.DetectGatewayAPIVersionWithClient(ctx, crdClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.Channel).To(Equal(gwapiutils.GwApiChannelExperimental))
			Expect(info.Channel.IsExperimental()).To(BeTrue())
			Expect(info.Channel.IsStandard()).To(BeFalse())
			Expect(info.Version.String()).To(Equal("1.4.0"))
		})
	})

	Context("with missing Gateway CRD", func() {
		It("should return an error", func() {
			setupCRDReactor(fakeClient, nil)

			info, err := gwapiutils.DetectGatewayAPIVersionWithClient(ctx, crdClient)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get Gateway CRD"))
			Expect(info).To(BeNil())
		})
	})

	Context("with missing channel annotation", func() {
		It("should return an error", func() {
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateways.gateway.networking.k8s.io",
					Annotations: map[string]string{
						"gateway.networking.k8s.io/bundle-version": "v1.2.0",
					},
				},
			}
			setupCRDReactor(fakeClient, crd)

			info, err := gwapiutils.DetectGatewayAPIVersionWithClient(ctx, crdClient)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing 'gateway.networking.k8s.io/channel' annotation"))
			Expect(info).To(BeNil())
		})
	})

	Context("with missing version annotation", func() {
		It("should return an error", func() {
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateways.gateway.networking.k8s.io",
					Annotations: map[string]string{
						"gateway.networking.k8s.io/channel": "standard",
					},
				},
			}
			setupCRDReactor(fakeClient, crd)

			info, err := gwapiutils.DetectGatewayAPIVersionWithClient(ctx, crdClient)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing 'gateway.networking.k8s.io/bundle-version' annotation"))
			Expect(info).To(BeNil())
		})
	})

	Context("with invalid version string", func() {
		It("should return an error", func() {
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateways.gateway.networking.k8s.io",
					Annotations: map[string]string{
						"gateway.networking.k8s.io/channel":        "standard",
						"gateway.networking.k8s.io/bundle-version": "not-a-version",
					},
				},
			}
			setupCRDReactor(fakeClient, crd)

			info, err := gwapiutils.DetectGatewayAPIVersionWithClient(ctx, crdClient)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse Gateway API version"))
			Expect(info).To(BeNil())
		})
	})
})
