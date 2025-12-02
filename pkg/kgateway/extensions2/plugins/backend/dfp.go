package backend

import (
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_dfp_cluster "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dynamic_forward_proxy/v3"
	envoydfp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_forward_proxy/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	eiutils "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

var dfpFilterConfig = &envoydfp.FilterConfig{
	ImplementationSpecifier: &envoydfp.FilterConfig_SubClusterConfig{
		SubClusterConfig: &envoydfp.SubClusterConfig{},
	},
}

// DfpIr is the internal representation of a dynamic forward proxy backend.
type DfpIr struct {
	clusterTypeConfig *anypb.Any
	transportSocket   *envoycorev3.TransportSocket
}

// Equals checks if two DfpIr objects are equal.
func (u *DfpIr) Equals(other any) bool {
	otherDfp, ok := other.(*DfpIr)
	if !ok {
		return false
	}
	return cmputils.CompareWithNils(u, otherDfp, func(a, b *DfpIr) bool {
		return proto.Equal(a.clusterTypeConfig, b.clusterTypeConfig) &&
			proto.Equal(a.transportSocket, b.transportSocket)
	})
}

func buildDfpIr(in *v1alpha1.DynamicForwardProxyBackend) (*DfpIr, error) {
	ir := &DfpIr{}

	c := &envoy_dfp_cluster.ClusterConfig{
		ClusterImplementationSpecifier: &envoy_dfp_cluster.ClusterConfig_SubClustersConfig{
			SubClustersConfig: &envoy_dfp_cluster.SubClustersConfig{
				LbPolicy: envoyclusterv3.Cluster_LEAST_REQUEST,
			},
		},
	}
	anyCluster, err := utils.MessageToAny(c)
	if err != nil {
		return nil, err
	}
	ir.clusterTypeConfig = anyCluster

	if ptr.Deref(in.EnableTls, false) {
		validationContext := &envoytlsv3.CertificateValidationContext{}
		sdsValidationCtx := &envoytlsv3.SdsSecretConfig{
			Name: eiutils.SystemCaSecretName,
		}

		tlsContextDefault := &envoytlsv3.UpstreamTlsContext{
			CommonTlsContext: &envoytlsv3.CommonTlsContext{
				ValidationContextType: &envoytlsv3.CommonTlsContext_CombinedValidationContext{
					CombinedValidationContext: &envoytlsv3.CommonTlsContext_CombinedCertificateValidationContext{
						DefaultValidationContext:         validationContext,
						ValidationContextSdsSecretConfig: sdsValidationCtx,
					},
				},
			},
		}

		typedConfig, err := utils.MessageToAny(tlsContextDefault)
		if err != nil {
			return nil, err
		}
		ir.transportSocket = &envoycorev3.TransportSocket{
			Name: wellknown.TransportSocketTls,
			ConfigType: &envoycorev3.TransportSocket_TypedConfig{
				TypedConfig: typedConfig,
			},
		}
	}

	return ir, nil
}

// processDynamicForwardProxy applies the DFP IR to the envoy cluster.
func processDynamicForwardProxy(ir *DfpIr, out *envoyclusterv3.Cluster) {
	out.LbPolicy = envoyclusterv3.Cluster_CLUSTER_PROVIDED
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_ClusterType{
		ClusterType: &envoyclusterv3.Cluster_CustomClusterType{
			Name:        "envoy.clusters.dynamic_forward_proxy",
			TypedConfig: ir.clusterTypeConfig,
		},
	}

	if ir.transportSocket != nil {
		out.TransportSocket = ir.transportSocket
	}
}
