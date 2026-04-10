package irtranslator

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/wrapperspb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestValidateWeightedClusters(t *testing.T) {
	tests := []struct {
		name     string
		clusters []*envoyroutev3.WeightedCluster_ClusterWeight
		wantErr  bool
	}{
		{
			name:     "no clusters",
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{},
			wantErr:  false,
		},
		{
			name: "single cluster with weight 0",
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(0),
				},
			},
			wantErr: true,
		},
		{
			name: "single cluster with weight > 0",
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
				{
					Weight: wrapperspb.UInt32(100),
				},
			},
			wantErr: false,
		},
		{
			name: "multiple clusters all with weight 0",
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
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
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
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
			clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
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

func TestTranslateRouteAction(t *testing.T) {
	h := &httpRouteConfigurationTranslator{
		gw: ir.GatewayIR{
			SourceObject: &ir.Gateway{
				Obj: &gwv1.Gateway{},
			},
		},
	}

	t.Run("single backend with weight 0 uses WeightedClusters", func(t *testing.T) {
		in := ir.HttpRouteRuleMatchIR{
			Backends: []ir.HttpBackend{
				{
					Backend: ir.BackendRefIR{
						ClusterName: "cluster-1",
						Weight:      0,
					},
				},
			},
		}

		outRoute := &envoyroutev3.Route{}
		action := h.translateRouteAction(in, outRoute, nil)

		weightedClusters := action.Route.GetWeightedClusters()
		assert.NotNil(t, weightedClusters, "expected RouteAction_WeightedClusters when weight is 0")
		assert.Len(t, weightedClusters.GetClusters(), 1)
		assert.Equal(t, uint32(0), weightedClusters.GetClusters()[0].GetWeight().GetValue())
	})

	t.Run("single backend with weight > 0 uses Cluster", func(t *testing.T) {
		in := ir.HttpRouteRuleMatchIR{
			Backends: []ir.HttpBackend{
				{
					Backend: ir.BackendRefIR{
						ClusterName: "cluster-1",
						Weight:      1,
					},
				},
			},
		}

		outRoute := &envoyroutev3.Route{}
		action := h.translateRouteAction(in, outRoute, nil)

		cluster := action.Route.GetCluster()
		assert.Equal(t, "cluster-1", cluster)
		assert.Nil(t, action.Route.GetWeightedClusters())
	})
}
