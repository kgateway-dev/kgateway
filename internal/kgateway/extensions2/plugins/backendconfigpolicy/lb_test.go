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
		config   *v1alpha1.LoadBalancerConfig
		expected *clusterv3.Cluster
	}{
		{
			name: "HealthyPanicThreshold",
			config: &v1alpha1.LoadBalancerConfig{
				HealthyPanicThreshold: ptr.To(uint32(100)),
			},
			expected: &clusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &typev3.Percent{
						Value: 100,
					},
				},
			},
		},
		{
			name: "UpdateMergeWindow",
			config: &v1alpha1.LoadBalancerConfig{
				UpdateMergeWindow: &metav1.Duration{
					Duration: 10 * time.Second,
				},
			},
			expected: &clusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					UpdateMergeWindow: durationpb.New(10 * time.Second),
				},
			},
		},
		{
			name: "LoadBalancerTypeRandom",
			config: &v1alpha1.LoadBalancerConfig{
				Type: ptr.To(v1alpha1.LoadBalancerTypeRandom),
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_RANDOM,
			},
		},
		{
			name: "RoundRobin basic config",
			config: &v1alpha1.LoadBalancerConfig{
				Type: ptr.To(v1alpha1.LoadBalancerTypeRoundRobin),
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_ROUND_ROBIN,
			},
		},
		{
			name: "RoundRobin full config",
			config: &v1alpha1.LoadBalancerConfig{
				Type: ptr.To(v1alpha1.LoadBalancerTypeRoundRobin),
				RoundRobin: &v1alpha1.LoadBalancerRoundRobinConfig{
					SlowStartConfig: &v1alpha1.SlowStartConfig{
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
			},
		},
		{
			name: "LeastRequest basic config",
			config: &v1alpha1.LoadBalancerConfig{
				Type: ptr.To(v1alpha1.LoadBalancerTypeLeastRequest),
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_LEAST_REQUEST,
			},
		},
		{
			name: "LeastRequest full config",
			config: &v1alpha1.LoadBalancerConfig{
				Type: ptr.To(v1alpha1.LoadBalancerTypeLeastRequest),
				LeastRequest: &v1alpha1.LoadBalancerLeastRequestConfig{
					ChoiceCount: ptr.To(uint32(10)),
					SlowStartConfig: &v1alpha1.SlowStartConfig{
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
			},
		},
		{
			name: "RingHash basic config",
			config: &v1alpha1.LoadBalancerConfig{
				Type: ptr.To(v1alpha1.LoadBalancerTypeRingHash),
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_RING_HASH,
				LbConfig: &clusterv3.Cluster_RingHashLbConfig_{
					RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{},
				},
			},
		},
		{
			name: "RingHash full config",
			config: &v1alpha1.LoadBalancerConfig{
				Type: ptr.To(v1alpha1.LoadBalancerTypeRingHash),
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
			},
		},
		{
			name: "Maglev",
			config: &v1alpha1.LoadBalancerConfig{
				Type: ptr.To(v1alpha1.LoadBalancerTypeMaglev),
			},
			expected: &clusterv3.Cluster{
				Name:     "test",
				LbPolicy: clusterv3.Cluster_MAGLEV,
			},
		},
		{
			name: "LocalityWeightedLb",
			config: &v1alpha1.LoadBalancerConfig{
				LocalityConfigType: ptr.To(v1alpha1.LocalityConfigTypeWeightedLb),
			},
			expected: &clusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					LocalityConfigSpecifier: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
						LocalityWeightedLbConfig: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
					},
				},
			},
		},
		{
			name: "CloseConnectionsOnHostSetChange",
			config: &v1alpha1.LoadBalancerConfig{
				CloseConnectionsOnHostSetChange: ptr.To(true),
			},
			expected: &clusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
					CloseConnectionsOnHostSetChange: true,
				},
			},
		},
		{
			name: "UseHostnameForHashing",
			config: &v1alpha1.LoadBalancerConfig{
				UseHostnameForHashing: ptr.To(true),
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
			applyLoadBalancerConfig(test.config, cluster)
			if !proto.Equal(cluster, test.expected) {
				t.Errorf("expected %v, got %v", test.expected, cluster)
			}
		})
	}
}
