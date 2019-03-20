package kubeconverters_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Secretconverter", func() {
	It("should covnert kube secret to gloo secret", func() {
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

	It("should covnert to gloo secret kube in gloo format", func() {
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
			},
		}
		var t TLSSecretConverter
		resource, err := t.FromKubeSecret(context.Background(), nil, secret)
		Expect(err).NotTo(HaveOccurred())
		kubeSecret, err := t.ToKubeSecret(context.Background(), nil, resource)
		Expect(err).NotTo(HaveOccurred())

		Expect(secret).To(Equal(kubeSecret))

	})
})
