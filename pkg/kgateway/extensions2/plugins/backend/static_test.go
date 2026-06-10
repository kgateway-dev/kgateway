package backend

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestBuildStaticIrUsesDnsClusterForHostnames(t *testing.T) {
	ir, err := buildStaticIr(&kgateway.StaticBackend{
		Hosts: []kgateway.Host{{
			Host: "example.com",
			Port: 8080,
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, ir)
	require.NotNil(t, ir.clusterTypeConfig)

	var dnsCluster envoydnsv3.DnsCluster
	err = anypb.UnmarshalTo(ir.clusterTypeConfig, &dnsCluster, proto.UnmarshalOptions{})
	require.NoError(t, err)
	assert.False(t, dnsCluster.GetAllAddressesInSingleEndpoint(), "static backends should keep strict DNS semantics")
}

func TestProcessStaticUsesCustomDnsCluster(t *testing.T) {
	ir, err := buildStaticIr(&kgateway.StaticBackend{
		Hosts: []kgateway.Host{{
			Host: "example.com",
			Port: 8080,
		}},
	})
	require.NoError(t, err)

	cluster := &envoyclusterv3.Cluster{Name: "test-cluster"}
	processStatic(ir, cluster)

	clusterType := cluster.GetClusterType()
	require.NotNil(t, clusterType, "expected custom cluster type")
	require.Equal(t, dnsClusterExtensionName, clusterType.GetName())

	var dnsCluster envoydnsv3.DnsCluster
	err = anypb.UnmarshalTo(clusterType.GetTypedConfig(), &dnsCluster, proto.UnmarshalOptions{})
	require.NoError(t, err)
	assert.False(t, dnsCluster.GetAllAddressesInSingleEndpoint(), "static backends should keep strict DNS semantics")

	require.NotNil(t, cluster.LoadAssignment)
	require.Len(t, cluster.LoadAssignment.Endpoints, 1)
	require.Len(t, cluster.LoadAssignment.Endpoints[0].LbEndpoints, 1)
	endpoint := cluster.LoadAssignment.Endpoints[0].LbEndpoints[0].GetEndpoint()
	require.NotNil(t, endpoint)
	assert.Equal(t, "example.com", endpoint.GetHostname())
	assert.Equal(t, "example.com", endpoint.GetAddress().GetSocketAddress().GetAddress())
	assert.Equal(t, uint32(8080), endpoint.GetAddress().GetSocketAddress().GetPortValue())
}

// One StaticIr can be translated under multiple cluster names: gateway-scoped
// backend variants (e.g. CloneForGatewayBackendClientCertificate) share the IR
// but derive their own cluster name. Each name must get a load assignment with
// the matching name, while repeated translations of the same name share one copy.
func TestProcessStaticLoadAssignmentPerClusterName(t *testing.T) {
	ir, err := buildStaticIr(&kgateway.StaticBackend{
		Hosts: []kgateway.Host{{
			Host: "example.com",
			Port: 8080,
		}},
	})
	require.NoError(t, err)

	base := &envoyclusterv3.Cluster{Name: "base-cluster"}
	processStatic(ir, base)
	variant := &envoyclusterv3.Cluster{Name: "gateway-variant-cluster"}
	processStatic(ir, variant)
	baseAgain := &envoyclusterv3.Cluster{Name: "base-cluster"}
	processStatic(ir, baseAgain)

	assert.Equal(t, "base-cluster", base.GetLoadAssignment().GetClusterName())
	assert.Equal(t, "gateway-variant-cluster", variant.GetLoadAssignment().GetClusterName())
	assert.Same(t, base.GetLoadAssignment(), baseAgain.GetLoadAssignment(),
		"clusters with the same name should share one load assignment")
	assert.NotSame(t, base.GetLoadAssignment(), variant.GetLoadAssignment(),
		"clusters with different names must not share a load assignment")
	assert.Empty(t, ir.loadAssignment.GetClusterName(),
		"the IR's original load assignment must not be mutated")
}
