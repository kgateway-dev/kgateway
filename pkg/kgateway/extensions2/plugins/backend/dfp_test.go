package backend

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycommondnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/common/dns/v3"
	envoy_dfp_cluster "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dynamic_forward_proxy/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestDynamicForwardProxyDNSLookupFamily(t *testing.T) {
	defaultIR, err := buildDfpIr(&kgateway.DynamicForwardProxyBackend{})
	require.NoError(t, err)
	var defaultConfig envoy_dfp_cluster.ClusterConfig
	require.NoError(t, defaultIR.clusterTypeConfig.UnmarshalTo(&defaultConfig))
	require.NotNil(t, defaultConfig.GetSubClustersConfig())
	assert.Nil(t, defaultConfig.GetSubClustersConfig().GetDnsClusterConfig())

	backend := &kgateway.DynamicForwardProxyBackend{
		DNSLookupFamily: new(kgateway.DnsLookupFamilyIPv4Only),
	}
	ipv4IR, err := buildDfpIr(backend)
	require.NoError(t, err)
	cluster := &envoyclusterv3.Cluster{}
	processDynamicForwardProxy(ipv4IR, cluster)
	var ipv4Config envoy_dfp_cluster.ClusterConfig
	require.NoError(t, cluster.GetClusterType().GetTypedConfig().UnmarshalTo(&ipv4Config))
	subClusters := ipv4Config.GetSubClustersConfig()
	require.NotNil(t, subClusters.GetDnsClusterConfig())
	assert.Equal(t, envoycommondnsv3.DnsLookupFamily_V4_ONLY, subClusters.GetDnsClusterConfig().GetDnsLookupFamily())
	assert.Equal(t, envoyclusterv3.Cluster_LEAST_REQUEST, subClusters.GetLbPolicy())

	ipv4IRAgain, err := buildDfpIr(backend)
	require.NoError(t, err)
	assert.True(t, ipv4IR.Equals(ipv4IRAgain))
	assert.False(t, ipv4IR.Equals(defaultIR))

	ipv6IR, err := buildDfpIr(&kgateway.DynamicForwardProxyBackend{
		DNSLookupFamily: new(kgateway.DnsLookupFamilyIPv6Only),
	})
	require.NoError(t, err)
	assert.False(t, ipv4IR.Equals(ipv6IR))
}
