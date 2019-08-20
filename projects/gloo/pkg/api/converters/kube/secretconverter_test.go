package kubeconverters_test

import (
	"context"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SecretConverter", func() {
	It("should convert kube secret to gloo secret", func() {
		secret := &kubev1.Secret{
			Type: kubev1.SecretTypeTLS,
			Data: map[string][]byte{
				kubev1.TLSCertKey:       []byte("cert"),
				kubev1.TLSPrivateKeyKey: []byte("key"),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "s1",
				Namespace: "ns",
			},
		}
		var t TLSSecretConverter
		resource, err := t.FromKubeSecret(context.Background(), nil, secret)
		Expect(err).NotTo(HaveOccurred())
		glooSecret := resource.(*v1.Secret).Kind.(*v1.Secret_Tls).Tls
		Expect(resource.GetMetadata().Name).To(Equal(secret.ObjectMeta.Name))
		Expect(resource.GetMetadata().Namespace).To(Equal(secret.ObjectMeta.Namespace))

		Expect(glooSecret.CertChain).To(BeEquivalentTo(secret.Data[kubev1.TLSCertKey]))
		Expect(glooSecret.PrivateKey).To(BeEquivalentTo(secret.Data[kubev1.TLSPrivateKeyKey]))
	})

	It("should convert to gloo secret kube in gloo format", func() {
		secret := &v1.Secret{
			Kind: &v1.Secret_Tls{
				Tls: &v1.TlsSecret{
					PrivateKey: "key",
					CertChain:  "cert",
				},
			},
			Metadata: core.Metadata{
				Name:      "s1",
				Namespace: "ns",
			},
		}
		var t TLSSecretConverter
		kubeSecret, err := t.ToKubeSecret(context.Background(), nil, secret)
		Expect(err).NotTo(HaveOccurred())

		// use default behavior
		Expect(kubeSecret).To(BeNil())
	})

	It("should round trip kube ssl secret back to kube ssl secret", func() {
		secret := &kubev1.Secret{
			Type: kubev1.SecretTypeTLS,
			Data: map[string][]byte{
				kubev1.TLSCertKey:       []byte("cert"),
				kubev1.TLSPrivateKeyKey: []byte("key"),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "s1",
				Namespace: "ns",
				Labels:    map[string]string{},
			},
		}
		var t TLSSecretConverter
		resource, err := t.FromKubeSecret(context.Background(), nil, secret)
		Expect(err).NotTo(HaveOccurred())
		kubeSecret, err := t.ToKubeSecret(context.Background(), nil, resource)
		Expect(err).NotTo(HaveOccurred())

		Expect(secret).To(Equal(kubeSecret))

	})

	It("should convert annotation-free from of aws secret from kube secret", func() {
		secret := &kubev1.Secret{
			Data: map[string][]byte{
				AwsAccessKeyName: []byte("access"),
				AwsSecretKeyName: []byte("secret"),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "s1",
				Namespace: "ns",
				Labels:    map[string]string{},
			},
		}
		var awsConverter AwsSecretConverter
		mockKube := &kubev1.Secret{Type: "this is a mock"}
		mockResource := &v1.Secret{Metadata: core.Metadata{Name: "mock-name"}}
		mockConverter := newMockConverter(mockKube, mockResource)
		chainedConverter := NewSecretConverterChain(&awsConverter, mockConverter)
		resource, err := chainedConverter.FromKubeSecret(context.Background(), nil, secret)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.GetMetadata().Name).To(Equal("s1"))
		Expect(resource.(*v1.Secret).Kind.(*v1.Secret_Aws).Aws).NotTo(Equal(""))
		kubeSecret, err := chainedConverter.ToKubeSecret(context.Background(), nil, resource)
		Expect(err).NotTo(HaveOccurred())
		Expect(kubeSecret).To(Equal(mockKube))
	})

	It("converter chain should exit in expected order", func() {
		secret := &kubev1.Secret{
			Data: map[string][]byte{
				AwsAccessKeyName: []byte("access"),
				AwsSecretKeyName: []byte("secret"),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "s1",
				Namespace: "ns",
				Labels:    map[string]string{},
			},
		}
		var awsConverter AwsSecretConverter
		mockKube := &kubev1.Secret{Type: "this is a mock"}
		mockResource := &v1.Secret{Metadata: core.Metadata{Name: "mock-name"}}
		mockConverter := newMockConverter(mockKube, mockResource)
		chainedConverterMockFirst := NewSecretConverterChain(mockConverter, &awsConverter)
		resource, err := chainedConverterMockFirst.FromKubeSecret(context.Background(), nil, secret)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.GetMetadata().Name).To(Equal("mock-name"))
		kubeSecret, err := chainedConverterMockFirst.ToKubeSecret(context.Background(), nil, resource)
		Expect(err).NotTo(HaveOccurred())
		Expect(kubeSecret).To(Equal(mockKube))
	})
})

// This can be used to test the secret converter chain.
// It will always return the secrets that you construct it with.
type mockConverter struct {
	terminalKubeSecret *kubev1.Secret
	terminalGlooSecret resources.Resource
}

var _ kubesecret.SecretConverter = &mockConverter{}

func newMockConverter(kube *kubev1.Secret, resource resources.Resource) *mockConverter {
	return &mockConverter{
		terminalKubeSecret: kube,
		terminalGlooSecret: resource,
	}
}
func (t *mockConverter) FromKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, secret *kubev1.Secret) (resources.Resource, error) {
	return t.terminalGlooSecret, nil
}
func (t *mockConverter) ToKubeSecret(ctx context.Context, rc *kubesecret.ResourceClient, resource resources.Resource) (*kubev1.Secret, error) {
	return t.terminalKubeSecret, nil
}
