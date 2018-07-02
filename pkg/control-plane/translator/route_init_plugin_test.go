package translator

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	// . "github.com/solo-io/gloo/test/helpers"
	// . "github.com/solo-io/gloo/pkg/translator"

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

		Expect(getCluster(clusterweight, "my-upstream").Weight.Value).To(BeEquivalentTo(20))
		Expect(getCluster(clusterweight, "my-upstream2").Weight.Value).To(BeEquivalentTo(10))

		Expect(routeweight).To(BeEquivalentTo(30))

		metadata := outroute.Metadata
		clusterroutemeta := metadata.FilterMetadata[filterName].Fields["my-upstream"].Kind.(*types.Value_StructValue).StructValue.Fields
		wieghtedfunctionsmeta := clusterroutemeta[multiFunctionDestinationKey].Kind.(*types.Value_StructValue).StructValue.Fields
		totalfuncweight := wieghtedfunctionsmeta[multiFunctionWeightDestinationKey].Kind.(*types.Value_NumberValue).NumberValue
		Expect(totalfuncweight).To(BeEquivalentTo(20))

		functionslist := wieghtedfunctionsmeta[multiFunctionListDestinationKey].Kind.(*types.Value_ListValue).ListValue.Values
		for _, fl := range functionslist {
			weight := fl.Kind.(*types.Value_StructValue).StructValue.Fields["weight"].Kind.(*types.Value_NumberValue).NumberValue
			Expect(weight).To(BeEquivalentTo(10))

		}
	})

})

func getCluster(clusters *envoyroute.WeightedCluster, name string) *envoyroute.WeightedCluster_ClusterWeight {
	for _, c := range clusters.Clusters {
		if c.Name == name {
			return c
		}
	}
	return nil
}
