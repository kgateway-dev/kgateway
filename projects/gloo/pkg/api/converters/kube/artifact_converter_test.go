package kubeconverters_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
)

var _ = Describe("Artifact converters", func() {

	It("should convert config map to artifact and back preserving all information", func() {
		configMap := &kubev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "cfg",
				Namespace:       "foo",
				Labels:          map[string]string{"foo": "bar"},
				Annotations:     map[string]string{"foo": "bar2"},
				OwnerReferences: []metav1.OwnerReference{},
			},
			Data: map[string]string{
				"test": "data",
			},
		}
		artifact := KubeConfigMapToArtifact(configMap)
		Expect(artifact).NotTo(BeNil())

		cfgmap2, err := ArtifactToKubeConfigMap(artifact)
		Expect(err).NotTo(HaveOccurred())
		Expect(configMap).To(Equal(cfgmap2))
	})
})
