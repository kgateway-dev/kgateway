package proxy_syncer

import (
	"context"
	"errors"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

type backendBatchValidator struct {
	validateFunc  func([]*envoyclusterv3.Cluster) error
	clusterCounts []int
}

func (v *backendBatchValidator) Validate(_ context.Context, bootstrap *envoybootstrapv3.Bootstrap) error {
	clusters := bootstrap.GetStaticResources().GetClusters()
	v.clusterCounts = append(v.clusterCounts, len(clusters))
	if v.validateFunc != nil {
		return v.validateFunc(clusters)
	}
	return nil
}

func TestValidateTranslatedClustersStrictBatchesValidClusters(t *testing.T) {
	v := &backendBatchValidator{}
	translator := &irtranslator.BackendTranslator{
		Mode:      apisettings.ValidationStrict,
		Validator: v,
	}

	out := validateTranslatedClusters(context.Background(), translator, []uccWithCluster{
		testUCCWithCluster("cluster-a"),
		testUCCWithCluster("cluster-b"),
		testUCCWithCluster("cluster-c"),
	})

	require.Len(t, out, 3)
	for _, cluster := range out {
		assert.NoError(t, cluster.Error)
	}
	assert.Equal(t, []int{3}, v.clusterCounts)
}

func TestValidateTranslatedClustersStandardSkipsEnvoyValidation(t *testing.T) {
	v := &backendBatchValidator{}
	translator := &irtranslator.BackendTranslator{
		Mode:      apisettings.ValidationStandard,
		Validator: v,
	}

	out := validateTranslatedClusters(context.Background(), translator, []uccWithCluster{
		testUCCWithCluster("cluster-a"),
		testUCCWithCluster("cluster-b"),
	})

	require.Len(t, out, 2)
	assert.Empty(t, v.clusterCounts)
}

func TestValidateTranslatedClustersStrictIsolatesInvalidClusterAfterBatchFailure(t *testing.T) {
	badClusterErr := errors.New("bad cluster")
	v := &backendBatchValidator{
		validateFunc: func(clusters []*envoyclusterv3.Cluster) error {
			if len(clusters) > 1 {
				if len(clusters) == 3 {
					return errors.New("batch failed")
				}
				return nil
			}
			if clusters[0].GetName() == "cluster-b" {
				return badClusterErr
			}
			return nil
		},
	}
	translator := &irtranslator.BackendTranslator{
		Mode:      apisettings.ValidationStrict,
		Validator: v,
	}

	out := validateTranslatedClusters(context.Background(), translator, []uccWithCluster{
		testUCCWithCluster("cluster-a"),
		testUCCWithCluster("cluster-b"),
		testUCCWithCluster("cluster-c"),
	})

	require.Len(t, out, 3)
	assert.NoError(t, out[0].Error)
	assert.ErrorIs(t, out[1].Error, badClusterErr)
	assert.NoError(t, out[2].Error)
	assert.Equal(t, envoyclusterv3.Cluster_STATIC, out[1].Cluster.GetType())
	assert.Empty(t, out[1].Cluster.GetLoadAssignment().GetEndpoints())
	assert.Equal(t, []int{3, 1, 1, 1, 2}, v.clusterCounts)
}

func testUCCWithCluster(name string) uccWithCluster {
	cluster := &envoyclusterv3.Cluster{Name: name}
	return uccWithCluster{
		Name:           name,
		Cluster:        cluster,
		ClusterVersion: utils.HashProto(cluster),
	}
}
