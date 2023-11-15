package snapshot_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/v2/pkg/translator/utils"
	"github.com/solo-io/gloo/v2/pkg/xds/snapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
)

var _ = Describe("EnvoySnapshot", func() {

	It("clones correctly", func() {

		toBeCloned := snapshot.NewSnapshot("1234",
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_endpoint_v3.ClusterLoadAssignment{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_cluster_v3.Cluster{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_route_v3.Route{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_listener_v3.Listener{})},
		)

		// Create an identical struct which is guaranteed not to have been touched to compare against
		untouched := snapshot.NewSnapshot("1234",
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_endpoint_v3.ClusterLoadAssignment{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_cluster_v3.Cluster{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_route_v3.Route{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_listener_v3.Listener{})},
		)

		clone := toBeCloned.Clone()

		// Verify that original snapshot and clone are identical
		Expect(toBeCloned.Equal(clone.(*snapshot.EnvoySnapshot))).To(BeTrue())
		Expect(untouched.Equal(clone.(*snapshot.EnvoySnapshot))).To(BeTrue())

		// Mutate the clone
		clone.GetResources(
			types.EndpointTypeV3,
		).Items[""].(*resource.EnvoyResource).ResourceProto().(*envoy_config_endpoint_v3.ClusterLoadAssignment).ClusterName = "new_endpoint"

		// Verify that original snapshot was not mutated
		Expect(toBeCloned.Equal(clone.(*snapshot.EnvoySnapshot))).NotTo(BeTrue())
		Expect(toBeCloned.Equal(untouched)).To(BeTrue())
	})

	It("makes an inconsistent snapshot consistent", func() {
		hcm := &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
			StatPrefix: "placeholder",
			RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_Rds{
				Rds: &envoy_extensions_filters_network_http_connection_manager_v3.Rds{
					RouteConfigName: "routeName",
				},
			},
		}

		hcmAny := utils.ToAny(hcm)

		snapshot := snapshot.NewSnapshot("1234",
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_endpoint_v3.ClusterLoadAssignment{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_cluster_v3.Cluster{
				Name:                 "clusterName",
				ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{Type: envoy_config_cluster_v3.Cluster_EDS},
				EdsClusterConfig: &envoy_config_cluster_v3.Cluster_EdsClusterConfig{
					EdsConfig:   &envoy_config_core_v3.ConfigSource{},
					ServiceName: "edsServiceName",
				},
			})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_route_v3.Route{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_listener_v3.Listener{
				FilterChains: []*envoy_config_listener_v3.FilterChain{
					{
						Name: "placeholder_filter_chain",
						Filters: []*envoy_config_listener_v3.Filter{
							{
								ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
									TypedConfig: hcmAny,
								},
								Name: wellknown.HTTPConnectionManager,
							},
						},
					},
				},
			})})

		Expect(snapshot.Consistent()).To(HaveOccurred())
		snapshot.MakeConsistent()
		Expect(snapshot.Consistent()).NotTo(HaveOccurred())
	})
})
