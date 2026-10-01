package irtranslator

import (
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
	"github.com/kgateway-dev/kgateway/v2/pkg/xds/bootstrap"
)

// TestValidationMemoKeyCoversEverythingTheBootstrapReads pins the assumption
// behind BackendTranslator.ValidationMemo: the key is a hash of the cluster's
// bytes alone, which is sound only while the validation bootstrap built in
// validateClusterConfig is a pure function of the cluster. Two clusters with
// equal bytes must produce byte-identical bootstraps, and a cluster with
// different bytes a different bootstrap, with nothing else contributing. If
// the bootstrap builder ever starts reading settings or other state, this test
// fails and the key must grow to cover it.
func TestValidationMemoKeyCoversEverythingTheBootstrapReads(t *testing.T) {
	build := func(c *envoyclusterv3.Cluster) []byte {
		t.Helper()
		b := bootstrap.New()
		b.AddCluster(c)
		bs, err := b.Build()
		require.NoError(t, err)
		out, err := proto.MarshalOptions{Deterministic: true}.Marshal(bs)
		require.NoError(t, err)
		return out
	}
	cluster := func(timeout time.Duration) *envoyclusterv3.Cluster {
		return &envoyclusterv3.Cluster{
			Name:                 "svc",
			ConnectTimeout:       durationpb.New(timeout),
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
			EdsClusterConfig: &envoyclusterv3.Cluster_EdsClusterConfig{
				EdsConfig: &envoycorev3.ConfigSource{ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{Ads: &envoycorev3.AggregatedConfigSource{}}},
			},
		}
	}

	a, aAgain, b := cluster(time.Second), cluster(time.Second), cluster(2*time.Second)
	keyA, err := validator.ContentKeyOf(a)
	require.NoError(t, err)
	keyAAgain, err := validator.ContentKeyOf(aAgain)
	require.NoError(t, err)
	keyB, err := validator.ContentKeyOf(b)
	require.NoError(t, err)

	require.Equal(t, keyA, keyAAgain, "equal cluster bytes must key the same verdict")
	require.NotEqual(t, keyA, keyB, "different cluster bytes must key different verdicts")

	// Equal key => equal bootstrap; different key => different bootstrap. This is
	// the direction that matters: a memo hit must have validated the same
	// bootstrap that would be built now.
	require.Equal(t, build(a), build(aAgain), "the bootstrap must be a function of the cluster bytes alone")
	require.NotEqual(t, build(a), build(b))

	// And the bootstrap carries exactly the cluster under validation as its only
	// static cluster, so nothing else the validator sees can vary between calls.
	bs := bootstrap.New()
	bs.AddCluster(a)
	built, err := bs.Build()
	require.NoError(t, err)
	require.Len(t, built.GetStaticResources().GetClusters(), 1)
	require.True(t, proto.Equal(a, built.GetStaticResources().GetClusters()[0]))
}
