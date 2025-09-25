package backendconfigpolicy

import (
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/common/v3"
	leastrequestv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/least_request/v3"
	maglevv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/maglev/v3"
	randomv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/random/v3"
	ringhashv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/ring_hash/v3"
	roundrobinv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/round_robin/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestApplyLoadBalancerConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *v1alpha1.LoadBalancer
		cluster  *envoyclusterv3.Cluster
		expected *envoyclusterv3.Cluster
	}{
		{
			name: "HealthyPanicThreshold",
			config: &v1alpha1.LoadBalancer{
				HealthyPanicThreshold: ptr.To(int32(100)),
			},
			expected: &envoyclusterv3.Cluster{
				Name: "test",
				CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{
					HealthyPanicThreshold: &typev3.Percent{
						Value: 100,
					},
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
					UpdateMergeWindow: durationpb.New(10 * time.Second),
				},
			},
		},
		{
			name: "LoadBalancerTypeRandom",
			config: &v1alpha1.LoadBalancer{
				Random: &v1alpha1.LoadBalancerRandomConfig{},
			},
			expected: func() *envoyclusterv3.Cluster {
				msg, _ := utils.MessageToAny(&randomv3.Random{})
				return &envoyclusterv3.Cluster{
					Name: "test",
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.random",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
		},
		{
			name: "RoundRobin basic config",
			config: &v1alpha1.LoadBalancer{
				RoundRobin: &v1alpha1.LoadBalancerRoundRobinConfig{},
			},
			expected: func() *envoyclusterv3.Cluster {
				msg, _ := utils.MessageToAny(&roundrobinv3.RoundRobin{})
				return &envoyclusterv3.Cluster{
					Name: "test",
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.round_robin",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
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
						MinWeightPercent: ptr.To(int32(10)),
					},
				},
			},
			expected: func() *envoyclusterv3.Cluster {
				rr := &roundrobinv3.RoundRobin{
					SlowStartConfig: &envoycommonv3.SlowStartConfig{
						SlowStartWindow: durationpb.New(10 * time.Second),
						Aggression: &envoycorev3.RuntimeDouble{
							DefaultValue: 1.1,
							RuntimeKey:   "policy.default.slowStart.aggression",
						},
						MinWeightPercent: &typev3.Percent{Value: 10},
					},
				}
				msg, _ := utils.MessageToAny(rr)
				return &envoyclusterv3.Cluster{
					Name: "test",
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.round_robin",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
		},
		{
			name: "LeastRequest basic config",
			config: &v1alpha1.LoadBalancer{
				LeastRequest: &v1alpha1.LoadBalancerLeastRequestConfig{
					ChoiceCount: 3,
				},
			},
			expected: func() *envoyclusterv3.Cluster {
				lr := &leastrequestv3.LeastRequest{
					ChoiceCount: &wrapperspb.UInt32Value{Value: 3},
				}
				msg, _ := utils.MessageToAny(lr)
				return &envoyclusterv3.Cluster{
					Name: "test",
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.least_request",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
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
						MinWeightPercent: ptr.To(int32(10)),
					},
				},
			},
			expected: func() *envoyclusterv3.Cluster {
				lr := &leastrequestv3.LeastRequest{
					ChoiceCount: &wrapperspb.UInt32Value{Value: 10},
					SlowStartConfig: &envoycommonv3.SlowStartConfig{
						SlowStartWindow:  durationpb.New(10 * time.Second),
						Aggression:       &envoycorev3.RuntimeDouble{DefaultValue: 1.1, RuntimeKey: "policy.default.slowStart.aggression"},
						MinWeightPercent: &typev3.Percent{Value: 10},
					},
				}
				msg, _ := utils.MessageToAny(lr)
				return &envoyclusterv3.Cluster{
					Name: "test",
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.least_request",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
		},
		{
			name: "RingHash basic config",
			config: &v1alpha1.LoadBalancer{
				RingHash: &v1alpha1.LoadBalancerRingHashConfig{},
			},
			expected: func() *envoyclusterv3.Cluster {
				msg, _ := utils.MessageToAny(&ringhashv3.RingHash{})
				return &envoyclusterv3.Cluster{
					Name: "test",
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.ring_hash",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
		},
		{
			name: "RingHash full config",
			config: &v1alpha1.LoadBalancer{
				RingHash: &v1alpha1.LoadBalancerRingHashConfig{
					MinimumRingSize: ptr.To(int64(10)),
					MaximumRingSize: ptr.To(int64(100)),
				},
			},
			expected: func() *envoyclusterv3.Cluster {
				rh := &ringhashv3.RingHash{
					MinimumRingSize: &wrapperspb.UInt64Value{Value: 10},
					MaximumRingSize: &wrapperspb.UInt64Value{Value: 100},
				}
				msg, _ := utils.MessageToAny(rh)
				return &envoyclusterv3.Cluster{
					Name: "test",
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.ring_hash",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
		},
		{
			name: "RingHash with hash policies",
			config: &v1alpha1.LoadBalancer{
				RingHash: &v1alpha1.LoadBalancerRingHashConfig{
					HashPolicies: []*v1alpha1.HashPolicy{
						{
							Header: &v1alpha1.Header{
								Name: "x-user-id",
							},
						},
						{
							Cookie: &v1alpha1.Cookie{
								Name: "session-id",
							},
						},
					},
				},
			},
			expected: func() *envoyclusterv3.Cluster {
				msg, _ := utils.MessageToAny(&ringhashv3.RingHash{
					ConsistentHashingLbConfig: &envoycommonv3.ConsistentHashingLbConfig{
						HashPolicy: constructHashPolicy([]*v1alpha1.HashPolicy{
							{
								Header: &v1alpha1.Header{
									Name: "x-user-id",
								},
							},
							{
								Cookie: &v1alpha1.Cookie{
									Name: "session-id",
								},
							},
						}),
					},
				})
				return &envoyclusterv3.Cluster{
					Name: "test",
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.ring_hash",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
		},
		{
			name: "Maglev with hash policies",
			config: &v1alpha1.LoadBalancer{
				Maglev: &v1alpha1.LoadBalancerMaglevConfig{
					HashPolicies: []*v1alpha1.HashPolicy{
						{
							Header: &v1alpha1.Header{
								Name: "x-user-id",
							},
						},
						{
							Cookie: &v1alpha1.Cookie{
								Name: "session-id",
							},
						},
					},
				},
			},
			expected: func() *envoyclusterv3.Cluster {
				msg, _ := utils.MessageToAny(&maglevv3.Maglev{
					ConsistentHashingLbConfig: &envoycommonv3.ConsistentHashingLbConfig{
						HashPolicy: constructHashPolicy([]*v1alpha1.HashPolicy{
							{
								Header: &v1alpha1.Header{
									Name: "x-user-id",
								},
							},
							{
								Cookie: &v1alpha1.Cookie{
									Name: "session-id",
								},
							},
						}),
					},
				})
				return &envoyclusterv3.Cluster{
					Name: "test",
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.maglev",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
		},
		{
			name: "Maglev",
			config: &v1alpha1.LoadBalancer{
				Maglev: &v1alpha1.LoadBalancerMaglevConfig{},
			},
			expected: func() *envoyclusterv3.Cluster {
				msg, _ := utils.MessageToAny(&maglevv3.Maglev{})
				return &envoyclusterv3.Cluster{
					Name: "test",
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.maglev",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
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
				},
			},
		},
		{
			name: "RingHash: UseHostnameForHashing, STRICT_DNS",
			config: &v1alpha1.LoadBalancer{
				RingHash: &v1alpha1.LoadBalancerRingHashConfig{
					UseHostnameForHashing: ptr.To(true),
				},
			},
			cluster: &envoyclusterv3.Cluster{
				ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
					Type: envoyclusterv3.Cluster_STRICT_DNS,
				},
			},
			expected: func() *envoyclusterv3.Cluster {
				msg, _ := utils.MessageToAny(&ringhashv3.RingHash{
					ConsistentHashingLbConfig: &envoycommonv3.ConsistentHashingLbConfig{
						UseHostnameForHashing: true,
					},
				})
				return &envoyclusterv3.Cluster{
					Name:                 "test",
					ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS},
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.ring_hash",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
		},
		{
			name: "Ringhash: UseHostnameForHashing, EDS",
			config: &v1alpha1.LoadBalancer{
				RingHash: &v1alpha1.LoadBalancerRingHashConfig{
					UseHostnameForHashing: ptr.To(true),
				},
			},
			cluster: &envoyclusterv3.Cluster{
				ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
					Type: envoyclusterv3.Cluster_EDS,
				},
			},
			expected: func() *envoyclusterv3.Cluster {
				msg, _ := utils.MessageToAny(&ringhashv3.RingHash{
					ConsistentHashingLbConfig: &envoycommonv3.ConsistentHashingLbConfig{
						UseHostnameForHashing: false,
					},
				})
				return &envoyclusterv3.Cluster{
					Name:                 "test",
					ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
					LoadBalancingPolicy: &envoyclusterv3.LoadBalancingPolicy{
						Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
							TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
								Name:        "envoy.load_balancing_policies.ring_hash",
								TypedConfig: msg,
							},
						}},
					},
					CommonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
				}
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cluster := test.cluster
			if cluster == nil {
				cluster = &envoyclusterv3.Cluster{}
			}
			cluster.Name = "test"
			lbConfig, err := translateLoadBalancerConfig(test.config, "policy", "default")
			if err != nil {
				t.Fatalf("failed to translate load balancer config: %v", err)
			}
			applyLoadBalancerConfig(lbConfig, cluster)
			if !proto.Equal(cluster, test.expected) {
				t.Errorf("expected %v, got %v", test.expected, cluster)
			}
		})
	}
}

func TestConstructHashPolicy(t *testing.T) {
	tests := []struct {
		name         string
		hashPolicies []*v1alpha1.HashPolicy
		expected     []*envoyroutev3.RouteAction_HashPolicy
	}{
		{
			name:         "nil hash policies",
			hashPolicies: nil,
			expected:     nil,
		},
		{
			name:         "empty hash policies",
			hashPolicies: []*v1alpha1.HashPolicy{},
			expected:     nil,
		},
		{
			name: "header hash policy",
			hashPolicies: []*v1alpha1.HashPolicy{
				{
					Header: &v1alpha1.Header{
						Name: "x-user-id",
					},
					Terminal: ptr.To(true),
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: true,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Header_{
						Header: &envoyroutev3.RouteAction_HashPolicy_Header{
							HeaderName: "x-user-id",
						},
					},
				},
			},
		},
		{
			name: "cookie hash policy without TTL and path",
			hashPolicies: []*v1alpha1.HashPolicy{
				{
					Cookie: &v1alpha1.Cookie{
						Name: "session-id",
					},
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Cookie_{
						Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
							Name: "session-id",
						},
					},
				},
			},
		},
		{
			name: "cookie hash policy with TTL and path",
			hashPolicies: []*v1alpha1.HashPolicy{
				{
					Cookie: &v1alpha1.Cookie{
						Name: "session-id",
						TTL: &metav1.Duration{
							Duration: 30 * time.Minute,
						},
						Path: ptr.To("/api"),
					},
					Terminal: ptr.To(true),
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: true,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Cookie_{
						Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
							Name: "session-id",
							Ttl:  durationpb.New(30 * time.Minute),
							Path: "/api",
						},
					},
				},
			},
		},
		{
			name: "cookie hash policy with all attributes",
			hashPolicies: []*v1alpha1.HashPolicy{
				{
					Cookie: &v1alpha1.Cookie{
						Name:     "session-id",
						Secure:   ptr.To(true),
						HttpOnly: ptr.To(true),
						SameSite: ptr.To("Strict"),
					},
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Cookie_{
						Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
							Name: "session-id",
							Attributes: []*envoyroutev3.RouteAction_HashPolicy_CookieAttribute{
								{Name: "Secure", Value: "true"},
								{Name: "HttpOnly", Value: "true"},
								{Name: "SameSite", Value: "Strict"},
							},
						},
					},
				},
			},
		},
		{
			name: "source IP hash policy",
			hashPolicies: []*v1alpha1.HashPolicy{
				{
					SourceIP: &v1alpha1.SourceIP{},
					Terminal: ptr.To(false),
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties_{
						ConnectionProperties: &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties{
							SourceIp: true,
						},
					},
				},
			},
		},
		{
			name: "multiple hash policies",
			hashPolicies: []*v1alpha1.HashPolicy{
				{
					Header: &v1alpha1.Header{
						Name: "x-user-id",
					},
					Terminal: ptr.To(true),
				},
				{
					Cookie: &v1alpha1.Cookie{
						Name: "session-id",
						TTL: &metav1.Duration{
							Duration: 1 * time.Hour,
						},
					},
				},
				{
					SourceIP: &v1alpha1.SourceIP{},
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: true,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Header_{
						Header: &envoyroutev3.RouteAction_HashPolicy_Header{
							HeaderName: "x-user-id",
						},
					},
				},
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Cookie_{
						Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
							Name: "session-id",
							Ttl:  durationpb.New(1 * time.Hour),
						},
					},
				},
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties_{
						ConnectionProperties: &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties{
							SourceIp: true,
						},
					},
				},
			},
		},
		{
			name: "cookie hash policy with nil TTL",
			hashPolicies: []*v1alpha1.HashPolicy{
				{
					Cookie: &v1alpha1.Cookie{
						Name: "session-id",
						TTL:  nil,
						Path: ptr.To("/api"),
					},
					Terminal: ptr.To(false),
				},
			},
			expected: []*envoyroutev3.RouteAction_HashPolicy{
				{
					Terminal: false,
					PolicySpecifier: &envoyroutev3.RouteAction_HashPolicy_Cookie_{
						Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
							Name: "session-id",
							Path: "/api",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := constructHashPolicy(tt.hashPolicies)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
