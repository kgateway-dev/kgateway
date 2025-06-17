package backend

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_dfp_cluster "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dynamic_forward_proxy/v3"
	envoydfp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_forward_proxy/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	eiutils "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"

	envoy_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

var dfpFilterConfig = &envoydfp.FilterConfig{
	ImplementationSpecifier: &envoydfp.FilterConfig_SubClusterConfig{
		SubClusterConfig: &envoydfp.SubClusterConfig{},
	},
}

func processDynamicForwardProxy(in *v1alpha1.DynamicForwardProxyBackend, out *envoy_config_cluster_v3.Cluster) error {
	out.LbPolicy = envoy_config_cluster_v3.Cluster_CLUSTER_PROVIDED
	c := &envoy_dfp_cluster.ClusterConfig{
		ClusterImplementationSpecifier: &envoy_dfp_cluster.ClusterConfig_SubClustersConfig{
			SubClustersConfig: &envoy_dfp_cluster.SubClustersConfig{
				LbPolicy: envoy_config_cluster_v3.Cluster_LEAST_REQUEST,
			},
		},
	}
	anyCluster, err := utils.MessageToAny(c)
	if err != nil {
		return err
	}
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_ClusterType{
		ClusterType: &envoy_config_cluster_v3.Cluster_CustomClusterType{
			Name:        "envoy.clusters.dynamic_forward_proxy",
			TypedConfig: anyCluster,
		},
	}

	if in.EnableTls {
		validationContext := &envoy_tls_v3.CertificateValidationContext{}
		sdsValidationCtx := &envoy_tls_v3.SdsSecretConfig{
			Name: eiutils.SystemCaSecretName,
		}

		tlsContextDefault := &envoy_tls_v3.UpstreamTlsContext{
			CommonTlsContext: &envoy_tls_v3.CommonTlsContext{
				ValidationContextType: &envoy_tls_v3.CommonTlsContext_CombinedValidationContext{
					CombinedValidationContext: &envoy_tls_v3.CommonTlsContext_CombinedCertificateValidationContext{
						DefaultValidationContext:         validationContext,
						ValidationContextSdsSecretConfig: sdsValidationCtx,
					},
				},
			},
		}

		typedConfig, _ := utils.MessageToAny(tlsContextDefault)
		out.TransportSocket = &envoy_config_core_v3.TransportSocket{
			Name: wellknown.TransportSocketTls,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
				TypedConfig: typedConfig,
			},
		}
	}

	return nil
}
