package metrics

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("ConfigStatusMetrics Internal Test", func() {

	Context("extractValueFromResource", func() {
		It("Should return the value when the path is valid", func() {
			res := makeVirtualService("test-ns", "some-vs")
			path := "{.metadata.name}"

			val, err := extractValueFromResource(res, path)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("some-vs"))
		})
		It("Should return an error when the path is invalid", func() {
			res := makeVirtualService("test-ns", "some-vs")
			path := "{"

			_, err := extractValueFromResource(res, path)
			Expect(err).To(HaveOccurred())
		})
	})
	Context("getMutators", func() {
		It("Should work", func() {
			m := &resourceMetric{
				gauge: nil,
				labelToPath: map[string]string{
					// label key cannot be empty
					"somePath": "{.some.path}",
				},
			}
			res := &gwv1.VirtualService{}
			mutators, err := getMutators(m, res)

			Expect(err).NotTo(HaveOccurred())
			Expect(mutators).NotTo(BeNil())
			Expect(mutators).To(HaveLen(1))
		})
		It("Should return an error if a resource metric has an invalid label key", func() {
			m := &resourceMetric{
				gauge: nil,
				labelToPath: map[string]string{
					// label key cannot be empty
					"": "{.some.path}",
				},
			}
			res := &gwv1.VirtualService{}
			_, err := getMutators(m, res)
			Expect(err).To(HaveOccurred())
		})
	})
})

func makeVirtualService(namespace string, name string) *gwv1.VirtualService {
	return &gwv1.VirtualService{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      name,
		},
	}
}
