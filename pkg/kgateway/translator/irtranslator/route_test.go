package irtranslator

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/wrapperspb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
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

func TestSetEnvoyPathMatcher_PathPrefix(t *testing.T) {
	pathPrefix := gwv1.PathMatchPathPrefix

	tests := []struct {
		name         string
		path         string
		wantPrefix   string
		wantSeparate bool
	}{
		{
			name:         "uses path separated prefix for clean prefix",
			path:         "/foo",
			wantPrefix:   "/foo",
			wantSeparate: true,
		},
		{
			name:         "ignores trailing slash for non root prefix",
			path:         "/foo/",
			wantPrefix:   "/foo",
			wantSeparate: true,
		},
		{
			name:         "keeps root prefix unchanged",
			path:         "/",
			wantPrefix:   "/",
			wantSeparate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &envoyroutev3.RouteMatch{}

			setEnvoyPathMatcher(gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  &pathPrefix,
					Value: &tt.path,
				},
			}, out)

			if tt.wantSeparate {
				spec, ok := out.PathSpecifier.(*envoyroutev3.RouteMatch_PathSeparatedPrefix)
				assert.True(t, ok)
				assert.Equal(t, tt.wantPrefix, spec.PathSeparatedPrefix)
				return
			}

			spec, ok := out.PathSpecifier.(*envoyroutev3.RouteMatch_Prefix)
			assert.True(t, ok)
			assert.Equal(t, tt.wantPrefix, spec.Prefix)
		})
	}
}
