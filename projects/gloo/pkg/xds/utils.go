package xds

import (
	"time"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/ptypes"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

func SetEdsOnCluster(out *envoyapi.Cluster, settings *v1.Settings) {
	out.ClusterDiscoveryType = &envoyapi.Cluster_Type{
		Type: envoyapi.Cluster_EDS,
	}
	// The default value for enableRestEds should be set to true via helm.
	// If nil will default to rest eds.
	if !settings.GetGloo().GetEnableRestEds().GetValue() {
		out.EdsClusterConfig = &envoyapi.Cluster_EdsClusterConfig{
			EdsConfig: &envoycore.ConfigSource{
				ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
					Ads: &envoycore.AggregatedConfigSource{},
				},
			},
		}
	} else {
		out.EdsClusterConfig = &envoyapi.Cluster_EdsClusterConfig{
			EdsConfig: &envoycore.ConfigSource{
				ConfigSourceSpecifier: &envoycore.ConfigSource_ApiConfigSource{
					ApiConfigSource: &envoycore.ApiConfigSource{
						ApiType:        envoycore.ApiConfigSource_REST,
						ClusterNames:   []string{defaults.GlooRestXdsName},
						RefreshDelay:   ptypes.DurationProto(time.Second * 5),
						RequestTimeout: ptypes.DurationProto(time.Second * 5),
					},
				},
			},
		}
	}
}
