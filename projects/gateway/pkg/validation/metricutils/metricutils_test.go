package metricutils_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/validation/metricutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.opencensus.io/stats/view"
)

func getGauge(viewName string, labelKey string, labelValue string) int {
	rows, err := view.RetrieveData(viewName)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	for _, row := range rows {
		for _, tag := range row.Tags {
			if tag.Key.Name() == labelKey && tag.Value == labelValue {
				return int(row.Data.(*view.LastValueData).Value)
			}
		}
	}
	return 0
}

func makeVirtualService(namespace string, name string) *gwv1.VirtualService {
	return &gwv1.VirtualService{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      name,
		},
	}
}

var _ = Describe("ConfigStatusMetrics Test", func() {
	Context("NewConfigStatusMetrics", func() {
		It("Should work", func() {
			opts := map[string]*metricutils.MetricLabels{
				"VirtualService.v1.gateway.solo.io": {
					LabelToPath: map[string]string{
						"name": "{.metadata.name}",
					},
				},
			}
			c, err := metricutils.NewConfigStatusMetrics(opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(c).NotTo(BeNil())
		})
	})
	Context("SetResource[Invalid|Valid]", func() {
		It("Should increment the gauge after SetResourceInvalid, and decrement after SetResourceValid", func() {
			opts := map[string]*metricutils.MetricLabels{
				"VirtualService.v1.gateway.solo.io": {
					LabelToPath: map[string]string{
						"name": "{.metadata.name}",
					},
				},
			}
			c, err := metricutils.NewConfigStatusMetrics(opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(c).NotTo(BeNil())

			// Should be 0 initially
			res := makeVirtualService("test-ns", "some-vs")
			val := getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "some-vs")
			Expect(val).To(Equal(0))
			// Should increment to 1
			c.SetResourceInvalid(context.TODO(), res)
			val = getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "some-vs")
			Expect(val).To(Equal(1))
			// Should return to 0
			c.SetResourceValid(context.TODO(), res)
			val = getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "some-vs")
			Expect(val).To(Equal(0))
		})
		It("Should track metrics for resources of the same type independently", func() {
			opts := map[string]*metricutils.MetricLabels{
				"VirtualService.v1.gateway.solo.io": {
					LabelToPath: map[string]string{
						"name": "{.metadata.name}",
					},
				},
			}
			c, err := metricutils.NewConfigStatusMetrics(opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(c).NotTo(BeNil())

			vs1 := makeVirtualService("test-ns", "vs1")
			vs2 := makeVirtualService("test-ns", "vs2")

			// Should be 0 initially
			val1 := getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "vs1")
			Expect(val1).To(Equal(0))
			val2 := getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "vs2")
			Expect(val2).To(Equal(0))

			// Setting vs1 invalid should not affect vs2
			c.SetResourceInvalid(context.TODO(), vs1)
			val1 = getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "vs1")
			Expect(val1).To(Equal(1))
			val2 = getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "vs2")
			Expect(val2).To(Equal(0))

			// Setting vs2 invalid should not affect vs1
			c.SetResourceInvalid(context.TODO(), vs2)
			val1 = getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "vs1")
			Expect(val1).To(Equal(1))
			val2 = getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "vs2")
			Expect(val2).To(Equal(1))

			// Set both back to valid
			c.SetResourceValid(context.TODO(), vs1)
			c.SetResourceValid(context.TODO(), vs2)
			val1 = getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "vs1")
			Expect(val1).To(Equal(0))
			val2 = getGauge("validation.gateway.solo.io/virtual_service_config_status", "name", "vs2")
			Expect(val2).To(Equal(0))
		})
	})
})
