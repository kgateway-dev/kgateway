package kubeconverters_test

import (
	"context"
	"reflect"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	. "github.com/solo-io/solo-kit/test/matchers"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SecretConverter", func() {
	It("should have the correct number of converters with cli components", func() {
		// note that this test does not confirm that the number of secrets are being used in the cli
		converterField, ok := reflect.TypeOf(*GlooSecretConverterChain).FieldByName("converters")
		Expect(ok).To(BeTrue())
		convertersValue := reflect.ValueOf(*GlooSecretConverterChain).FieldByName(converterField.Name)
		// NOTE: when adding a converter here, please add the glooctl command
		// for the secret converter as well add to cli projects/gloo/cli/pkg/cmd/create/secret
		Expect(convertersValue.Len()).To(Equal(8))
	})
	It("should convert kube secret to gloo secret", func() {
		secret := &kubev1.Secret{
			Type: kubev1.SecretTypeTLS,
			Data: map[string][]byte{
				kubev1.TLSCertKey:              []byte("cert"),
				kubev1.TLSPrivateKeyKey:        []byte("key"),
				kubev1.ServiceAccountRootCAKey: []byte("ca"),
				OCSPStapleKey:                  []byte("ocsp"),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "s1",
				Namespace:       "ns",
				OwnerReferences: []metav1.OwnerReference{},
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
		Expect(glooSecret.RootCa).To(BeEquivalentTo(secret.Data[kubev1.ServiceAccountRootCAKey]))
		Expect(glooSecret.OcspStaple).To(BeEquivalentTo(secret.Data[OCSPStapleKey]))
	})

	It("should convert kube secret to gloo secret without optional root ca or optional ocsp staple", func() {
		secret := &kubev1.Secret{
			Type: kubev1.SecretTypeTLS,
			Data: map[string][]byte{
				kubev1.TLSCertKey:       []byte("cert"),
				kubev1.TLSPrivateKeyKey: []byte("key"),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "s1",
				Namespace:       "ns",
				OwnerReferences: []metav1.OwnerReference{},
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
		Expect(glooSecret.RootCa).To(BeEquivalentTo(""))
		Expect(glooSecret.OcspStaple).To(BeEquivalentTo(""))
	})

	It("should convert to gloo secret kube in gloo format", func() {
		secret := &v1.Secret{
			Kind: &v1.Secret_Tls{
				Tls: &v1.TlsSecret{
					PrivateKey: "key",
					CertChain:  "cert",
					RootCa:     "ca",
					OcspStaple: []byte("ocsp"),
				},
			},
			Metadata: &core.Metadata{
				Name:      "s1",
				Namespace: "ns",
			},
		}
		var t TLSSecretConverter
		kubeSecret, err := t.ToKubeSecret(context.Background(), nil, secret)
		Expect(err).NotTo(HaveOccurred())
		Expect(kubeSecret).To(MatchProto(&kubev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "s1",
				Namespace: "ns",
			},
			Data: map[string][]byte{
				"tls.key":         []byte("key"),
				"tls.crt":         []byte("cert"),
				"ca.crt":          []byte("ca"),
				"tls.ocsp-staple": []byte("ocsp"),
			},
			Type: "kubernetes.io/tls",
		}))
	})

	It("should round trip kube ssl secret back to kube ssl secret", func() {
		secret := &kubev1.Secret{
			Type: kubev1.SecretTypeTLS,
			Data: map[string][]byte{
				kubev1.TLSCertKey:              []byte("cert"),
				kubev1.TLSPrivateKeyKey:        []byte("key"),
				kubev1.ServiceAccountRootCAKey: []byte("ca"),
				OCSPStapleKey:                  []byte("ocsp"),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "s1",
				Namespace:       "ns",
				Labels:          map[string]string{},
				OwnerReferences: []metav1.OwnerReference{},
			},
		}
		var t TLSSecretConverter
		resource, err := t.FromKubeSecret(context.Background(), nil, secret)
		Expect(err).NotTo(HaveOccurred())
		kubeSecret, err := t.ToKubeSecret(context.Background(), nil, resource)
		Expect(err).NotTo(HaveOccurred())

		Expect(secret).To(Equal(kubeSecret))

	})

	It("should round trip kube ssl secret back to kube ssl secret without optional root ca or ocsp staple", func() {
		secret := &kubev1.Secret{
			Type: kubev1.SecretTypeTLS,
			Data: map[string][]byte{
				kubev1.TLSCertKey:       []byte("cert"),
				kubev1.TLSPrivateKeyKey: []byte("key"),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "s1",
				Namespace:       "ns",
				Labels:          map[string]string{},
				OwnerReferences: []metav1.OwnerReference{},
			},
		}
		var t TLSSecretConverter
		resource, err := t.FromKubeSecret(context.Background(), nil, secret)
		Expect(err).NotTo(HaveOccurred())
		kubeSecret, err := t.ToKubeSecret(context.Background(), nil, resource)
		Expect(err).NotTo(HaveOccurred())

		Expect(secret).To(Equal(kubeSecret))

	})

	It("should round trip kube aws secret to gloo aws secret and back to kube aws secret", func() {
		awsSecret := &v1.AwsSecret{
			AccessKey:    "access",
			SecretKey:    "secret",
			SessionToken: "token",
		}
		kubeSecret := &kubev1.Secret{
			Type: kubev1.SecretTypeOpaque,
			ObjectMeta: metav1.ObjectMeta{
				Name:            "s1",
				Namespace:       "ns",
				Labels:          map[string]string{},
				OwnerReferences: []metav1.OwnerReference{},
			},
			Data: map[string][]byte{
				AwsAccessKeyName:    []byte(awsSecret.AccessKey),
				AwsSecretKeyName:    []byte(awsSecret.SecretKey),
				AwsSessionTokenName: []byte(awsSecret.SessionToken),
			},
		}
		var awsConverter AwsSecretConverter
		chainedConverter := NewSecretConverterChain(&awsConverter)
		resource, err := chainedConverter.FromKubeSecret(context.Background(), nil, kubeSecret)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.GetMetadata().Name).To(Equal("s1"))
		Expect(resource.(*v1.Secret).Kind.(*v1.Secret_Aws).Aws).To(Equal(awsSecret))
		derivedSecret, err := chainedConverter.ToKubeSecret(context.Background(), nil, resource)
		Expect(err).NotTo(HaveOccurred())
		Expect(derivedSecret).To(Equal(kubeSecret))
	})

	It("converter chain should exit in expected order", func() {
		secret := &kubev1.Secret{
			Data: map[string][]byte{
				AwsAccessKeyName: []byte("access"),
				AwsSecretKeyName: []byte("secret"),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            "s1",
				Namespace:       "ns",
				Labels:          map[string]string{},
				OwnerReferences: []metav1.OwnerReference{},
			},
		}
		var awsConverter AwsSecretConverter
		mockKube := &kubev1.Secret{Type: "this is a mock"}
		mockResource := &v1.Secret{Metadata: &core.Metadata{Name: "mock-name"}}
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
