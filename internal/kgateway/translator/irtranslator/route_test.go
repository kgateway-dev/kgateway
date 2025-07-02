package irtranslator

import (
	"testing"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/stretchr/testify/assert"
)

func TestValidateWeightedClusters(t *testing.T) {
	tests := []struct {
		name     string
		clusters []*envoy_config_route_v3.WeightedCluster_ClusterWeight
		wantErr  bool
	}{
		{
			name:     "no clusters",
			clusters: []*envoy_config_route_v3.WeightedCluster_ClusterWeight{},
			wantErr:  false,
		},
		{
			name: "single cluster with weight 0",
			clusters: []*envoy_config_route_v3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(0),
				},
			},
			wantErr: true,
		},
		{
			name: "single cluster with weight > 0",
			clusters: []*envoy_config_route_v3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(100),
				},
			},
			wantErr: false,
		},
		{
			name: "multiple clusters all with weight 0",
			clusters: []*envoy_config_route_v3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(0),
				},
				{
					Weight: wrapperspb.UInt32(0),
				},
			},
			wantErr: true,
		},
		{
			name: "multiple clusters with mixed weights",
			clusters: []*envoy_config_route_v3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(0),
				},
				{
					Weight: wrapperspb.UInt32(100),
				},
			},
			wantErr: false,
		},
		{
			name: "multiple clusters all with weight > 0",
			clusters: []*envoy_config_route_v3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(50),
				},
				{
					Weight: wrapperspb.UInt32(50),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errs []error
			validateWeightedClusters(tt.clusters, &errs)

			if tt.wantErr {
				assert.Len(t, errs, 1)
				assert.Contains(t, errs[0].Error(), "All backend weights are 0. At least one backendRef in the HTTPRoute rule must specify a non-zero weight")
			} else {
				assert.Len(t, errs, 0)
			}
		})
	}
}
