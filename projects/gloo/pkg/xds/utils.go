package xds

import (
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/golang/protobuf/ptypes"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

func SetEdsOnCluster(out *envoy_config_cluster_v3.Cluster, settings *v1.Settings) {
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_EDS,
	}

	restEds := true
	if restEdsSetting := settings.GetGloo().GetEnableRestEds(); restEdsSetting != nil {
		restEds = restEdsSetting.GetValue()
	}
	// The default value for enableRestEds should be set to true via helm.
	// If nil, will enable rest eds
	if restEds {
		out.EdsClusterConfig = &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
			EdsConfig: &envoy_config_core_v3.ConfigSource{
				ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
				ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_ApiConfigSource{
					ApiConfigSource: &envoy_config_core_v3.ApiConfigSource{
						ApiType:             envoy_config_core_v3.ApiConfigSource_REST,
						TransportApiVersion: envoy_config_core_v3.ApiVersion_V3,
						ClusterNames:        []string{defaults.GlooRestXdsName},
						RefreshDelay:        ptypes.DurationProto(time.Second * 5),
						RequestTimeout:      ptypes.DurationProto(time.Second * 5),
					},
				},
			},
		}
	} else {
		out.EdsClusterConfig = &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
			EdsConfig: &envoy_config_core_v3.ConfigSource{
				ResourceApiVersion: envoy_config_core_v3.ApiVersion_V3,
				ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_Ads{
					Ads: &envoy_config_core_v3.AggregatedConfigSource{},
				},
			},
		}

	}
}
