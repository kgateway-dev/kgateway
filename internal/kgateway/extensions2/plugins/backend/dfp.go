package backend

import (
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_dfp_cluster "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dynamic_forward_proxy/v3"
	envoydfp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_forward_proxy/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	eiutils "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"

	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

var dfpFilterConfig = &envoydfp.FilterConfig{
	ImplementationSpecifier: &envoydfp.FilterConfig_SubClusterConfig{
		SubClusterConfig: &envoydfp.SubClusterConfig{},
	},
}

func processDynamicForwardProxy(in *v1alpha1.DynamicForwardProxyBackend, out *envoyclusterv3.Cluster) error {
	out.LbPolicy = envoyclusterv3.Cluster_CLUSTER_PROVIDED
	c := &envoy_dfp_cluster.ClusterConfig{
		ClusterImplementationSpecifier: &envoy_dfp_cluster.ClusterConfig_SubClustersConfig{
			SubClustersConfig: &envoy_dfp_cluster.SubClustersConfig{
				LbPolicy: envoyclusterv3.Cluster_LEAST_REQUEST,
			},
		},
	}
	anyCluster, err := utils.MessageToAny(c)
	if err != nil {
		return err
	}
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_ClusterType{
		ClusterType: &envoyclusterv3.Cluster_CustomClusterType{
			Name:        "envoy.clusters.dynamic_forward_proxy",
			TypedConfig: anyCluster,
		},
	}

	if in.EnableTls {
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

		typedConfig, _ := utils.MessageToAny(tlsContextDefault)
		out.TransportSocket = &envoycorev3.TransportSocket{
			Name: wellknown.TransportSocketTls,
			ConfigType: &envoycorev3.TransportSocket_TypedConfig{
				TypedConfig: typedConfig,
			},
		}
	}

	return nil
}
