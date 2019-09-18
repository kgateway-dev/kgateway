package kubeconverters_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/solo-io/gloo/projects/gloo/pkg/api/converters/kube"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("Configmapconverter", func() {

	Context("kube converter", func() {
		It("should convert config map to artifact", func() {
			kubeConverter := NewKubeConfigMapConverter()
			cfgmap := &kubev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cfg", Namespace: "foo",
					Labels:          map[string]string{"foo": "bar"},
					Annotations:     map[string]string{"foo": "bar2"},
					OwnerReferences: []metav1.OwnerReference{},
				},
				Data: map[string]string{
					"test": "data",
				},
			}
			artifact, err := kubeConverter.FromKubeConfigMapWithResource(context.TODO(), new(v1.Artifact), "artifact", cfgmap)
			Expect(err).NotTo(HaveOccurred())
			Expect(artifact).NotTo(BeNil())
			cfgmap2, err := kubeConverter.ToKubeConfigMapSimple(context.TODO(), artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfgmap).To(Equal(cfgmap2))

		})
	})
})
