package translator

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	// . "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/pkg/protoutil"

	"github.com/k0kubun/pp"
	"github.com/solo-io/gloo/pkg/plugins"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("InitPlugin", func() {
	It("should set correct weights for functions", func() {
		initPlugin := newRouteInitializerPlugin()

		outroute := envoyroute.Route{Metadata: new(envoycore.Metadata)}
		outroute.Metadata.FilterMetadata = make(map[string]*types.Struct)
		inroute := &v1.Route{
			MultipleDestinations: []*v1.WeightedDestination{{
				Destination: &v1.Destination{
					DestinationType: &v1.Destination_Function{
						Function: &v1.FunctionDestination{
							UpstreamName: "my-upstream",
							FunctionName: "func1",
						},
					},
				},
				Weight: 10,
			}, {
				Destination: &v1.Destination{
					DestinationType: &v1.Destination_Function{
						Function: &v1.FunctionDestination{
							UpstreamName: "my-upstream",
							FunctionName: "func2",
						},
					},
				},
				Weight: 10,
			}, {
				Destination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: &v1.UpstreamDestination{
							Name: "my-upstream2",
						},
					},
				},
				Weight: 10,
			}},
		}
		params := &plugins.RoutePluginParams{}
		pp.Fprintln(GinkgoWriter, inroute)

		err := initPlugin.ProcessRoute(params, inroute, &outroute)
		pp.Fprintln(GinkgoWriter, outroute)
		Expect(err).NotTo(HaveOccurred())
		clusterweight := outroute.Action.(*envoyroute.Route_Route).Route.ClusterSpecifier.(*envoyroute.RouteAction_WeightedClusters).WeightedClusters
		routeweight := clusterweight.TotalWeight.Value

		Expect(clusterweight.Clusters[0].Weight.Value).To(BeEquivalentTo(10))
		Expect(clusterweight.Clusters[0].PerFilterConfig[filterName]).NotTo(BeNil())
		var res FunctionalFilterRouteConfig
		err = protoutil.UnmarshalStruct(clusterweight.Clusters[0].PerFilterConfig[filterName], &res)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.FunctionName).To(Equal("func1"))

		Expect(clusterweight.Clusters[1].Weight.Value).To(BeEquivalentTo(10))
		Expect(clusterweight.Clusters[2].Weight.Value).To(BeEquivalentTo(10))

		Expect(routeweight).To(BeEquivalentTo(30))

	})

})
