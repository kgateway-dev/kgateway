package loadbalancer_test

import (
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/lbhash"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/loadbalancer"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ = Describe("Plugin", func() {

	var (
		params   plugins.Params
		plugin   plugins.UpstreamPlugin
		upstream *v1.Upstream
		out      *envoy_config_cluster_v3.Cluster
	)

	BeforeEach(func() {
		out = new(envoy_config_cluster_v3.Cluster)

		params = plugins.Params{}
		upstream = &v1.Upstream{}
		plugin = NewPlugin()
	})

	It("should set HealthyPanicThreshold", func() {

		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			HealthyPanicThreshold: &wrappers.DoubleValue{
				Value: 50,
			},
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.CommonLbConfig.HealthyPanicThreshold.Value).To(BeEquivalentTo(50))
	})

	It("should set UpdateMergeWindow", func() {
		t := prototime.DurationToProto(time.Second)
		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			UpdateMergeWindow: t,
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.CommonLbConfig.UpdateMergeWindow.Seconds).To(BeEquivalentTo(1))
		Expect(out.CommonLbConfig.UpdateMergeWindow.Nanos).To(BeEquivalentTo(0))
	})

	It("should set lb policy random", func() {
		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_Random_{
				Random: &v1.LoadBalancerConfig_Random{},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_RANDOM))
	})

	Context("p2c", func() {
		BeforeEach(func() {
			upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
				Type: &v1.LoadBalancerConfig_LeastRequest_{
					LeastRequest: &v1.LoadBalancerConfig_LeastRequest{ChoiceCount: 5},
				},
			}
		})
		It("should set lb policy p2c", func() {
			err := plugin.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_LEAST_REQUEST))
			Expect(out.GetLeastRequestLbConfig().ChoiceCount.Value).To(BeEquivalentTo(5))
		})
		It("should set lb policy p2c with default config", func() {
			upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
				Type: &v1.LoadBalancerConfig_LeastRequest_{
					LeastRequest: &v1.LoadBalancerConfig_LeastRequest{},
				},
			}

			err := plugin.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_LEAST_REQUEST))
			Expect(out.GetLeastRequestLbConfig()).To(BeNil())
		})
		It("should set lb policy p2c with full config", func() {
			upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
				Type: &v1.LoadBalancerConfig_LeastRequest_{
					LeastRequest: &v1.LoadBalancerConfig_LeastRequest{
						ChoiceCount: 5,
						SlowStartConfig: &v1.LoadBalancerConfig_SlowStartConfig{
							SlowStartWindow:  prototime.DurationToProto(time.Minute),
							Aggression:       &wrappers.DoubleValue{Value: 2},
							MinWeightPercent: &wrappers.DoubleValue{Value: 20},
						},
					},
				},
			}

			sampleUpstream := *upstream
			sampleInputResource := v1.UpstreamList{&sampleUpstream}.AsInputResources()[0]
			yamlForm, err := printers.GenerateKubeCrdString(sampleInputResource, v1.UpstreamCrd)
			Expect(err).NotTo(HaveOccurred())
			// sample user config
			sampleInputYaml := `apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  creationTimestamp: null
spec:
  loadBalancerConfig:
    leastRequest:
      choiceCount: 5
      slowStartConfig:
        aggression: 2
        minWeightPercent: 20
        slowStartWindow: 60s
status: {}
`
			Expect(yamlForm).To(Equal(sampleInputYaml))

			err = plugin.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_LEAST_REQUEST))
			Expect(out.GetLeastRequestLbConfig()).NotTo(BeNil())
		})
	})

	It("should set lb policy round robin - basic config", func() {
		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_RoundRobin_{
				RoundRobin: &v1.LoadBalancerConfig_RoundRobin{},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_ROUND_ROBIN))
		Expect(out.GetRoundRobinLbConfig()).To(BeNil())
	})

	It("should set lb policy round robin - full config", func() {
		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_RoundRobin_{
				RoundRobin: &v1.LoadBalancerConfig_RoundRobin{
					SlowStartConfig: &v1.LoadBalancerConfig_SlowStartConfig{
						SlowStartWindow:  prototime.DurationToProto(time.Hour),
						Aggression:       &wrappers.DoubleValue{Value: 2},
						MinWeightPercent: &wrappers.DoubleValue{Value: 20},
					},
				},
			},
		}

		sampleUpstream := *upstream
		sampleInputResource := v1.UpstreamList{&sampleUpstream}.AsInputResources()[0]
		yamlForm, err := printers.GenerateKubeCrdString(sampleInputResource, v1.UpstreamCrd)
		Expect(err).NotTo(HaveOccurred())
		// sample user config
		sampleInputYaml := `apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  creationTimestamp: null
spec:
  loadBalancerConfig:
    roundRobin:
      slowStartConfig:
        aggression: 2
        minWeightPercent: 20
        slowStartWindow: 3600s
status: {}
`
		Expect(yamlForm).To(Equal(sampleInputYaml))

		err = plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_ROUND_ROBIN))
		Expect(out.GetRoundRobinLbConfig()).NotTo(BeNil())
	})

	It("should set lb policy ring hash - basic config", func() {
		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_RingHash_{
				RingHash: &v1.LoadBalancerConfig_RingHash{},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_RING_HASH))
	})

	It("should set lb policy ring hash - full config", func() {
		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_RingHash_{
				RingHash: &v1.LoadBalancerConfig_RingHash{
					RingHashConfig: &v1.LoadBalancerConfig_RingHashConfig{
						MinimumRingSize: 100,
						MaximumRingSize: 200,
					},
				},
			},
		}
		sampleUpstream := *upstream
		sampleInputResource := v1.UpstreamList{&sampleUpstream}.AsInputResources()[0]
		yamlForm, err := printers.GenerateKubeCrdString(sampleInputResource, v1.UpstreamCrd)
		Expect(err).NotTo(HaveOccurred())
		// sample user config
		sampleInputYaml := `apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  creationTimestamp: null
spec:
  loadBalancerConfig:
    ringHash:
      ringHashConfig:
        maximumRingSize: "200"
        minimumRingSize: "100"
status: {}
`
		Expect(yamlForm).To(Equal(sampleInputYaml))
		err = plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_RING_HASH))
		Expect(out.LbConfig).To(Equal(&envoy_config_cluster_v3.Cluster_RingHashLbConfig_{
			RingHashLbConfig: &envoy_config_cluster_v3.Cluster_RingHashLbConfig{
				MinimumRingSize: &wrappers.UInt64Value{Value: 100},
				MaximumRingSize: &wrappers.UInt64Value{Value: 200},
				HashFunction:    envoy_config_cluster_v3.Cluster_RingHashLbConfig_XX_HASH,
			},
		}))
	})

	It("should set lb policy maglev - basic config", func() {
		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_Maglev_{
				Maglev: &v1.LoadBalancerConfig_Maglev{},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_MAGLEV))
	})

	It("should set lb policy maglev - full config", func() {
		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_Maglev_{
				Maglev: &v1.LoadBalancerConfig_Maglev{},
			},
		}
		sampleUpstream := *upstream
		sampleInputResource := v1.UpstreamList{&sampleUpstream}.AsInputResources()[0]
		yamlForm, err := printers.GenerateKubeCrdString(sampleInputResource, v1.UpstreamCrd)
		Expect(err).NotTo(HaveOccurred())
		// sample user config
		sampleInputYaml := `apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  creationTimestamp: null
spec:
  loadBalancerConfig:
    maglev: {}
status: {}
`
		Expect(yamlForm).To(Equal(sampleInputYaml))
		err = plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoy_config_cluster_v3.Cluster_MAGLEV))
		Expect(out.LbConfig).To(BeNil())
	})

	It("should set locality config - locality weighted lb config", func() {
		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			LocalityConfig: &v1.LoadBalancerConfig_LocalityWeightedLbConfig{
				LocalityWeightedLbConfig: &empty.Empty{},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.CommonLbConfig.LocalityConfigSpecifier).To(Equal(
			&envoy_config_cluster_v3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
				LocalityWeightedLbConfig: &envoy_config_cluster_v3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
			}))
	})

	It("should not set locality config if no config", func() {
		upstream.LoadBalancerConfig = &v1.LoadBalancerConfig{
			// We include this, so that the plugin generates a CommonLbConfig object
			HealthyPanicThreshold: &wrappers.DoubleValue{
				Value: 50,
			},
			LocalityConfig: nil,
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.CommonLbConfig.LocalityConfigSpecifier).To(BeNil())
	})

	Context("route plugin", func() {

		var (
			routeParams plugins.RouteParams
			routePlugin plugins.RoutePlugin
			route       *v1.Route
			outRoute    *envoy_config_route_v3.Route
		)

		BeforeEach(func() {
			outRoute = new(envoy_config_route_v3.Route)

			routeParams = plugins.RouteParams{}
			routePlugin = NewPlugin()
			route = &v1.Route{}

		})

		// positive cases
		It("configures routes - basic config", func() {
			route.Options = &v1.RouteOptions{
				LbHash: &lbhash.RouteActionHashConfig{
					HashPolicies: []*lbhash.HashPolicy{{
						KeyType:  &lbhash.HashPolicy_Header{Header: "origin"},
						Terminal: false,
					},
					},
				},
			}
			err := routePlugin.ProcessRoute(routeParams, route, outRoute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outRoute.GetRoute().HashPolicy).To(Equal([]*envoy_config_route_v3.RouteAction_HashPolicy{{
				PolicySpecifier: &envoy_config_route_v3.RouteAction_HashPolicy_Header_{
					Header: &envoy_config_route_v3.RouteAction_HashPolicy_Header{
						HeaderName: "origin",
					},
				},
				Terminal: false,
			}}))
		})
		It("configures routes - all types", func() {
			ttlDur := prototime.DurationToProto(time.Second)
			route.Options = &v1.RouteOptions{
				LbHash: &lbhash.RouteActionHashConfig{
					HashPolicies: []*lbhash.HashPolicy{
						{
							// users may choose to add a specialty terminal header such as this
							KeyType:  &lbhash.HashPolicy_Header{Header: "x-test-affinity"},
							Terminal: true,
						},
						{
							KeyType:  &lbhash.HashPolicy_Header{Header: "origin"},
							Terminal: false,
						},
						{
							KeyType:  &lbhash.HashPolicy_SourceIp{SourceIp: true},
							Terminal: false,
						},
						{
							KeyType: &lbhash.HashPolicy_Cookie{Cookie: &lbhash.Cookie{
								Name: "gloo",
								Ttl:  ttlDur,
								Path: "/abc",
							}},
							Terminal: false,
						},
					},
				},
			}
			sampleVirtualService := &gatewayv1.VirtualService{
				VirtualHost: &gatewayv1.VirtualHost{
					Routes: []*gatewayv1.Route{{Options: route.Options}},
				},
			}
			sampleInputResource := gatewayv1.VirtualServiceList{sampleVirtualService}.AsInputResources()[0]
			yamlForm, err := printers.GenerateKubeCrdString(sampleInputResource, gatewayv1.VirtualServiceCrd)
			Expect(err).NotTo(HaveOccurred())
			// sample user config
			sampleInputYaml := `apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: null
spec:
  virtualHost:
    routes:
    - options:
        lbHash:
          hashPolicies:
          - header: x-test-affinity
            terminal: true
          - header: origin
          - sourceIp: true
          - cookie:
              name: gloo
              path: /abc
              ttl: 1s
status: {}
`
			Expect(yamlForm).To(Equal(sampleInputYaml))
			err = routePlugin.ProcessRoute(routeParams, route, outRoute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outRoute.GetRoute().HashPolicy).To(Equal([]*envoy_config_route_v3.RouteAction_HashPolicy{
				{
					PolicySpecifier: &envoy_config_route_v3.RouteAction_HashPolicy_Header_{
						Header: &envoy_config_route_v3.RouteAction_HashPolicy_Header{
							HeaderName: "x-test-affinity",
						},
					},
					Terminal: true,
				},
				{
					PolicySpecifier: &envoy_config_route_v3.RouteAction_HashPolicy_Header_{
						Header: &envoy_config_route_v3.RouteAction_HashPolicy_Header{
							HeaderName: "origin",
						},
					},
					Terminal: false,
				},
				{
					PolicySpecifier: &envoy_config_route_v3.RouteAction_HashPolicy_ConnectionProperties_{
						ConnectionProperties: &envoy_config_route_v3.RouteAction_HashPolicy_ConnectionProperties{
							SourceIp: true,
						},
					},
					Terminal: false,
				},
				{
					PolicySpecifier: &envoy_config_route_v3.RouteAction_HashPolicy_Cookie_{
						Cookie: &envoy_config_route_v3.RouteAction_HashPolicy_Cookie{
							Name: "gloo",
							Ttl:  ttlDur,
							Path: "/abc",
						},
					},
					Terminal: false,
				},
			}))
		})
		// negative cases
		It("skips non-route-action routes", func() {
			outRoute.Action = &envoy_config_route_v3.Route_Redirect{}
			route.Action = &v1.Route_RedirectAction{}
			// the following represents a misconfigured route
			route.Options = &v1.RouteOptions{
				LbHash: &lbhash.RouteActionHashConfig{
					HashPolicies: []*lbhash.HashPolicy{{
						KeyType:  &lbhash.HashPolicy_Header{Header: "origin"},
						Terminal: false,
					},
					},
				},
			}
			err := routePlugin.ProcessRoute(routeParams, route, outRoute)
			Expect(err).To(HaveOccurred())
			Expect(outRoute.GetRoute()).To(BeNil())
		})
		It("skips routes that do not feature the plugin", func() {
			outRoute.Action = &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{},
			}
			route.Options = &v1.RouteOptions{}
			err := routePlugin.ProcessRoute(routeParams, route, outRoute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outRoute.GetRoute().HashPolicy).To(BeNil())
		})
	})
})
