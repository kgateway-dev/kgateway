package grpc

import (
	"regexp"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	envoy_transform "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	pluginsv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	v1grpc "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	transformapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {

	var (
		p            *plugin
		params       plugins.Params
		upstream     *v1.Upstream
		upstreamSpec *v1static.UpstreamSpec
		out          *envoy_config_cluster_v3.Cluster
		grpcSpec     *pluginsv1.ServiceSpec_Grpc
	)

	BeforeEach(func() {
		p = NewPlugin()
		out = new(envoy_config_cluster_v3.Cluster)

		grpcSpec = &pluginsv1.ServiceSpec_Grpc{
			Grpc: &v1grpc.ServiceSpec{
				GrpcServices: []*v1grpc.ServiceSpec_GrpcService{{
					PackageName:   "foo",
					ServiceName:   "bar",
					FunctionNames: []string{"func"},
				}},
			},
		}

		p.Init(plugins.InitParams{})
		upstreamSpec = &v1static.UpstreamSpec{
			ServiceSpec: &pluginsv1.ServiceSpec{
				PluginType: grpcSpec,
			},
			Hosts: []*v1static.Host{{
				Addr: "localhost",
				Port: 1234,
			}},
		}
		upstream = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "test",
				Namespace: "default",
			},
			UpstreamType: &v1.Upstream_Static{
				Static: upstreamSpec,
			},
		}
	})
	Context("upstream", func() {
		It("should not mark non-grpc upstreams as http2", func() {
			upstreamSpec.ServiceSpec.PluginType = nil
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Http2ProtocolOptions).To(BeNil())
		})

		It("should mark grpc upstreams as http2", func() {
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Http2ProtocolOptions).NotTo(BeNil())
		})
	})

	Context("route", func() {
		var (
			ps       *transformapi.Parameters
			routeIn  *v1.Route
			routeOut *envoy_config_route_v3.Route
		)

		BeforeEach(func() {
			ps = &transformapi.Parameters{
				Path: &wrappers.StringValue{Value: "/{what}/{ ever }/{nested.field}/too"},
				Headers: map[string]string{
					"header-simple":            "{simple}",
					"header-simple-with-space": "{ simple_with_space }",
					"header-nested":            "{something.nested}",
				},
			}
			routeIn = &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationSpec: &v1.DestinationSpec{
									DestinationType: &v1.DestinationSpec_Grpc{
										Grpc: &v1grpc.DestinationSpec{
											Package:    "foo",
											Service:    "bar",
											Function:   "func",
											Parameters: ps,
										},
									},
								},
								DestinationType: &v1.Destination_Upstream{
									Upstream: upstream.Metadata.Ref(),
								},
							},
						},
					},
				},
			}
			routeOut = &envoy_config_route_v3.Route{
				Match: &envoy_config_route_v3.RouteMatch{
					PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{Prefix: "/"},
				},
				Action: &envoy_config_route_v3.Route_Route{
					Route: &envoy_config_route_v3.RouteAction{},
				},
			}
		})
		It("should process route", func() {
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())

			var routeParams plugins.RouteParams
			err = p.ProcessRoute(routeParams, routeIn, routeOut)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoy_transform.RouteTransformations
			goTypedConfig := routeOut.GetTypedPerFilterConfig()[transformation.FilterName]
			err = ptypes.UnmarshalAny(goTypedConfig, &cfg)
			Expect(err).NotTo(HaveOccurred())

			tt := cfg.GetRequestTransformation().GetTransformationTemplate()
			Expect(tt.GetMergeExtractorsToBody()).NotTo(BeNil())

			extrs := tt.GetExtractors()
			Expect(extrs["what"].GetHeader()).To(Equal(":path"))
			Expect(extrs["what"].GetSubgroup()).To(Equal(uint32(1)))

			Expect(extrs["ever"].GetHeader()).To(Equal(":path"))
			Expect(extrs["ever"].GetSubgroup()).To(Equal(uint32(2)))

			Expect(extrs["nested.field"].GetHeader()).To(Equal(":path"))
			Expect(extrs["nested.field"].GetSubgroup()).To(Equal(uint32(3)))

			Expect(extrs["simple"].GetHeader()).To(Equal("header-simple"))
			Expect(extrs["simple"].GetSubgroup()).To(Equal(uint32(1)))

			Expect(extrs["simple_with_space"].GetHeader()).To(Equal("header-simple-with-space"))
			Expect(extrs["simple_with_space"].GetSubgroup()).To(Equal(uint32(1)))

			Expect(extrs["something.nested"].GetHeader()).To(Equal("header-nested"))
			Expect(extrs["something.nested"].GetSubgroup()).To(Equal(uint32(1)))

		})

		It("should produce path extractors that can match URLs", func() {
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())

			var routeParams plugins.RouteParams
			err = p.ProcessRoute(routeParams, routeIn, routeOut)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoy_transform.RouteTransformations
			goTypedConfig := routeOut.GetTypedPerFilterConfig()[transformation.FilterName]
			err = ptypes.UnmarshalAny(goTypedConfig, &cfg)
			Expect(err).NotTo(HaveOccurred())

			tt := cfg.GetRequestTransformation().GetTransformationTemplate()
			Expect(tt.GetMergeExtractorsToBody()).NotTo(BeNil())

			extrs := tt.GetExtractors()
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
