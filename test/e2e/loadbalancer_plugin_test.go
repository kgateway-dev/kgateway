package e2e_test

import (
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/testutils"

	envoy_admin_v3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	glooV1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"
)

// setupLBPluginTest sets up a test context with a virtual service that uses the provided load balancer config
func setupLBPluginTest(testContext *e2e.TestContext, lbConfig *glooV1.LoadBalancerConfig) {
	upstream := testContext.TestUpstream().Upstream
	upstream.LoadBalancerConfig = lbConfig

	customVS := helpers.NewVirtualServiceBuilder().
		WithName("vs-test").
		WithNamespace(writeNamespace).
		WithDomain("custom-domain.com").
		WithRoutePrefixMatcher(e2e.DefaultRouteName, "/endpoint").
		WithRouteActionToUpstream(e2e.DefaultRouteName, upstream).
		Build()

	testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
		customVS,
	}
}

var _ = Describe("Load Balancer Plugin", Label(), func() {
	var (
		testContext *e2e.TestContext
	)

	BeforeEach(func() {
		var testRequirements []testutils.Requirement

		testContext = testContextFactory.NewTestContext(testRequirements...)
		testContext.BeforeEach()
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("Maglev LoadBalancer", func() {
		BeforeEach(func() {
			setupLBPluginTest(testContext, &glooV1.LoadBalancerConfig{
				Type: &glooV1.LoadBalancerConfig_Maglev_{
					Maglev: &glooV1.LoadBalancerConfig_Maglev{},
				},
			})
		})

		It("can route traffic", func() {
			requestBuilder := testContext.GetHttpRequestBuilder().
				WithHost("custom-domain.com").
				WithPath("endpoint")

			Eventually(func(g Gomega) {
				g.Expect(testutils.DefaultHttpClient.Do(requestBuilder.Build())).Should(matchers.HaveOkResponse())
			}, "5s", ".5s").Should(Succeed())
		})

		It("should have expected envoy config", func() {
			Eventually(func(g Gomega) {
				dump, err := testContext.EnvoyInstance().StructuredConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				dacs, err := findDynamicActiveClusters(dump)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(dacs).NotTo(BeEmpty())
				g.Expect(dacs).To(HaveLen(1))

				g.Expect(dacs[0].LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_MAGLEV))
				g.Expect(dacs[0].CommonLbConfig).To(BeNil())
			}, "5s", ".5s").Should(Succeed())
		})
	})

	Context("Maglev LB w/ close connections on set change", func() {
		BeforeEach(func() {
			setupLBPluginTest(testContext, &glooV1.LoadBalancerConfig{
				Type: &glooV1.LoadBalancerConfig_Maglev_{
					Maglev: &glooV1.LoadBalancerConfig_Maglev{},
				},
				CloseConnectionsOnHostSetChange: true,
			})
		})

		It("should have expected envoy config", func() {
			Eventually(func(g Gomega) {
				dump, err := testContext.EnvoyInstance().StructuredConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				dacs, err := findDynamicActiveClusters(dump)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(dacs).NotTo(BeEmpty())
				g.Expect(dacs).To(HaveLen(1))

				g.Expect(dacs[0].LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_MAGLEV), dacs[0])
				g.Expect(dacs[0].CommonLbConfig).ToNot(BeNil())
				g.Expect(dacs[0].CommonLbConfig.CloseConnectionsOnHostSetChange).To(BeTrue())
			}, "5s", ".5s").Should(Succeed())
		})
	})
})

// findDynamicActiveClusters finds the dynamic active clusters in the config dump
func findDynamicActiveClusters(dump *envoy_admin_v3.ConfigDump) ([]*envoy_config_cluster_v3.Cluster, error) {
	clusters := []*envoy_config_cluster_v3.Cluster{}

	var found []*envoy_admin_v3.ClustersConfigDump_DynamicCluster
	for _, cfg := range dump.Configs {
		if cfg.TypeUrl == "type.googleapis.com/envoy.admin.v3.ClustersConfigDump" {
			clusterConfigDump := &envoy_admin_v3.ClustersConfigDump{}
			err := cfg.UnmarshalTo(clusterConfigDump)
			if err != nil {
				return nil, err
			}

			found = clusterConfigDump.DynamicActiveClusters
		}
	}

	if found == nil {
		return clusters, nil
	}

	for _, clusterDump := range found {
		cluster := envoy_config_cluster_v3.Cluster{}
		err := clusterDump.Cluster.UnmarshalTo(&cluster)
		if err != nil {
			return nil, err
		}

		clusters = append(clusters, &cluster)
	}

	return clusters, nil
}
