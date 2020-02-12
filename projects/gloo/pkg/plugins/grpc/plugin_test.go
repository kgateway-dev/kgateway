package grpc

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	pluginsv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	v1grpc "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	v1kube "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
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
		grpcSepc     *pluginsv1.ServiceSpec_Grpc
	)

	BeforeEach(func() {
		b := false
		p = NewPlugin(&b)
		out = new(envoyapi.Cluster)

		grpcSepc = &pluginsv1.ServiceSpec_Grpc{
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
				PluginType: grpcSepc,
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
		It("should not mark none grpc upstreams as http2", func() {
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
		It("should process route", func() {

			var routeParams plugins.RouteParams
			routeIn := &v1.Route{
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							Single: &v1.Destination{
								DestinationSpec: &v1.DestinationSpec{
									DestinationType: &v1.DestinationSpec_Grpc{
										Grpc: &v1grpc.DestinationSpec{},
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
		})

	})

	Context("filters", func() {

		var (
			upstream1     *v1.Upstream
			upstreamSpec1 *v1kube.UpstreamSpec
			grpcSpec1     *pluginsv1.ServiceSpec_Grpc

			entryList     [][]ServicesAndDescriptor
		)

		BeforeEach(func() {

			Expect(p.upstreamServices).To(BeEmpty())

			grpcSpec1 = &pluginsv1.ServiceSpec_Grpc{
				Grpc: &v1grpc.ServiceSpec{
					GrpcServices: []*v1grpc.ServiceSpec_GrpcService{{
						PackageName:   "foo",
						ServiceName:   "baz",
					}},
					Descriptors: []byte("randomString"),
				},
			}

			upstreamSpec1 = &v1kube.UpstreamSpec{
				ServiceSpec: &pluginsv1.ServiceSpec{
					PluginType: grpcSpec1,
				},
			}

			upstream1 = &v1.Upstream{
				Metadata: core.Metadata{
					Name:      "test",
					Namespace: "default",
				},
				UpstreamType: &v1.Upstream_Kube{
					Kube: upstreamSpec1,
				},
			}

			// process upstreams
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			err = p.ProcessUpstream(params, upstream1, out)
			Expect(err).NotTo(HaveOccurred())

			// modify upstreamServices for specific case
			Expect(p.upstreamServices).To(HaveLen(2))
			p.upstreamServices = append(p.upstreamServices, ServicesAndDescriptor{
				Spec:        p.upstreamServices[0].Spec,
				Descriptors: p.upstreamServices[1].Descriptors,
			})

			// check that values we sort on are different
			spec0 := p.upstreamServices[0].Spec.String()
			spec1 := p.upstreamServices[1].Spec.String()
			spec2 := p.upstreamServices[2].Spec.String()

			descriptors0 := p.upstreamServices[0].Descriptors.String()
			descriptors1 := p.upstreamServices[1].Descriptors.String()
			descriptors2 := p.upstreamServices[2].Descriptors.String()

			Expect(spec2).To(Equal(spec0))
			Expect(spec2).NotTo(Equal(spec1))

			Expect(descriptors2).To(Equal(descriptors1))
			Expect(descriptors2).NotTo(Equal(descriptors0))

			// build all permutations of p.upstreamServices
			entryList = append(entryList,
				[]ServicesAndDescriptor{p.upstreamServices[0], p.upstreamServices[1], p.upstreamServices[2]},
				[]ServicesAndDescriptor{p.upstreamServices[0], p.upstreamServices[2], p.upstreamServices[1]},
				[]ServicesAndDescriptor{p.upstreamServices[1], p.upstreamServices[0], p.upstreamServices[2]},
				[]ServicesAndDescriptor{p.upstreamServices[1], p.upstreamServices[2], p.upstreamServices[0]},
				[]ServicesAndDescriptor{p.upstreamServices[2], p.upstreamServices[0], p.upstreamServices[1]},
				[]ServicesAndDescriptor{p.upstreamServices[2], p.upstreamServices[1], p.upstreamServices[0]},
			)

		})

		DescribeTable("should always have filters in the same order",
			func(entryListIndex int) {
				// set p.upstreamServices to the proper permutation for this entry
				p.upstreamServices = entryList[entryListIndex]
				Expect(p.upstreamServices).To(HaveLen(3))

				// run HttpFilters which will sort p.upstreamServices
				filters, err := p.HttpFilters(params, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(filters).NotTo(BeEmpty())

				// check that we have 3 grpc_json_transcoder filters
				Expect(filters).To(HaveLen(3))
				Expect(filters[0].HttpFilter.Name).To(Equal(filterName))
				Expect(filters[1].HttpFilter.Name).To(Equal(filterName))
				Expect(filters[2].HttpFilter.Name).To(Equal(filterName))

				// check that the order of filter names afterward is always the same
				Expect(filters[0].HttpFilter.GetConfig().Fields["services"].GetListValue().Values[0].GetStringValue()).To(Equal("foo.baz"))
				Expect(filters[1].HttpFilter.GetConfig().Fields["services"].GetListValue().Values[0].GetStringValue()).To(Equal("foo.bar"))
				Expect(filters[2].HttpFilter.GetConfig().Fields["services"].GetListValue().Values[0].GetStringValue()).To(Equal("foo.bar"))

				// check that the odd one out for the filter file descriptor is always the same
				Expect(filters[0].HttpFilter.GetConfig().Fields["protoDescriptorBin"].GetStringValue()).NotTo(Equal(
					filters[1].HttpFilter.GetConfig().Fields["protoDescriptorBin"].GetStringValue()))
				Expect(filters[0].HttpFilter.GetConfig().Fields["protoDescriptorBin"].GetStringValue()).To(Equal(
					filters[2].HttpFilter.GetConfig().Fields["protoDescriptorBin"].GetStringValue()))

				// check that the odd one out for the upstreamServices spec and descriptors is always the same
				Expect(p.upstreamServices[0].Spec.String()).NotTo(Equal(
					p.upstreamServices[1].Spec.String()))
				Expect(p.upstreamServices[1].Spec.String()).To(Equal(
					p.upstreamServices[2].Spec.String()))

				Expect(p.upstreamServices[0].Descriptors.String()).NotTo(Equal(
					p.upstreamServices[1].Descriptors.String()))
				Expect(p.upstreamServices[0].Descriptors.String()).To(Equal(
					p.upstreamServices[2].Descriptors.String()))

			},

			Entry("for the 0th permutation", 0),
			Entry("for the 1st permutation", 1),
			Entry("for the 2nd permutation", 2),
			Entry("for the 3rd permutation", 3),
			Entry("for the 4th permutation", 4),
			Entry("for the 5th permutation", 5),

			)
	})

})
