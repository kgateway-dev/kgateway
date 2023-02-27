package kubeconverters_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v12 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("OAuth Secret Converter", func() {

	var (
		ctx            context.Context
		converter      kubesecret.SecretConverter
		resourceClient *kubesecret.ResourceClient
		glooSecret     *v1.Secret
	)

	BeforeEach(func() {
		ctx = context.TODO()
		converter = &kubeconverters.OAuthSecretConverter{}

		glooSecret = &v1.Secret{
			Metadata: &core.Metadata{
				Name:            "foo",
				Namespace:       "bar",
				OwnerReferences: []*core.Metadata_OwnerReference{},
			},
			Kind: &v1.Secret_Oauth{
				Oauth: &v12.OauthSecret{
					ClientSecret: "some-client-secret",
				},
			},
		}

		clientset := fake.NewSimpleClientset()
		coreCache, err := cache.NewKubeCoreCache(ctx, clientset)
		Expect(err).NotTo(HaveOccurred())
		resourceClient, err = kubesecret.NewResourceClient(clientset, &v1.Secret{}, false, coreCache)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("converting from a Kubernetes secret to Gloo", func() {
		var secret *corev1.Secret

		BeforeEach(func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Data: map[string][]byte{
					kubeconverters.ClientSecretDataKey: []byte("some-client-secret"),
				},
				Type: kubeconverters.OAuthSecretType,
			}
		})

		It("correctly converts oauth secrets", func() {
			actual, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(Equal(glooSecret))
		})

		It("ignores secrets that aren't oauth", func() {
			secret.Type = corev1.SecretTypeTLS

			glooSecret, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(glooSecret).To(BeNil())
		})

		It("ignores OAuth secrets that do not contain client secret", func() {
			secret.Data = map[string][]byte{
				"some-data": []byte("some-val"),
			}
			glooSecret, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(glooSecret).To(BeNil())
		})
	})

	Describe("converting from a Gloo secret to Kubernetes", func() {

		var expected = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "foo",
				Namespace:       "bar",
				OwnerReferences: []metav1.OwnerReference{},
			},
			Data: map[string][]byte{
				kubeconverters.ClientSecretDataKey: []byte("some-client-secret"),
			},
			Type: kubeconverters.OAuthSecretType,
		}

		It("ignores resources that are not secrets", func() {
			actual, err := converter.ToKubeSecret(ctx, resourceClient, &v1.Proxy{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(BeNil())
		})

		It("ignores secret that are not OAuth secrets", func() {
			actual, err := converter.ToKubeSecret(ctx, resourceClient, &v1.Secret{
				Metadata: &core.Metadata{Name: "foo"},
				Kind:     &v1.Secret_Aws{},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(BeNil())
		})

		It("correctly converts OAuth secrets", func() {
			actual, err := converter.ToKubeSecret(ctx, resourceClient, glooSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})

		It("correctly converts the old OAuth format to the new one", func() {
			glooSecret.Metadata.Annotations = map[string]string{
				kubeconverters.GlooKindAnnotationKey: "*v1.Secret",
			}

			actual, err := converter.ToKubeSecret(ctx, resourceClient, glooSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})

	})

})
