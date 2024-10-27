package e2e_test

import (
	"fmt"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/services/envoy"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/gloo/test/v1helpers"

	envoy_admin_v3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	glooV1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/helpers"
)

// setupLBPluginTest sets up a test context with a virtual service that uses the provided load balancer config
func setupLBPluginTest(testContext *e2e.TestContext, lbConfig *glooV1.LoadBalancerConfig) {
	upstream := testContext.TestUpstream().Upstream
	upstream.LoadBalancerConfig = lbConfig

	dest := &gloov1.MultiDestination{
		Destinations: []*gloov1.WeightedDestination{{
			Weight: &wrappers.UInt32Value{Value: 1},
			Destination: &gloov1.Destination{
				DestinationType: &gloov1.Destination_Upstream{
					Upstream: testContext.TestUpstream().Upstream.Metadata.Ref(),
				},
			},
		}},
	}

	customVS := helpers.NewVirtualServiceBuilder().
		WithName("vs-test").
		WithNamespace(writeNamespace).
		WithDomain("custom-domain.com").
		WithRoutePrefixMatcher(e2e.DefaultRouteName, "/endpoint").
		WithRouteActionToMultiDestination(e2e.DefaultRouteName, dest).
		Build()

	testContext.ResourcesToCreate().VirtualServices = v1.VirtualServiceList{
		customVS,
	}
}

var _ = FDescribe("Load Balancer Plugin", Label(), func() {
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

		It("should not drain cluster when a host is added", func() {
			// Confirm that the cluster doesn't have a draining listeners
			Eventually(func(g Gomega) {
				dump, err := testContext.EnvoyInstance().StructuredConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				activeListeners, err := findListenersByState(dump, ActiveState)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(activeListeners).To(HaveLen(1))

				drainingListeners, err := findListenersByState(dump, DrainingState)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(drainingListeners).To(HaveLen(0))
			}, "10s", ".5s").Should(Succeed())

			err := addEnvoyInstance(testContext)
			Expect(err).NotTo(HaveOccurred())

			// Confirm the cluster doesn't have a draining listener and 2 active listeners
			Consistently(func(g Gomega) {
				dump, err := testContext.EnvoyInstance().StructuredConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				drainingListeners, err := findListenersByState(dump, DrainingState)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(drainingListeners).To(HaveLen(0))
			}, "10s", ".5s").Should(Succeed())
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

		It("should drain cluster when a host is added", func() {
			// Confirm that the cluster doesn't have a draining listeners
			Eventually(func(g Gomega) {
				dump, err := testContext.EnvoyInstance().StructuredConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				activeListeners, err := findListenersByState(dump, ActiveState)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(activeListeners).To(HaveLen(1))

				drainingListeners, err := findListenersByState(dump, DrainingState)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(drainingListeners).To(HaveLen(0))
			}, "10s", ".5s").Should(Succeed())

			err := addEnvoyInstance(testContext)
			Expect(err).NotTo(HaveOccurred())

			// Confirm the cluster drains the listener
			Eventually(func(g Gomega) {
				dump, err := testContext.EnvoyInstance().StructuredConfigDump()
				g.Expect(err).NotTo(HaveOccurred())

				drainingListeners, err := findListenersByState(dump, DrainingState)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(drainingListeners).To(HaveLen(1))
			}, "30s", ".5s").Should(Succeed())
		})
	})
})

func addEnvoyInstance(testContext *e2e.TestContext) error {
	// Start a new envoy instance
	testClients := testContext.TestClients()
	newEnvoyInstance := testContextFactory.EnvoyFactory.NewInstance()
	err := newEnvoyInstance.RunWith(envoy.RunConfig{
		Context:     testContext.Ctx(),
		Role:        fmt.Sprintf("%v~%v", e2e.WriteNamespace, e2e.DefaultProxyName),
		Port:        uint32(testClients.GlooPort),
		RestXdsPort: uint32(testClients.RestXdsPort),
	})
	if err != nil {
		return err
	}

	newUpstream := v1helpers.NewTestHttpUpstream(testContext.Ctx(), newEnvoyInstance.LocalAddr())
	if newUpstream == nil {
		return fmt.Errorf("failed to create new upstream")
	}

	// Add the new envoy instance to the virtual service
	testContext.PatchDefaultVirtualService(func(vs *v1.VirtualService) *v1.VirtualService {
		// Add the new upstream to the virtual service
		weight := &wrappers.UInt32Value{Value: 1}
		dests := &gloov1.MultiDestination{
			Destinations: []*gloov1.WeightedDestination{
				{
					Weight: weight,
					Destination: &gloov1.Destination{
						DestinationType: &gloov1.Destination_Upstream{
							Upstream: testContext.TestUpstream().Upstream.Metadata.Ref(),
						},
					},
				},
				{
					Weight: weight,
					Destination: &gloov1.Destination{
						DestinationType: &gloov1.Destination_Upstream{
							Upstream: newUpstream.Upstream.Metadata.Ref(),
						},
					},
				},
			},
		}

		vs.VirtualHost.Routes[0].Action = &v1.Route_RouteAction{
			RouteAction: &gloov1.RouteAction{
				Destination: &gloov1.RouteAction_Multi{
					Multi: dests,
				},
			},
		}

		return vs
	})

	return nil
}

type ListenerState int

const (
	ActiveState ListenerState = iota + 1
	WarmingState
	DrainingState
)

// findListenersByState finds the listeners in the config dump that are in the provided state
func findListenersByState(dump *envoy_admin_v3.ConfigDump,
	state ListenerState) ([]*envoy_admin_v3.ListenersConfigDump_DynamicListenerState, error) {

	listenerStates := []*envoy_admin_v3.ListenersConfigDump_DynamicListenerState{}

	var listeners []*envoy_admin_v3.ListenersConfigDump_DynamicListener
	for _, cfg := range dump.Configs {
		if cfg.TypeUrl == "type.googleapis.com/envoy.admin.v3.ListenersConfigDump" {
			configDump := &envoy_admin_v3.ListenersConfigDump{}
			err := cfg.UnmarshalTo(configDump)
			if err != nil {
				return nil, err
			}

			// fmt.Printf("configDump: %v\n", configDump)

			listeners = configDump.DynamicListeners
		}
	}

	if len(listeners) == 0 {
		return listenerStates, nil
	}

	for _, listenerDump := range listeners {
		switch state {
		case ActiveState:
			if listenerDump.ActiveState != nil {
				listenerStates = append(listenerStates, listenerDump.ActiveState)
			}
		case WarmingState:
			if listenerDump.WarmingState != nil {
				listenerStates = append(listenerStates, listenerDump.WarmingState)
			}
		case DrainingState:
			if listenerDump.DrainingState != nil {
				listenerStates = append(listenerStates, listenerDump.DrainingState)
			}
		default:
			return nil, fmt.Errorf("unknown listener state: %v", state)
		}
	}

	return listenerStates, nil
}

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
