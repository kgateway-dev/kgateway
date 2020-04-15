package transformation_test

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
)

var _ = Describe("Plugin", func() {
	var (
		p        *Plugin
		t        *transformation.RouteTransformations
		expected *structpb.Struct
	)

	BeforeEach(func() {
		p = NewPlugin()
		t = &transformation.RouteTransformations{
			ClearRouteCache: true,
			ResponseTransformation: &transformation.Transformation{
				TransformationType: &transformation.Transformation_TransformationTemplate{
					TransformationTemplate: &transformation.TransformationTemplate{
						AdvancedTemplates: false,
						Extractors:        nil,
						Headers: map[string]*transformation.InjaTemplate{
							":status": &transformation.InjaTemplate{Text: `{% if default(data.error.message, "") != "" %}400{% else %}{{
								header(":status") }}{% endif %}`},
						},
						BodyTransformation:    nil,
						ParseBodyBehavior:     0,
						IgnoreErrorOnParse:    false,
						DynamicMetadataValues: nil,
						XXX_NoUnkeyedLiteral:  struct{}{},
						XXX_unrecognized:      nil,
						XXX_sizecache:         0,
					},
				},
			},
		}
		configStruct, err := conversion.MessageToStruct(t)
		Expect(err).NotTo(HaveOccurred())

		expected = configStruct
	})

	It("sets transformation config for weighted destinations", func() {
		out := &envoyroute.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				Transformations: t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
	})
	FIt("sets transformation config for virtual hosts", func() {
		out := &envoyroute.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				Transformations: t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
	})
	It("sets transformation config for routes", func() {

		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				Transformations: t,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PerFilterConfig).To(HaveKeyWithValue(FilterName, expected))
	})
})
