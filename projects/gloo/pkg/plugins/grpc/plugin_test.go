package grpc

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/utils"
	envoy_transform "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	pluginsv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	v1grpc "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	transformapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
)

var _ = Describe("Plugin", func() {

	var (
		p            *plugin
		params       plugins.Params
		upstream     *v1.Upstream
		upstreamSpec *v1static.UpstreamSpec
		out          *envoyapi.Cluster
		grpcSpec     *pluginsv1.ServiceSpec_Grpc
	)

	BeforeEach(func() {
		b := false
		p = NewPlugin(&b)
		out = new(envoyapi.Cluster)

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

		It("should not accept connection streams that are too small", func() {
			tooSmall := &v1.Upstream{
				Metadata: core.Metadata{
					Name:      "test",
					Namespace: "default",
				},
				UpstreamType: &v1.Upstream_Static{
					Static: upstreamSpec,
				},
				InitialConnectionWindowSize: &types.UInt32Value{Value: 65534},
			}

			err := p.ProcessUpstream(params, tooSmall, out)
			Expect(err).To(HaveOccurred())
		})
		It("should not accept connection streams that are too large", func() {
			tooBig := &v1.Upstream{
				Metadata: core.Metadata{
					Name:      "test",
					Namespace: "default",
				},
				UpstreamType: &v1.Upstream_Static{
					Static: upstreamSpec,
				},
				InitialStreamWindowSize: &types.UInt32Value{Value: 2147483648},
			}
			err := p.ProcessUpstream(params, tooBig, out)
			Expect(err).To(HaveOccurred())
		})
		It("should accept connection streams that are within the correct range", func() {
			validUpstream := &v1.Upstream{
				Metadata: core.Metadata{
					Name:      "test",
					Namespace: "default",
				},
				UpstreamType: &v1.Upstream_Static{
					Static: upstreamSpec,
				},
				InitialStreamWindowSize:     &types.UInt32Value{Value: 268435457},
				InitialConnectionWindowSize: &types.UInt32Value{Value: 65535},
			}
			err := p.ProcessUpstream(params, validUpstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Http2ProtocolOptions).NotTo(BeNil())
			Expect(out.InitialStreamWindowSize).To(Equal(&wrappers.UInt32Value{Value: 268435457}))
			Expect(out.Http2ProtocolOptions.InitialConnectionWindowSize).To(Equal(&wrappers.UInt32Value{Value: 65535}))
		})
	})

	Context("route", func() {

		ps := &transformapi.Parameters{
			Path: &types.StringValue{Value: "/{what}/{ ever }/{nested.field}/too"},
			Headers: map[string]string{
				"header-simple":            "{simple}",
				"header-simple-with-space": "{ simple_with_space }",
				"header-nested":            "{something.nested}",
			},
		}

		It("should process route", func() {

			var routeParams plugins.RouteParams
			routeIn := &v1.Route{
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
									Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
								},
							},
						},
					},
				},
			}

			routeOut := &envoyroute.Route{
				Match: &envoyroute.RouteMatch{
					PathSpecifier: &envoyroute.RouteMatch_Prefix{Prefix: "/"},
				},
				Action: &envoyroute.Route_Route{
					Route: &envoyroute.RouteAction{},
				},
			}
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			err = p.ProcessRoute(routeParams, routeIn, routeOut)
			Expect(err).NotTo(HaveOccurred())

			var cfg envoy_transform.RouteTransformations
			goTypedConfig := routeOut.GetTypedPerFilterConfig()[transformation.FilterName]
			gogoTypedConfig := &types.Any{TypeUrl: goTypedConfig.TypeUrl, Value: goTypedConfig.Value}
			err = types.UnmarshalAny(gogoTypedConfig, &cfg)
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
	})
})
