package xds

import (
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/golang/protobuf/ptypes"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

// SetEdsOnCluster marks an Envoy Cluster to receive its Endpoints from the xDS Server
// https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/service_discovery#arch-overview-service-discovery-types-eds
// In Gloo, we support both streaming (gRPC) and polling (REST)
//
// NOTE: REST EDS was introduced as a way of bypassing https://github.com/envoyproxy/envoy/issues/13070
// a bug in Envoy that would cause clusters to warm, without endpoints.
// That bug has since been resolved and gRPC EDS is preferred as the polling solution is more
// resource intensive and could delay updates by as much as 5 seconds (or whatever the refresh delay is)
func SetEdsOnCluster(out *envoy_config_cluster_v3.Cluster, settings *v1.Settings) {
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_EDS,
	}

	restEds := false
	if restEdsSetting := settings.GetGloo().GetEnableRestEds(); restEdsSetting != nil {
		restEds = restEdsSetting.GetValue()
	}
	// The default value for enableRestEds should be set to false via helm.
	// If nil, will use grpc eds
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
