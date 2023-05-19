package kubeconverters_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("Encryption Key converters", func() {
	const encryptionValue = "This is the encryption key"

	It("should convert secret to encryption key secret and back preserving all information", func() {
		encryptionSecret := &v1.EncryptionKeySecret{
			Key: encryptionValue,
		}
		kubeSecret := &kubev1.Secret{
			Type: EncryptionKeySecretType,
			ObjectMeta: metav1.ObjectMeta{
				Name:            "s1",
				Namespace:       "ns",
				Labels:          map[string]string{},
				OwnerReferences: []metav1.OwnerReference{},
			},
			Data: map[string][]byte{
				EncryptionDataKey: []byte(encryptionSecret.Key),
			},
		}
		encryptionConverter := EncryptionSecretConverter{}
		chainedConverter := NewSecretConverterChain(&encryptionConverter)
		resource, err := chainedConverter.FromKubeSecret(context.Background(), nil, kubeSecret)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.GetMetadata().Name).To(Equal("s1"))
		Expect(resource.(*v1.Secret).Kind.(*v1.Secret_Encryption).Encryption).To(Equal(encryptionSecret))
		derivedSecret, err := chainedConverter.ToKubeSecret(context.Background(), nil, resource)
		Expect(err).NotTo(HaveOccurred())
		Expect(derivedSecret).To(Equal(kubeSecret))
	})
})
