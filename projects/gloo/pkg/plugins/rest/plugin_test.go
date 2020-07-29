package rest

import (
	"regexp"

	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/utils"
	envoy_transform "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	pluginsv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	v1rest "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/rest"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	transformapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
)

var _ = Describe("Plugin", func() {

	var (
		p            *plugin
		params       plugins.Params
		upstream     *v1.Upstream
		upstreamSpec *v1static.UpstreamSpec
		out          *envoyapi.Cluster
		restSpec     *pluginsv1.ServiceSpec_Rest
	)

	BeforeEach(func() {
		b := false
		p = NewPlugin(&b).(*plugin)
		out = new(envoyapi.Cluster)

		restSpec = &pluginsv1.ServiceSpec_Rest{
			Rest: &v1rest.ServiceSpec{
				Transformations: map[string]*envoy_transform.TransformationTemplate{
					"func": &envoy_transform.TransformationTemplate{},
				},
			},
		}

		p.Init(plugins.InitParams{})
		upstreamSpec = &v1static.UpstreamSpec{
			ServiceSpec: &pluginsv1.ServiceSpec{
				PluginType: restSpec,
			},
			Hosts: []*v1static.Host{{
				Addr: "localhost",
				Port: 1234,
			}},
		}
		upstream = &v1.Upstream{
			Metadata: core.Metadata{
				Name:      "test",
				Namespace: "default",
			},
			UpstreamType: &v1.Upstream_Static{
				Static: upstreamSpec,
			},
		}

	})
	Context("upstream", func() {
		It("should process the upstream", func() {
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).ToNot(HaveOccurred())
		})
		It("fails if the service spec is empty", func() {
			upstream.UpstreamType.(*v1.Upstream_Static).Static.ServiceSpec.PluginType.(*pluginsv1.ServiceSpec_Rest).Rest = nil
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
		})
	})
	Context("route", func() {
		var (
			ps       *transformapi.Parameters
			routeIn  *v1.Route
			routeOut *envoyroute.Route
		)

		BeforeEach(func() {
			ps = &transformapi.Parameters{
				Path: &types.StringValue{Value: "/{what}/{ ever }/{nested.field}/too"},
			}
			routeIn = &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationSpec: &v1.DestinationSpec{
									DestinationType: &v1.DestinationSpec_Rest{
										Rest: &v1rest.DestinationSpec{
											FunctionName: "func",
											Parameters:   ps,
										},
									},
								},
								DestinationType: &v1.Destination_Upstream{
									Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
								},
							},
						},
					},
				},
			}

			routeOut = &envoyroute.Route{
				Match: &envoyroute.RouteMatch{
					PathSpecifier: &envoyroute.RouteMatch_Prefix{Prefix: "/"},
				},
				Action: &envoyroute.Route_Route{
					Route: &envoyroute.RouteAction{},
				},
			}

		})

		It("should process route", func() {
			var routeParams plugins.RouteParams
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			err = p.ProcessRoute(routeParams, routeIn, routeOut)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoy_transform.RouteTransformations
			goTypedConfig := routeOut.GetTypedPerFilterConfig()[transformation.FilterName]
			gogoTypedConfig := &types.Any{TypeUrl: goTypedConfig.TypeUrl, Value: goTypedConfig.Value}
			err = types.UnmarshalAny(gogoTypedConfig, &cfg)
			Expect(err).NotTo(HaveOccurred())

			extrs := cfg.GetRequestTransformation().GetTransformationTemplate().GetExtractors()
			Expect(extrs["what"].GetHeader()).To(Equal(":path"))
			Expect(extrs["what"].GetSubgroup()).To(Equal(uint32(1)))

			Expect(extrs["ever"].GetHeader()).To(Equal(":path"))
			Expect(extrs["ever"].GetSubgroup()).To(Equal(uint32(2)))

			Expect(extrs["nested.field"].GetHeader()).To(Equal(":path"))
			Expect(extrs["nested.field"].GetSubgroup()).To(Equal(uint32(3)))
		})

		It("should produce path extractors that can match URLs", func() {
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())

			var routeParams plugins.RouteParams
			err = p.ProcessRoute(routeParams, routeIn, routeOut)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoy_transform.RouteTransformations
			goTypedConfig := routeOut.GetTypedPerFilterConfig()[transformation.FilterName]
			gogoTypedConfig := &types.Any{TypeUrl: goTypedConfig.TypeUrl, Value: goTypedConfig.Value}
			err = types.UnmarshalAny(gogoTypedConfig, &cfg)
			Expect(err).NotTo(HaveOccurred())

			extrs := cfg.GetRequestTransformation().GetTransformationTemplate().GetExtractors()

			matchablePath := "/first_value/second%34value/third-value/too"
			compiledRe, err := regexp.Compile(extrs["what"].Regex)
			subMatches := compiledRe.FindStringSubmatch(matchablePath)
			Expect(subMatches).NotTo(BeNil())
			// We expect the entire string to match since this is what the matching code in
			// https://github.com/solo-io/envoy-transformation/blob/289d945b0a85c9df92918c478caa016020bbe981/source/extensions/filters/http/transformation/transformer.cc#L50
			// expects as well.
			Expect(len(subMatches[0])).To(Equal(len(matchablePath)))
			Expect(subMatches[extrs["what"].Subgroup]).To(Equal("first_value"))
			Expect(subMatches[extrs["ever"].Subgroup]).To(Equal("second%34value"))
			Expect(subMatches[extrs["nested.field"].Subgroup]).To(Equal("third-value"))
		})
	})
})
