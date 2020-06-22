package xds

import (
	envoycluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

func SetEdsOnCluster(out *envoycluster.Cluster) {
	out.ClusterDiscoveryType = &envoycluster.Cluster_Type{
		Type: envoycluster.Cluster_EDS,
	}
	out.EdsClusterConfig = &envoycluster.Cluster_EdsClusterConfig{
		EdsConfig: &envoycore.ConfigSource{
			ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
				Ads: &envoycore.AggregatedConfigSource{},
			},
		},
	}
}
