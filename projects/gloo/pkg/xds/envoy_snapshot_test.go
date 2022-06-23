package xds_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
)

var _ = Describe("EnvoySnapshot", func() {

	It("clones correctly", func() {

		toBeCloned := xds.NewSnapshot("1234",
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_endpoint_v3.ClusterLoadAssignment{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_cluster_v3.Cluster{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_route_v3.Route{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_listener_v3.Listener{})},
		)

		// Create an identical struct which is guaranteed not to have been touched to compare against
		untouched := xds.NewSnapshot("1234",
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_endpoint_v3.ClusterLoadAssignment{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_cluster_v3.Cluster{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_route_v3.Route{})},
			[]cache.Resource{resource.NewEnvoyResource(&envoy_config_listener_v3.Listener{})},
		)

		clone := toBeCloned.Clone()

		// Verify that original snapshot and clone are identical
		Expect(toBeCloned.Equal(clone.(*xds.EnvoySnapshot))).To(BeTrue())
		Expect(untouched.Equal(clone.(*xds.EnvoySnapshot))).To(BeTrue())

		// Mutate the clone
		clone.GetResources(
			types.EndpointTypeV3,
		).Items[""].(*resource.EnvoyResource).ResourceProto().(*envoy_config_endpoint_v3.ClusterLoadAssignment).ClusterName = "new_endpoint"

		// Verify that original snapshot was not mutated
		Expect(toBeCloned.Equal(clone.(*xds.EnvoySnapshot))).NotTo(BeTrue())
		Expect(toBeCloned.Equal(untouched)).To(BeTrue())
	})
})
