package metrics_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils/metrics"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var (
	namespace = "test-ns"
)

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

func makeUpstreamGroup(nameSuffix string) resources.Resource {
	return &gloov1.UpstreamGroup{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      "usg-" + nameSuffix,
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

func makeProxy(nameSuffix string) resources.Resource {
	return &gloov1.Proxy{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      "secret-" + nameSuffix,
		},
	}
}

var _ = Describe("ConfigStatusMetrics Test", func() {
	DescribeTable("SetResource[Invalid|Valid] works as expected",
		func(gvk string, metricName string, makeResource func(nameSuffix string) resources.Resource) {
			opts := map[string]*metrics.MetricLabels{
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
			res1Name := res1.GetMetadata().GetName()
			res2Name := res2.GetMetadata().GetName()

			// Metrics should not have any data initially
			_, err = helpers.ReadMetricByLabel(metricName, "name", res1Name)
			Expect(err).To(HaveOccurred())
			_, err = helpers.ReadMetricByLabel(metricName, "name", res2Name)
			Expect(err).To(HaveOccurred())

			// Setting res1 invalid should not affect res2
			c.SetResourceInvalid(context.TODO(), res1)
			Expect(helpers.ReadMetricByLabel(metricName, "name", res1Name)).To(Equal(1))
			_, err = helpers.ReadMetricByLabel(metricName, "name", res2Name)
			Expect(err).To(HaveOccurred())

			// Setting res2 invalid should not affect res1
			c.SetResourceInvalid(context.TODO(), res2)
			Expect(helpers.ReadMetricByLabel(metricName, "name", res1Name)).To(Equal(1))
			Expect(helpers.ReadMetricByLabel(metricName, "name", res2Name)).To(Equal(1))

			// Setting both back to valid should return 0, not error
			c.SetResourceValid(context.TODO(), res1)
			c.SetResourceValid(context.TODO(), res2)
			Expect(helpers.ReadMetricByLabel(metricName, "name", res1Name)).To(Equal(0))
			Expect(helpers.ReadMetricByLabel(metricName, "name", res2Name)).To(Equal(0))
		},
		Entry("Virtual Service", "VirtualService.v1.gateway.solo.io", metrics.Names[gwv1.VirtualServiceGVK], makeVirtualService),
		Entry("Gateway", "Gateway.v1.gateway.solo.io", metrics.Names[gwv1.GatewayGVK], makeGateway),
		Entry("RouteTable", "RouteTable.v1.gateway.solo.io", metrics.Names[gwv1.RouteTableGVK], makeRouteTable),
		Entry("Upstream", "Upstream.v1.gloo.solo.io", metrics.Names[gloov1.UpstreamGVK], makeUpstream),
		Entry("UpstreamGroup", "UpstreamGroup.v1.gloo.solo.io", metrics.Names[gloov1.UpstreamGroupGVK], makeUpstreamGroup),
		Entry("Secret", "Secret.v1.gloo.solo.io", metrics.Names[gloov1.SecretGVK], makeSecret),
		Entry("Proxy", "Proxy.v1.gloo.solo.io", metrics.Names[gloov1.ProxyGVK], makeProxy),
	)
})
