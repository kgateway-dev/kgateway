package backendconfigpolicy

import (
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
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
		expected *clusterv3.Cluster
	}{
		{
			name: "HealthyPanicThreshold",
			config: &v1alpha1.LoadBalancer{
				HealthyPanicThreshold: ptr.To(uint32(100)),
			},
			expected: &clusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &typev3.Percent{
						Value: 100,
					},
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
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
			expected: &clusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					UpdateMergeWindow:         durationpb.New(10 * time.Second),
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "LoadBalancerTypeRandom",
			config: &v1alpha1.LoadBalancer{
				Random: &v1alpha1.LoadBalancerRandomConfig{},
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_RANDOM,
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "RoundRobin basic config",
			config: &v1alpha1.LoadBalancer{
				RoundRobin: &v1alpha1.LoadBalancerRoundRobinConfig{},
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_ROUND_ROBIN,
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
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
						Aggression:       "1.1",
						MinWeightPercent: ptr.To(uint32(10)),
					},
				},
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_ROUND_ROBIN,
				LbConfig: &clusterv3.Cluster_RoundRobinLbConfig_{
					RoundRobinLbConfig: &clusterv3.Cluster_RoundRobinLbConfig{
						SlowStartConfig: &clusterv3.Cluster_SlowStartConfig{
							SlowStartWindow: durationpb.New(10 * time.Second),
							Aggression: &corev3.RuntimeDouble{
								DefaultValue: 1.1,
								RuntimeKey:   "upstream.test.slowStart.aggression",
							},
							MinWeightPercent: &typev3.Percent{
								Value: 10,
							},
						},
					},
				},
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "LeastRequest basic config",
			config: &v1alpha1.LoadBalancer{
				LeastRequest: &v1alpha1.LoadBalancerLeastRequestConfig{},
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_LEAST_REQUEST,
				LbConfig: &clusterv3.Cluster_LeastRequestLbConfig_{
					LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{
						ChoiceCount: &wrapperspb.UInt32Value{},
					},
				},
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
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
						Aggression:       "1.1",
						MinWeightPercent: ptr.To(uint32(10)),
					},
				},
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_LEAST_REQUEST,
				LbConfig: &clusterv3.Cluster_LeastRequestLbConfig_{
					LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{
						ChoiceCount: &wrapperspb.UInt32Value{Value: 10},
						SlowStartConfig: &clusterv3.Cluster_SlowStartConfig{
							SlowStartWindow: durationpb.New(10 * time.Second),
							Aggression: &corev3.RuntimeDouble{
								DefaultValue: 1.1,
								RuntimeKey:   "upstream.test.slowStart.aggression",
							},
							MinWeightPercent: &typev3.Percent{
								Value: 10,
							},
						},
					},
				},
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "RingHash basic config",
			config: &v1alpha1.LoadBalancer{
				RingHash: &v1alpha1.LoadBalancerRingHashConfig{},
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_RING_HASH,
				LbConfig: &clusterv3.Cluster_RingHashLbConfig_{
					RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{},
				},
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
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
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_RING_HASH,
				LbConfig: &clusterv3.Cluster_RingHashLbConfig_{
					RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
						MinimumRingSize: &wrapperspb.UInt64Value{Value: 10},
						MaximumRingSize: &wrapperspb.UInt64Value{Value: 100},
					},
				},
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "Maglev",
			config: &v1alpha1.LoadBalancer{
				Maglev: &v1alpha1.LoadBalancerMaglevConfig{},
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_MAGLEV,
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "LocalityWeightedLb",
			config: &v1alpha1.LoadBalancer{
				LocalityType: ptr.To(v1alpha1.LocalityConfigTypeWeightedLb),
			},
			expected: &clusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					LocalityConfigSpecifier: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
						LocalityWeightedLbConfig: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
					},
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "CloseConnectionsOnHostSetChange",
			config: &v1alpha1.LoadBalancer{
				CloseConnectionsOnHostSetChange: ptr.To(true),
			},
			expected: &clusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					CloseConnectionsOnHostSetChange: true,
					ConsistentHashingLbConfig:       &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{},
				},
			},
		},
		{
			name: "UseHostnameForHashing",
			config: &v1alpha1.LoadBalancer{
				UseHostnameForHashing: true,
			},
			expected: &clusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					ConsistentHashingLbConfig: &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{
						UseHostnameForHashing: true,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cluster := &clusterv3.Cluster{}
			cluster.Name = "test"
			lbConfig := translateLoadBalancerConfig(test.config)
			applyLoadBalancerConfig(lbConfig, cluster)
			if !proto.Equal(cluster, test.expected) {
				t.Errorf("expected %v, got %v", test.expected, cluster)
			}
		})
	}
}
