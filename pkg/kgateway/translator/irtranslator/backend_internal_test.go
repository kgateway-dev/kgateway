package irtranslator

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/require"
)

func TestDefaultLocalityConfig(t *testing.T) {
	newEdsCluster := func() *envoyclusterv3.Cluster {
		return &envoyclusterv3.Cluster{
			Name:                 "test-eds",
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
			EdsClusterConfig:     &envoyclusterv3.Cluster_EdsClusterConfig{},
		}
	}

	tests := []struct {
		name   string
		input  *envoyclusterv3.Cluster
		assert func(t *testing.T, c *envoyclusterv3.Cluster)
	}{
		{
			name:  "EDS cluster without locality config gets zone-aware routing disabled",
			input: newEdsCluster(),
			assert: func(t *testing.T, c *envoyclusterv3.Cluster) {
				zoneAware := c.GetCommonLbConfig().GetZoneAwareLbConfig()
				require.NotNil(t, zoneAware.GetRoutingEnabled(),
					"EDS cluster should receive an explicit routing_enabled to disable implicit zone-aware routing")
				require.Zero(t, zoneAware.GetRoutingEnabled().GetValue(),
					"implicit zone-aware routing must be disabled")
				require.Nil(t, c.GetCommonLbConfig().GetLocalityWeightedLbConfig(),
					"locality-weighted LB drops endpoints without a locality, so it must not be the default")
			},
		},
		{
			name: "non-EDS cluster is left untouched",
			input: &envoyclusterv3.Cluster{
				Name:                 "test-static",
				ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC},
			},
			assert: func(t *testing.T, c *envoyclusterv3.Cluster) {
				require.Nil(t, c.GetCommonLbConfig().GetLocalityConfigSpecifier(),
					"non-EDS clusters should not receive locality defaulting")
			},
		},
		{
			name: "typed load balancing policy is not modified",
			input: func() *envoyclusterv3.Cluster {
				c := newEdsCluster()
				c.LoadBalancingPolicy = &envoyclusterv3.LoadBalancingPolicy{
					Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
						TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
							Name: "envoy.load_balancing_policies.round_robin",
						},
					}},
				}
				return c
			}(),
			assert: func(t *testing.T, c *envoyclusterv3.Cluster) {
				require.Nil(t, c.GetCommonLbConfig().GetLocalityConfigSpecifier(),
					"a typed LB policy owns its locality config; common_lb_config must stay empty")
			},
		},
		{
			name: "existing locality specifier is preserved",
			input: func() *envoyclusterv3.Cluster {
				c := newEdsCluster()
				c.CommonLbConfig = &envoyclusterv3.Cluster_CommonLbConfig{
					LocalityConfigSpecifier: &envoyclusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
						LocalityWeightedLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
					},
				}
				return c
			}(),
			assert: func(t *testing.T, c *envoyclusterv3.Cluster) {
				require.NotNil(t, c.GetCommonLbConfig().GetLocalityWeightedLbConfig(),
					"a locality mode chosen by a policy plugin must not be overwritten")
				require.Nil(t, c.GetCommonLbConfig().GetZoneAwareLbConfig())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defaultLocalityConfig(tc.input)
			tc.assert(t, tc.input)
		})
	}
}
