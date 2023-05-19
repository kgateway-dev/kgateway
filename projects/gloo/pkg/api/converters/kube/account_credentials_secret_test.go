package kubeconverters_test

import (
	"context"

	kubeconverters "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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
	const (
		expectedUser = "someuser"
		expectedPass = "apassword"
	)
	BeforeEach(func() {
		ctx = context.TODO()
		converter = &kubeconverters.AccountCredentialsSecretConverter{}

		glooSecret = &v1.Secret{
			Metadata: &core.Metadata{
				Name:      "foo",
				Namespace: "bar",
			},
			Kind: &v1.Secret_Credentials{
				Credentials: &v1.AccountCredentialsSecret{
					Username: expectedUser,
					Password: expectedPass,
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

		It("ignores secrets that aren't AccountCredentials", func() {
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
			actual, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(BeNil())
		})

		It("ignores AccountCredentials secrets that do not contain a username and password", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Data: map[string][]byte{
					"some-data": []byte("some-val"),
				},
				Type: kubeconverters.AccountCredentialsSecretType,
			}
			actual, err := converter.FromKubeSecret(ctx, resourceClient, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(BeNil())
		})

		It("correctly converts AccountCredentials secrets", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Data: map[string][]byte{
					kubeconverters.UsernameDataKey: []byte(expectedUser),
					kubeconverters.PasswordDataKey: []byte(expectedPass),
				},
				Type: kubeconverters.AccountCredentialsSecretType,
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

		It("ignores secret that are not AccountCredentials secrets", func() {
			actual, err := converter.ToKubeSecret(ctx, resourceClient, &v1.Secret{
				Metadata: &core.Metadata{Name: "foo"},
				Kind:     &v1.Secret_Aws{},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(BeNil())
		})

		It("correctly converts AccountCredentials secrets", func() {
			expected := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					OwnerReferences: []metav1.OwnerReference{},
				},
				StringData: map[string]string{
					kubeconverters.UsernameDataKey: expectedUser,
					kubeconverters.PasswordDataKey: expectedPass,
				},
				Type: kubeconverters.AccountCredentialsSecretType,
			}

			actual, err := converter.ToKubeSecret(ctx, resourceClient, glooSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})

	})

})
