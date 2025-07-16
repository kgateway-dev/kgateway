package backendconfigpolicy

import (
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestApplyLoadBalancerConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *v1alpha1.LoadBalancer
		expected *envoyclusterv3.Cluster
	}{
		{
			name: "HealthyPanicThreshold",
			config: &v1alpha1.LoadBalancer{
				HealthyPanicThreshold: ptr.To(uint32(100)),
			},
			expected: &envoyclusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &typev3.Percent{
						Value: 100,
					},
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "UpdateMergeWindow",
			config: &v1alpha1.LoadBalancer{
				UpdateMergeWindow: &metav1.Duration{
					Duration: 10 * time.Second,
				},
			},
			expected: &envoyclusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					UpdateMergeWindow:         durationpb.New(10 * time.Second),
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "LoadBalancerTypeRandom",
			config: &v1alpha1.LoadBalancer{
				Random: &v1alpha1.LoadBalancerRandomConfig{},
			},
			expected: &envoyclusterv3.Cluster{
				Name:     "test",
				LbPolicy: envoyclusterv3.Cluster_RANDOM,
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "RoundRobin basic config",
			config: &v1alpha1.LoadBalancer{
				RoundRobin: &v1alpha1.LoadBalancerRoundRobinConfig{},
			},
			expected: &envoyclusterv3.Cluster{
				Name:     "test",
				LbPolicy: envoyclusterv3.Cluster_ROUND_ROBIN,
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "RoundRobin full config",
			config: &v1alpha1.LoadBalancer{
				RoundRobin: &v1alpha1.LoadBalancerRoundRobinConfig{
					SlowStart: &v1alpha1.SlowStart{
						Window: &metav1.Duration{
							Duration: 10 * time.Second,
						},
						Aggression:       ptr.To("1.1"),
						MinWeightPercent: ptr.To(uint32(10)),
					},
				},
			},
			expected: &envoyclusterv3.Cluster{
				Name:     "test",
				LbPolicy: envoyclusterv3.Cluster_ROUND_ROBIN,
				LbConfig: &envoyclusterv3.Cluster_RoundRobinLbConfig_{
					RoundRobinLbConfig: &envoyclusterv3.Cluster_RoundRobinLbConfig{
						SlowStartConfig: &envoyclusterv3.Cluster_SlowStartConfig{
							SlowStartWindow: durationpb.New(10 * time.Second),
							Aggression: &envoycorev3.RuntimeDouble{
								DefaultValue: 1.1,
								RuntimeKey:   "upstream.test.slowStart.aggression",
							},
							MinWeightPercent: &typev3.Percent{
								Value: 10,
							},
						},
					},
				},
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "LeastRequest basic config",
			config: &v1alpha1.LoadBalancer{
				LeastRequest: &v1alpha1.LoadBalancerLeastRequestConfig{},
			},
			expected: &envoyclusterv3.Cluster{
				Name:     "test",
				LbPolicy: envoyclusterv3.Cluster_LEAST_REQUEST,
				LbConfig: &envoyclusterv3.Cluster_LeastRequestLbConfig_{
					LeastRequestLbConfig: &envoyclusterv3.Cluster_LeastRequestLbConfig{
						ChoiceCount: &wrapperspb.UInt32Value{},
					},
				},
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "LeastRequest full config",
			config: &v1alpha1.LoadBalancer{
				LeastRequest: &v1alpha1.LoadBalancerLeastRequestConfig{
					ChoiceCount: 10,
					SlowStart: &v1alpha1.SlowStart{
						Window: &metav1.Duration{
							Duration: 10 * time.Second,
						},
						Aggression:       ptr.To("1.1"),
						MinWeightPercent: ptr.To(uint32(10)),
					},
				},
			},
			expected: &envoyclusterv3.Cluster{
				Name:     "test",
				LbPolicy: envoyclusterv3.Cluster_LEAST_REQUEST,
				LbConfig: &envoyclusterv3.Cluster_LeastRequestLbConfig_{
					LeastRequestLbConfig: &envoyclusterv3.Cluster_LeastRequestLbConfig{
						ChoiceCount: &wrapperspb.UInt32Value{Value: 10},
						SlowStartConfig: &envoyclusterv3.Cluster_SlowStartConfig{
							SlowStartWindow: durationpb.New(10 * time.Second),
							Aggression: &envoycorev3.RuntimeDouble{
								DefaultValue: 1.1,
								RuntimeKey:   "upstream.test.slowStart.aggression",
							},
							MinWeightPercent: &typev3.Percent{
								Value: 10,
							},
						},
					},
				},
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "RingHash basic config",
			config: &v1alpha1.LoadBalancer{
				RingHash: &v1alpha1.LoadBalancerRingHashConfig{},
			},
			expected: &envoyclusterv3.Cluster{
				Name:     "test",
				LbPolicy: envoyclusterv3.Cluster_RING_HASH,
				LbConfig: &envoyclusterv3.Cluster_RingHashLbConfig_{
					RingHashLbConfig: &envoyclusterv3.Cluster_RingHashLbConfig{},
				},
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "RingHash full config",
			config: &v1alpha1.LoadBalancer{
				RingHash: &v1alpha1.LoadBalancerRingHashConfig{
					MinimumRingSize: ptr.To(uint64(10)),
					MaximumRingSize: ptr.To(uint64(100)),
				},
			},
			expected: &envoyclusterv3.Cluster{
				Name:     "test",
				LbPolicy: envoyclusterv3.Cluster_RING_HASH,
				LbConfig: &envoyclusterv3.Cluster_RingHashLbConfig_{
					RingHashLbConfig: &envoyclusterv3.Cluster_RingHashLbConfig{
						MinimumRingSize: &wrapperspb.UInt64Value{Value: 10},
						MaximumRingSize: &wrapperspb.UInt64Value{Value: 100},
					},
				},
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "Maglev",
			config: &v1alpha1.LoadBalancer{
				Maglev: &v1alpha1.LoadBalancerMaglevConfig{},
			},
			expected: &envoyclusterv3.Cluster{
				Name:     "test",
				LbPolicy: envoyclusterv3.Cluster_MAGLEV,
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "LocalityWeightedLb",
			config: &v1alpha1.LoadBalancer{
				LocalityType: ptr.To(v1alpha1.LocalityConfigTypeWeightedLb),
			},
			expected: &envoyclusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					LocalityConfigSpecifier: &envoyclusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
						LocalityWeightedLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
					},
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "CloseConnectionsOnHostSetChange",
			config: &v1alpha1.LoadBalancer{
				CloseConnectionsOnHostSetChange: ptr.To(true),
			},
			expected: &envoyclusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					CloseConnectionsOnHostSetChange: true,
					ConsistentHashingLbConfig:       &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "UseHostnameForHashing",
			config: &v1alpha1.LoadBalancer{
				UseHostnameForHashing: true,
			},
			expected: &envoyclusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{
						UseHostnameForHashing: true,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cluster := &envoyclusterv3.Cluster{}
			cluster.Name = "test"
			lbConfig := translateLoadBalancerConfig(test.config)
			applyLoadBalancerConfig(lbConfig, cluster)
			if !proto.Equal(cluster, test.expected) {
				t.Errorf("expected %v, got %v", test.expected, cluster)
			}
		})
	}
}
