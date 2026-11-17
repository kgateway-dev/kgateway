package backend

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycommondnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/common/dns/v3"
	envoy_dfp_cluster "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dynamic_forward_proxy/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestDynamicForwardProxyDNSLookupFamily(t *testing.T) {
	t.Setenv("KGW_DNS_LOOKUP_FAMILY", string(apisettings.DnsLookupFamilyV6Only))
	settings, err := apisettings.BuildSettings()
	require.NoError(t, err)
	translate := buildTranslateFunc(nil, nil, settings)
	backend := &kgateway.Backend{
		Spec: kgateway.BackendSpec{
			Type:                new(kgateway.BackendTypeDynamicForwardProxy),
			DynamicForwardProxy: &kgateway.DynamicForwardProxyBackend{},
		},
	}

	translateDFP := func() *DfpIr {
		t.Helper()
		ir := translate(krt.TestingDummyContext{}, backend)
		require.Empty(t, ir.errors)
		require.NotNil(t, ir.dfpIr)
		return ir.dfpIr
	}
	assertFamily := func(ir *DfpIr, want envoycommondnsv3.DnsLookupFamily) {
		t.Helper()
		cluster := &envoyclusterv3.Cluster{}
		processDynamicForwardProxy(ir, cluster)
		var config envoy_dfp_cluster.ClusterConfig
		require.NoError(t, cluster.GetClusterType().GetTypedConfig().UnmarshalTo(&config))
		subClusters := config.GetSubClustersConfig()
		require.NotNil(t, subClusters.GetDnsClusterConfig())
		assert.Equal(t, want, subClusters.GetDnsClusterConfig().GetDnsLookupFamily())
		assert.Equal(t, envoyclusterv3.Cluster_LEAST_REQUEST, subClusters.GetLbPolicy())
	}

	inheritedIR := translateDFP()
	assertFamily(inheritedIR, envoycommondnsv3.DnsLookupFamily_V6_ONLY)

	backend.Spec.DynamicForwardProxy.DNSLookupFamily = new(kgateway.DnsLookupFamilyIPv4Only)
	ipv4IR := translateDFP()
	assertFamily(ipv4IR, envoycommondnsv3.DnsLookupFamily_V4_ONLY)
	assert.True(t, ipv4IR.Equals(translateDFP()))
	assert.False(t, ipv4IR.Equals(inheritedIR))
}
