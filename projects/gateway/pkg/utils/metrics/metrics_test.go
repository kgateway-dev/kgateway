package metrics_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.opencensus.io/stats/view"
)

var (
	namespace = "test-ns"
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

func makeVirtualService(nameSuffix string) resources.Resource {
	return &gwv1.VirtualService{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      "vs-" + nameSuffix,
		},
	}
}

func makeGateway(nameSuffix string) resources.Resource {
	return &gwv1.Gateway{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      "gw-" + nameSuffix,
		},
	}
}

func makeRouteTable(nameSuffix string) resources.Resource {
	return &gwv1.RouteTable{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      "rt-" + nameSuffix,
		},
	}
}

func makeUpstream(nameSuffix string) resources.Resource {
	return &gloov1.Upstream{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      "us-" + nameSuffix,
		},
	}
}

func makeSecret(nameSuffix string) resources.Resource {
	return &gloov1.Secret{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      "secret-" + nameSuffix,
		},
	}
}

var _ = Describe("ConfigStatusMetrics Test", func() {
	DescribeTable("SetResource[Invalid|Valid] works as expected",
		func(gvk string, metricName string, makeResource func(nameSuffix string) resources.Resource) {
			opts := map[string]*metrics.Labels{
				gvk: {
					LabelToPath: map[string]string{
						"name": "{.metadata.name}",
					},
				},
			}
			c, err := metrics.NewConfigStatusMetrics(opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(c).NotTo(BeNil())

			// Create two resources
			res1 := makeResource("1")
			res2 := makeResource("2")

			// Metrics should be 0 initially
			val1 := getGauge(metricName, "name", res1.GetMetadata().GetName())
			Expect(val1).To(Equal(0))
			val2 := getGauge(metricName, "name", res2.GetMetadata().GetName())
			Expect(val2).To(Equal(0))

			// Setting res1 invalid should not affect res2
			c.SetResourceInvalid(context.TODO(), res1)
			val1 = getGauge(metricName, "name", res1.GetMetadata().GetName())
			Expect(val1).To(Equal(1))
			val2 = getGauge(metricName, "name", res2.GetMetadata().GetName())
			Expect(val2).To(Equal(0))

			// Setting res2 invalid should not affect res1
			c.SetResourceInvalid(context.TODO(), res2)
			val1 = getGauge(metricName, "name", res1.GetMetadata().GetName())
			Expect(val1).To(Equal(1))
			val2 = getGauge(metricName, "name", res2.GetMetadata().GetName())
			Expect(val2).To(Equal(1))

			// Set both back to valid
			c.SetResourceValid(context.TODO(), res1)
			c.SetResourceValid(context.TODO(), res2)
			val1 = getGauge(metricName, "name", res1.GetMetadata().GetName())
			Expect(val1).To(Equal(0))
			val2 = getGauge(metricName, "name", res2.GetMetadata().GetName())
			Expect(val2).To(Equal(0))
		},
		Entry("Virtual Service", "VirtualService.v1.gateway.solo.io", metrics.Names[gwv1.VirtualServiceGVK], makeVirtualService),
		Entry("Gateway", "Gateway.v1.gateway.solo.io", metrics.Names[gwv1.GatewayGVK], makeGateway),
		Entry("RouteTable", "RouteTable.v1.gateway.solo.io", metrics.Names[gwv1.RouteTableGVK], makeRouteTable),
		Entry("Upstream", "Upstream.v1.gloo.solo.io", metrics.Names[gloov1.UpstreamGVK], makeUpstream),
		Entry("Secret", "Secret.v1.gloo.solo.io", metrics.Names[gloov1.SecretGVK], makeSecret),
	)
})
