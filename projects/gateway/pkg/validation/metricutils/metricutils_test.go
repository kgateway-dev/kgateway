package metricutils_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/validation/metricutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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
			opts := &metricutils.ConfigStatusMetricsOpts{
				VirtualServiceLabels: &v1.Settings_ObservabilityOptions_ConfigStatusMetricsOptions_MetricLabels{
					LabelToPath: map[string]string{
						"namespace": "{.metadata.namespace}",
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
			opts := &metricutils.ConfigStatusMetricsOpts{
				VirtualServiceLabels: &v1.Settings_ObservabilityOptions_ConfigStatusMetricsOptions_MetricLabels{
					LabelToPath: map[string]string{
						"namespace": "{.metadata.namespace}",
					},
				},
			}
			c, err := metricutils.NewConfigStatusMetrics(opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(c).NotTo(BeNil())

			// Should be 0 initially
			res := makeVirtualService("test-ns", "some-vs")
			val := getGauge("validation.gateway.solo.io/virtual_service_config_status", "namespace", "test-ns")
			Expect(val).To(Equal(0))
			// Should increment to 1
			c.SetResourceInvalid(context.TODO(), res)
			val = getGauge("validation.gateway.solo.io/virtual_service_config_status", "namespace", "test-ns")
			Expect(val).To(Equal(1))
			// Should return to 0
			c.SetResourceValid(context.TODO(), res)
			val = getGauge("validation.gateway.solo.io/virtual_service_config_status", "namespace", "test-ns")
			Expect(val).To(Equal(0))
		})
		// TODO(mitchaman): Add test which set multiple VS, make sure they're recorded independently
	})
})
