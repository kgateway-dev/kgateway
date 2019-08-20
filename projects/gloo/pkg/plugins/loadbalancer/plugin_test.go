package loadbalancer_test

import (
	"time"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	types "github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/loadbalancer"
)

var _ = Describe("Plugin", func() {

	var (
		params       plugins.Params
		plugin       *Plugin
		upstream     *v1.Upstream
		upstreamSpec *v1.UpstreamSpec
		out          *envoyapi.Cluster
	)
	BeforeEach(func() {
		out = new(envoyapi.Cluster)

		params = plugins.Params{}
		upstreamSpec = &v1.UpstreamSpec{}
		upstream = &v1.Upstream{
			UpstreamSpec: upstreamSpec,
		}
		plugin = NewPlugin()
	})

	It("should set HealthyPanicThreshold", func() {

		upstreamSpec.LoadBalancerConfig = &v1.LoadBalancerConfig{
			HealthyPanicThreshold: &types.DoubleValue{
				Value: 50,
			},
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.CommonLbConfig.HealthyPanicThreshold.Value).To(BeEquivalentTo(50))
	})

	It("should set UpdateMergeWindow", func() {
		t := time.Second
		upstreamSpec.LoadBalancerConfig = &v1.LoadBalancerConfig{
			UpdateMergeWindow: &t,
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.CommonLbConfig.UpdateMergeWindow.Seconds).To(BeEquivalentTo(1))
		Expect(out.CommonLbConfig.UpdateMergeWindow.Nanos).To(BeEquivalentTo(0))
	})

	It("should set lb policy random", func() {
		upstreamSpec.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_Random_{
				Random: &v1.LoadBalancerConfig_Random{},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoyapi.Cluster_RANDOM))
	})
	Context("p2c", func() {
		BeforeEach(func() {
			upstreamSpec.LoadBalancerConfig = &v1.LoadBalancerConfig{
				Type: &v1.LoadBalancerConfig_LeastRequest_{
					LeastRequest: &v1.LoadBalancerConfig_LeastRequest{ChoiceCount: 5},
				},
			}
		})
		It("should set lb policy p2c", func() {
			err := plugin.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.LbPolicy).To(Equal(envoyapi.Cluster_LEAST_REQUEST))
			Expect(out.GetLeastRequestLbConfig().ChoiceCount.Value).To(BeEquivalentTo(5))
		})
		It("should set lb policy p2c with default config", func() {

			upstreamSpec.LoadBalancerConfig = &v1.LoadBalancerConfig{
				Type: &v1.LoadBalancerConfig_LeastRequest_{
					LeastRequest: &v1.LoadBalancerConfig_LeastRequest{},
				},
			}

			err := plugin.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.LbPolicy).To(Equal(envoyapi.Cluster_LEAST_REQUEST))
			Expect(out.GetLeastRequestLbConfig()).To(BeNil())
		})
	})

	It("should set lb policy round robin", func() {
		upstreamSpec.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_RoundRobin_{
				RoundRobin: &v1.LoadBalancerConfig_RoundRobin{},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoyapi.Cluster_ROUND_ROBIN))
	})

	It("should set lb policy ring hash - basic config", func() {
		upstreamSpec.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_RingHash_{
				RingHash: &v1.LoadBalancerConfig_RingHash{},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoyapi.Cluster_RING_HASH))
	})

	It("should set lb policy ring hash - full config", func() {
		upstreamSpec.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_RingHash_{
				RingHash: &v1.LoadBalancerConfig_RingHash{
					RingHashConfig: &v1.LoadBalancerConfig_RingHashConfig{
						MinimumRingSize: 100,
						MaximumRingSize: 200,
					},
				},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoyapi.Cluster_RING_HASH))
		Expect(out.LbConfig).To(Equal(&envoyapi.Cluster_RingHashLbConfig_{
			RingHashLbConfig: &envoyapi.Cluster_RingHashLbConfig{
				MinimumRingSize: &types.UInt64Value{Value: 100},
				MaximumRingSize: &types.UInt64Value{Value: 200},
				HashFunction:    envoyapi.Cluster_RingHashLbConfig_XX_HASH,
			},
		}))
	})

	It("should set lb policy maglev - basic config", func() {
		upstreamSpec.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_Maglev_{
				Maglev: &v1.LoadBalancerConfig_Maglev{},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoyapi.Cluster_MAGLEV))
	})

	// maglev is a drop in replacement for ring hash, uses same config
	It("should set lb policy maglev - full config", func() {
		upstreamSpec.LoadBalancerConfig = &v1.LoadBalancerConfig{
			Type: &v1.LoadBalancerConfig_Maglev_{
				Maglev: &v1.LoadBalancerConfig_Maglev{
					RingHashConfig: &v1.LoadBalancerConfig_RingHashConfig{
						MinimumRingSize: 100,
						MaximumRingSize: 200,
					},
				},
			},
		}
		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.LbPolicy).To(Equal(envoyapi.Cluster_MAGLEV))
		Expect(out.LbConfig).To(Equal(&envoyapi.Cluster_RingHashLbConfig_{
			RingHashLbConfig: &envoyapi.Cluster_RingHashLbConfig{
				MinimumRingSize: &types.UInt64Value{Value: 100},
				MaximumRingSize: &types.UInt64Value{Value: 200},
				HashFunction:    envoyapi.Cluster_RingHashLbConfig_XX_HASH,
			},
		}))
	})

})
