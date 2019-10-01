package transformation_test

import (
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	transformation "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
)

var _ = Describe("Plugin", func() {
	var (
		p        *Plugin
		t        *transformation.RouteTransformations
		expected *types.Struct
	)

	BeforeEach(func() {
		p = NewPlugin()
		t = &transformation.RouteTransformations{
			ClearRouteCache: true,
		}
		configStruct, err := util.MessageToStruct(t)
		Expect(err).NotTo(HaveOccurred())

		expected = configStruct
	})

	It("sets transformation config for weighted destinations", func() {
		out := &envoyroute.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			WeightedDestinationPlugins: &v1.WeightedDestinationPlugins{
				Transformations: t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
	})
	It("sets transformation config for virtual hosts", func() {
		out := &envoyroute.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			VirtualHostPlugins: &v1.VirtualHostPlugins{
				Transformations: t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
	})
	It("sets transformation config for routes", func() {

		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			RoutePlugins: &v1.RoutePlugins{
				Transformations: t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
	})
})
