package xds_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
)

var _ = Describe("Utils", func() {

	It("disabled REST EDS by default", func() {
		out := &envoy_config_cluster_v3.Cluster{}
		settings := &gloov1.Settings{}
		xds.SetEdsOnCluster(out, settings)
		Expect(out.GetEdsClusterConfig().GetEdsConfig().GetConfigSourceSpecifier()).To(Equal(&envoy_config_core_v3.ConfigSource_Ads{
			Ads: &envoy_config_core_v3.AggregatedConfigSource{},
		}))
	})

	It("sets EDS to true", func() {
		out := &envoy_config_cluster_v3.Cluster{}
		settings := &gloov1.Settings{
			Gloo: &gloov1.GlooOptions{EnableRestEds: &wrappers.BoolValue{Value: true}},
		}
		xds.SetEdsOnCluster(out, settings)
		Expect(out.GetEdsClusterConfig().GetEdsConfig().GetApiConfigSource().GetApiType()).To(Equal(envoy_config_core_v3.ApiConfigSource_REST))
	})
})
