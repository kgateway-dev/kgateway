package check_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
)

var _ = Describe("Gloo Stats", func() {

	const (
		deploymentName = "gloo"
	)

	Context("check resources in sync", func() {
		It("returns true when there are no stats", func() {
			result := check.ResourcesSyncedOverXds("", deploymentName)
			Expect(result).To(BeTrue())
		})

		It("returns false when there are resources out of sync", func() {
			stats := `
# TYPE glooe_solo_io_xds_insync counter
glooe_solo_io_xds_insync{resource="type.googleapis.com/envoy.api.v2.Cluster"} 1
glooe_solo_io_xds_insync{resource="type.googleapis.com/envoy.api.v2.ClusterLoadAssignment"} 1
glooe_solo_io_xds_insync{resource="type.googleapis.com/envoy.api.v2.Listener"} 1
glooe_solo_io_xds_insync{resource="type.googleapis.com/envoy.api.v2.RouteConfiguration"} 0
glooe_solo_io_xds_insync{resource="type.googleapis.com/glooe.solo.io.RateLimitConfig"} 1
# HELP glooe_solo_io_xds_nack The number of envoys that reported NACK
# TYPE glooe_solo_io_xds_nack counter
glooe_solo_io_xds_nack{resource="type.googleapis.com/envoy.api.v2.RouteConfiguration"} 1
# HELP glooe_solo_io_xds_total_entities The total number of XDS streams
# TYPE glooe_solo_io_xds_total_entities counter
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/enterprise.gloo.solo.io.ExtAuthConfig"} 1
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/envoy.api.v2.Cluster"} 1
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/envoy.api.v2.ClusterLoadAssignment"} 1
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/envoy.api.v2.Listener"} 1
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/envoy.api.v2.RouteConfiguration"} 1
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/glooe.solo.io.RateLimitConfig"} 1
`
			result := check.ResourcesSyncedOverXds(stats, deploymentName)
			Expect(result).To(BeFalse())
		})

		It("returns true when all resources in sync", func() {
			stats := `
# TYPE glooe_solo_io_xds_insync counter
glooe_solo_io_xds_insync{resource="type.googleapis.com/envoy.api.v2.Cluster"} 1
glooe_solo_io_xds_insync{resource="type.googleapis.com/envoy.api.v2.ClusterLoadAssignment"} 1
glooe_solo_io_xds_insync{resource="type.googleapis.com/envoy.api.v2.Listener"} 1
glooe_solo_io_xds_insync{resource="type.googleapis.com/envoy.api.v2.RouteConfiguration"} 1
glooe_solo_io_xds_insync{resource="type.googleapis.com/glooe.solo.io.RateLimitConfig"} 1
# HELP glooe_solo_io_xds_total_entities The total number of XDS streams
# TYPE glooe_solo_io_xds_total_entities counter
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/enterprise.gloo.solo.io.ExtAuthConfig"} 1
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/envoy.api.v2.Cluster"} 1
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/envoy.api.v2.ClusterLoadAssignment"} 1
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/envoy.api.v2.Listener"} 1
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/envoy.api.v2.RouteConfiguration"} 1
glooe_solo_io_xds_total_entities{resource="type.googleapis.com/glooe.solo.io.RateLimitConfig"} 1
`
			result := check.ResourcesSyncedOverXds(stats, deploymentName)
			Expect(result).To(BeTrue())
		})
	})

	Context("check rate limit connected state", func() {

		It("returns true when there are no stats", func() {
			result := check.CheckRateLimitConnectedState("")
			Expect(result).To(BeTrue())
		})

		It("returns true when connected state equals 1", func() {
			result := check.CheckRateLimitConnectedState("glooe_ratelimit_connected_state 1")
			Expect(result).To(BeTrue())
		})

		It("returns false when connected state stat equals 0", func() {
			result := check.CheckRateLimitConnectedState(check.GlooeRateLimitDisconnected)
			Expect(result).To(BeFalse())
		})
	})

})
