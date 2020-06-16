package grpc

import (
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
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
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

		basicRegex := "([\\-._[:alnum:]]+)"
		complexRegex := "/([\\-._[:alnum:]]+)/([\\-._[:alnum:]]+)/([\\-._[:alnum:]]+)/too"
		expected := &envoy_transform.RouteTransformations{
			RequestTransformation: &envoy_transform.Transformation{
				TransformationType: &envoy_transform.Transformation_TransformationTemplate{
					TransformationTemplate: &envoy_transform.TransformationTemplate{
						Extractors: map[string]*envoy_transform.Extraction{
							"what": {
								Source:   &envoy_transform.Extraction_Header{":path"},
								Subgroup: 1,
								Regex:    complexRegex,
							},
							"ever": {
								Source:   &envoy_transform.Extraction_Header{":path"},
								Subgroup: 2,
								Regex:    complexRegex,
							},
							"method": {
								Source:   &envoy_transform.Extraction_Header{":method"},
								Subgroup: 1,
								Regex:    basicRegex,
							},
							"nested.field": {
								Source:   &envoy_transform.Extraction_Header{":path"},
								Subgroup: 3,
								Regex:    complexRegex,
							},
							"path": {
								Source:   &envoy_transform.Extraction_Header{":path"},
								Subgroup: 1,
								Regex:    basicRegex,
							},
							"simple": {
								Source:   &envoy_transform.Extraction_Header{"header-simple"},
								Subgroup: 1,
								Regex:    basicRegex,
							},
							"simple_with_space": {
								Source:   &envoy_transform.Extraction_Header{"header-simple-with-space"},
								Subgroup: 1,
								Regex:    basicRegex,
							},
							"something.nested": {
								Source:   &envoy_transform.Extraction_Header{"header-nested"},
								Subgroup: 1,
								Regex:    basicRegex,
							},
						},
						Headers: map[string]*envoy_transform.InjaTemplate{
							":method": {Text: "POST"},
							":path":   {Text: "/f4951089/test/foo.bar/func?{{ default(query_string, \"\")}}"},
						},
						BodyTransformation: &envoy_transform.TransformationTemplate_MergeExtractorsToBody{
							MergeExtractorsToBody: &envoy_transform.MergeExtractorsToBody{},
						},
					},
				},
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

			configStruct, err := pluginutils.MessageToAny(expected)
			Expect(err).NotTo(HaveOccurred())
			Expect(routeOut.GetTypedPerFilterConfig()).To(HaveKeyWithValue(transformation.FilterName, configStruct))
		})
	})
})
