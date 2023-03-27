package translator_test

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"

	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"
)

var _ = Describe("Ssl Configuration", func() {

	Context("GroupVirtualServicesBySslConfig", func() {

		When("2 virtual services do not overlap", func() {

			var (
				vsEast, vsWest *v1.VirtualService
			)

			BeforeEach(func() {
				vsEast = helpers.NewVirtualServiceBuilder().
					WithName("vs-east").
					WithNamespace(defaults.GlooSystem).
					WithDomain("east.com").
					WithSslConfig(&ssl.SslConfig{
						OneWayTls: &wrappers.BoolValue{
							Value: true,
						},
					}).
					Build()
				vsWest = helpers.NewVirtualServiceBuilder().
					WithName("vs-west").
					WithNamespace(defaults.GlooSystem).
					WithDomain("west.com").
					WithSslConfig(&ssl.SslConfig{
						OneWayTls: &wrappers.BoolValue{
							Value: false,
						},
					}).
					Build()
			})

			It("does not merge the configurations", func() {
				sslConfig, virtualServicesBySsl := GroupVirtualServicesBySslConfig(v1.VirtualServiceList{vsEast, vsWest})

				Expect(sslConfig).To(HaveLen(2), "ssl configs should not be merged")
				Expect(sslConfig).To(ContainElements(vsEast.GetSslConfig(), vsWest.GetSslConfig()))
				Expect(virtualServicesBySsl).To(HaveLen(2), "2 unique sslConfigs in map")
			})

		})

		When("2 virtual services differ only by SNI", func() {

			var (
				vsEast, vsWest *v1.VirtualService
			)

			BeforeEach(func() {
				vsEast = helpers.NewVirtualServiceBuilder().
					WithName("vs-east").
					WithNamespace(defaults.GlooSystem).
					WithDomain("east.com").
					WithSslConfig(&ssl.SslConfig{
						OneWayTls: &wrappers.BoolValue{
							Value: true,
						},
						SniDomains: []string{
							"north.com",
							"south.com",
							"east.com",
						},
					}).
					Build()
				vsWest = helpers.NewVirtualServiceBuilder().
					WithName("vs-west").
					WithNamespace(defaults.GlooSystem).
					WithDomain("west.com").
					WithSslConfig(&ssl.SslConfig{
						OneWayTls: &wrappers.BoolValue{
							Value: true,
						},
						SniDomains: []string{
							"north.com",
							"west.com",
						},
					}).
					Build()
			})

			It("does merge the configurations", func() {
				sslConfig, virtualServicesBySslConfig := GroupVirtualServicesBySslConfig(v1.VirtualServiceList{vsEast, vsWest})

				Expect(sslConfig).To(HaveLen(1), "ssl configs should be merged")
				joinedSniDomains := []string{"north.com", "east.com", "west.com", "south.com"}
				Expect(sslConfig[0].GetSniDomains()).To(ContainElements(joinedSniDomains), "sni domains should be joined")

				Expect(virtualServicesBySslConfig).To(HaveLen(1), "only 1 unique sslConfig in map")

				for ssl, virtualServicesBySsl := range virtualServicesBySslConfig {
					Expect(ssl.GetSniDomains()).To(ContainElements(joinedSniDomains))
					Expect(virtualServicesBySsl).To(HaveLen(2))
				}
			})

		})

	})

})
