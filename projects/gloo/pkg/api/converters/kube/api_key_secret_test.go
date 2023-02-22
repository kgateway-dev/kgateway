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

var _ = Describe("API Key Secret Converter", func() {

	var (
		ctx            context.Context
		converter      kubesecret.SecretConverter
		resourceClient *kubesecret.ResourceClient
		glooSecret     *v1.Secret
	)

	BeforeEach(func() {
		ctx = context.TODO()
		converter = &kubeconverters.APIKeySecretConverter{}

		glooSecret = &v1.Secret{
			Metadata: &core.Metadata{
				Name:            "foo",
				Namespace:       "bar",
				OwnerReferences: []*core.Metadata_OwnerReference{},
			},
			Kind: &v1.Secret_ApiKey{
				ApiKey: &v12.ApiKey{
					ApiKey: "apikey",
					Metadata: map[string]string{
						"some-data": "some-val",
					},
				},
			},
		}

		clientset := fake.NewSimpleClientset()
		coreCache, err := cache.NewKubeCoreCache(ctx, clientset)
		Expect(err).NotTo(HaveOccurred())
		resourceClient, err = kubesecret.NewResourceClient(clientset, &v1.Secret{}, false, coreCache)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("converting from a Kubernetes secret to a Gloo one", func() {

		It("ignores secrets that aren't API keys", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Data: map[string][]byte{
					"foo": {0, 1, 2},
				},
				Type: corev1.SecretTypeOpaque,
			}
			glooSecret, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(glooSecret).To(BeNil())
		})

		It("ignores API key secrets that do not contain an API key", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Data: map[string][]byte{
					"some-data": []byte("some-val"),
				},
				Type: kubeconverters.APIKeySecretType,
			}
			glooSecret, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(glooSecret).To(BeNil())
		})

		It("correctly converts API key secrets", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Data: map[string][]byte{
					kubeconverters.APIKeyDataKey: []byte("apikey"),
					"some-data":                  []byte("some-val"),
				},
				Type: kubeconverters.APIKeySecretType,
			}
			actual, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(Equal(glooSecret))
		})
	})

	Describe("converting from a Gloo secret to a Kubernetes one", func() {

		It("ignores resources that are not secrets", func() {
			actual, err := converter.ToKubeSecret(ctx, resourceClient, &v1.Proxy{})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(BeNil())
		})

		It("ignores secret that are not API key secrets", func() {
			actual, err := converter.ToKubeSecret(ctx, resourceClient, &v1.Secret{
				Metadata: &core.Metadata{Name: "foo"},
				Kind:     &v1.Secret_Aws{},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(BeNil())
		})

		It("correctly converts API key secrets", func() {
			expected := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					OwnerReferences: []metav1.OwnerReference{},
				},
				StringData: map[string]string{
					kubeconverters.APIKeyDataKey: "apikey",
					"some-data":                  "some-val",
				},
				Type: kubeconverters.APIKeySecretType,
			}

			actual, err := converter.ToKubeSecret(ctx, resourceClient, glooSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})

		It("correctly converts the old API key format to the new one", func() {
			expected := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					OwnerReferences: []metav1.OwnerReference{},
					Annotations:     map[string]string{},
				},
				StringData: map[string]string{
					kubeconverters.APIKeyDataKey: "apikey",
					"some-data":                  "some-val",
				},
				Type: kubeconverters.APIKeySecretType,
			}

			glooSecret.Metadata.Annotations = map[string]string{
				kubeconverters.GlooKindAnnotationKey: "*v1.Secret",
			}

			actual, err := converter.ToKubeSecret(ctx, resourceClient, glooSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})

	})

})
