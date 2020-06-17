package transformation_test

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/any"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
)

var _ = Describe("Plugin", func() {
	var (
		p        *Plugin
		t        *transformation.RouteTransformations
		expected *any.Any
	)

	BeforeEach(func() {
		p = NewPlugin()
		t = &transformation.RouteTransformations{
			ClearRouteCache: true,
		}
		configAny, err := pluginutils.MessageToAny(t)
		Expect(err).NotTo(HaveOccurred())

		expected = configAny
	})

	It("sets transformation config for weighted destinations", func() {
		out := &envoyroute.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				Transformations: t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
	})
	It("sets transformation config for virtual hosts", func() {
		out := &envoyroute.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				Transformations: t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
	})
	It("sets transformation config for routes", func() {

		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				Transformations: t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.TypedPerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
	})
})
