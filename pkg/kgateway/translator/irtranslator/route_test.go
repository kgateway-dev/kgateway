package irtranslator

import (
	"context"
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestEnvoyRoutesDirectResponse(t *testing.T) {
	t.Parallel()

	regexMatch := gwv1.HeaderMatchRegularExpression
	translator := &httpRouteConfigurationTranslator{}
	route := translator.envoyRoutes(context.Background(), nil, ir.HttpRouteRuleMatchIR{
		Match: gwv1.HTTPRouteMatch{
			Headers: []gwv1.HTTPHeaderMatch{
				{
					Name:  gwv1.HTTPHeaderName(":authority"),
					Value: `(?i)^foo\.example\.org(?::[0-9]+)?$`,
					Type:  &regexMatch,
				},
			},
		},
		DirectResponse: &ir.DirectResponseIR{
			StatusCode: 421,
		},
	}, "synthetic")

	require.NotNil(t, route)
	require.NotNil(t, route.GetDirectResponse())
	assert.Equal(t, uint32(421), route.GetDirectResponse().GetStatus())
	require.Len(t, route.GetMatch().GetHeaders(), 1)
	assert.Equal(t, ":authority", route.GetMatch().GetHeaders()[0].GetName())
	assert.NotNil(t, route.GetMatch().GetHeaders()[0].GetStringMatch().GetSafeRegex())
}
