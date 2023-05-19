package grpcjson_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_http_grpc_json_transcoder_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_json_transcoder/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpcjson"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("GrpcJson", func() {

	var (
		initParams       plugins.InitParams
		glooGrpcJsonConf = &grpc_json.GrpcJsonTranscoder{
			DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptor{ProtoDescriptor: "/path/to/file"},
			Services:      []string{"main.Bookstore"},
		}
		envoyGrpcJsonConf = &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder{
			DescriptorSet: &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_ProtoDescriptor{ProtoDescriptor: "/path/to/file"},
			Services:      []string{"main.Bookstore"},
		}
		any, _         = utils.MessageToAny(envoyGrpcJsonConf)
		expectedFilter = []plugins.StagedHttpFilter{
			{
				HttpFilter: &envoyhttp.HttpFilter{
					Name: wellknown.GRPCJSONTranscoder,
					ConfigType: &envoyhttp.HttpFilter_TypedConfig{
						TypedConfig: any,
					},
				},
				Stage: plugins.BeforeStage(plugins.OutAuthStage),
			},
		}
	)

	It("should add filter and translate fields", func() {

		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				GrpcJsonTranscoder: glooGrpcJsonConf,
			},
		}

		p := grpcjson.NewPlugin()
		p.Init(initParams)
		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).NotTo(BeNil())
		Expect(f).To(HaveLen(1))
		Expect(f).To(matchers.BeEquivalentToDiff(expectedFilter))
	})
	It("Adds filters for grpc upstreams on routes", func() {
		us := &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "testUs",
				Namespace: "gloo-system",
			},
			UpstreamType: &v1.Upstream_Kube{
				Kube: &kubernetes.UpstreamSpec{
					ServiceSpec: &options.ServiceSpec{
						PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
							GrpcJsonTranscoder: glooGrpcJsonConf,
						},
					},
				},
			},
		}
		route := &v1.Route{
			Name: "host1_route1",
			Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      us.Metadata.Name,
									Namespace: us.Metadata.Namespace},
							},
						},
					},
				},
			},
		}
		outRoute := &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{},
			},
		}
		vhost := &v1.VirtualHost{Routes: []*v1.Route{route}}
		hl := &v1.HttpListener{VirtualHosts: []*v1.VirtualHost{vhost}}
		p := grpcjson.NewPlugin()
		p.Init(initParams)
		err := p.ProcessUpstream(plugins.Params{}, us, &envoy_config_cluster_v3.Cluster{})
		Expect(err).NotTo(HaveOccurred())
		err = p.ProcessRoute(plugins.RouteParams{VirtualHostParams: plugins.VirtualHostParams{HttpListener: hl}}, route, outRoute)
		Expect(err).NotTo(HaveOccurred())
		routeFilter, ok := outRoute.TypedPerFilterConfig[wellknown.GRPCJSONTranscoder]
		Expect(ok).To(BeTrue())
		Expect(routeFilter).To(matchers.BeEquivalentToDiff(expectedFilter[0].HttpFilter.GetTypedConfig()))
		listenerFilter, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).NotTo(HaveOccurred())

		Expect(listenerFilter).NotTo(BeNil())
		Expect(listenerFilter).To(HaveLen(1))
		// The filter should be a dummy filter to be overridden by route specific filters
		Expect(listenerFilter[0]).NotTo(matchers.BeEquivalentToDiff(expectedFilter[0].HttpFilter.GetTypedConfig()))
	})
	It("Does not create an empty filter on listener when no routes with gRPC are not configured", func() {
		us := &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "testUs",
				Namespace: "gloo-system",
			},
			UpstreamType: &v1.Upstream_Kube{
				Kube: &kubernetes.UpstreamSpec{},
			},
		}
		route := &v1.Route{
			Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Name:      us.Metadata.Name,
									Namespace: us.Metadata.Namespace},
							},
						},
					},
				},
			},
		}
		p := grpcjson.NewPlugin()
		p.Init(initParams)
		err := p.ProcessUpstream(plugins.Params{}, us, &envoy_config_cluster_v3.Cluster{})
		Expect(err).NotTo(HaveOccurred())
		vhost := &v1.VirtualHost{Routes: []*v1.Route{route}}
		hl := &v1.HttpListener{VirtualHosts: []*v1.VirtualHost{vhost}}
		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(f)).To(Equal(0))
	})
	It("Doesn't create empty filters on listeners when gRPC upstreams are configured but not referenced by routes on that listener", func() {
		us := &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "testUs",
				Namespace: "gloo-system",
			},
			UpstreamType: &v1.Upstream_Kube{
				Kube: &kubernetes.UpstreamSpec{
					ServiceSpec: &options.ServiceSpec{
						PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
							GrpcJsonTranscoder: glooGrpcJsonConf,
						},
					},
				},
			},
		}

		p := grpcjson.NewPlugin()
		p.Init(initParams)
		err := p.ProcessUpstream(plugins.Params{}, us, &envoy_config_cluster_v3.Cluster{})
		Expect(err).NotTo(HaveOccurred())
		hl := &v1.HttpListener{VirtualHosts: []*v1.VirtualHost{}}
		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(f)).To(Equal(0))
	})
	Context("proto descriptor configmap", func() {
		var (
			snap           *gloosnapshot.ApiSnapshot
			hl             *v1.HttpListener
			expectedFilter []plugins.StagedHttpFilter
		)

		BeforeEach(func() {
			// add an artifact (configmap) containing the proto descriptor data to the snapshot
			snap = &gloosnapshot.ApiSnapshot{
				Artifacts: v1.ArtifactList{
					&v1.Artifact{
						Metadata: &core.Metadata{
							Name:      "my-config-map",
							Namespace: "gloo-system",
						},
						Data: map[string]string{
							"protoDesc": "aGVsbG8K",
						},
					},
				},
			}
			hl = &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
						DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap{
							ProtoDescriptorConfigMap: &grpc_json.GrpcJsonTranscoder_DescriptorConfigMap{
								ConfigMapRef: &core.ResourceRef{Name: "my-config-map", Namespace: "gloo-system"},
							},
						},
						Services: []string{"main.Bookstore"},
					},
				},
			}

			envoyGrpcJsonConf := &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder{
				DescriptorSet: &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_ProtoDescriptorBin{
					ProtoDescriptorBin: []byte("hello\n"),
				},
				Services: []string{"main.Bookstore"},
			}
			any, err := utils.MessageToAny(envoyGrpcJsonConf)
			Expect(err).ToNot(HaveOccurred())
			expectedFilter = []plugins.StagedHttpFilter{
				{
					HttpFilter: &envoyhttp.HttpFilter{
						Name: wellknown.GRPCJSONTranscoder,
						ConfigType: &envoyhttp.HttpFilter_TypedConfig{
							TypedConfig: any,
						},
					},
					Stage: plugins.BeforeStage(plugins.OutAuthStage),
				},
			}
		})

		It("should return error if no configmap ref is provided", func() {
			hl.Options.GrpcJsonTranscoder.DescriptorSet.(*grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap).ProtoDescriptorConfigMap.ConfigMapRef = nil

			p := grpcjson.NewPlugin()
			p.Init(initParams)
			_, err := p.HttpFilters(plugins.Params{
				Snapshot: snap,
			}, hl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(grpcjson.NoConfigMapRefError().Error()))
		})

		It("should return error if specified configmap does not exist", func() {
			hl.Options.GrpcJsonTranscoder.DescriptorSet.(*grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap).ProtoDescriptorConfigMap.ConfigMapRef =
				&core.ResourceRef{Name: "does-not-exist", Namespace: "gloo-system"}

			p := grpcjson.NewPlugin()
			p.Init(initParams)
			_, err := p.HttpFilters(plugins.Params{
				Snapshot: snap,
			}, hl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(grpcjson.ConfigMapNotFoundError(hl.GetOptions().GetGrpcJsonTranscoder().GetProtoDescriptorConfigMap()).Error()))
		})

		It("should return error if configmap has no data", func() {
			snap.Artifacts[0].Data = nil

			p := grpcjson.NewPlugin()
			p.Init(initParams)
			_, err := p.HttpFilters(plugins.Params{
				Snapshot: snap,
			}, hl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(grpcjson.ConfigMapNoValuesError(hl.GetOptions().GetGrpcJsonTranscoder().GetProtoDescriptorConfigMap()).Error()))
		})

		It("should use protodescriptor specified by key", func() {
			snap.Artifacts[0].Data["another-key"] = "another"
			hl.Options.GrpcJsonTranscoder.DescriptorSet.(*grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap).ProtoDescriptorConfigMap.Key = "protoDesc"

			p := grpcjson.NewPlugin()
			p.Init(initParams)
			f, err := p.HttpFilters(plugins.Params{
				Snapshot: snap,
			}, hl)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
			Expect(f).To(HaveLen(1))
			Expect(f).To(matchers.BeEquivalentToDiff(expectedFilter))
		})

		It("should default to first mapping if there is only one mapping", func() {
			// no Key is explicitly set in the ProtoDescriptorConfigMap
			p := grpcjson.NewPlugin()
			p.Init(initParams)
			f, err := p.HttpFilters(plugins.Params{
				Snapshot: snap,
			}, hl)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
			Expect(f).To(HaveLen(1))
			Expect(f).To(matchers.BeEquivalentToDiff(expectedFilter))
		})

		It("should return error if specified key does not exist", func() {
			hl.Options.GrpcJsonTranscoder.DescriptorSet.(*grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap).ProtoDescriptorConfigMap.Key = "nonexistent-key"

			p := grpcjson.NewPlugin()
			p.Init(initParams)
			_, err := p.HttpFilters(plugins.Params{
				Snapshot: snap,
			}, hl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(grpcjson.NoDataError(hl.GetOptions().GetGrpcJsonTranscoder().GetProtoDescriptorConfigMap(), "nonexistent-key").Error()))
		})

		It("should return error if no key is specified and there are multiple mappings", func() {
			snap.Artifacts[0].Data["another-key"] = "another"

			p := grpcjson.NewPlugin()
			p.Init(initParams)
			_, err := p.HttpFilters(plugins.Params{
				Snapshot: snap,
			}, hl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(grpcjson.NoConfigMapKeyError(hl.GetOptions().GetGrpcJsonTranscoder().GetProtoDescriptorConfigMap(), 2).Error()))
		})

		It("should return error if data is not base64-encoded", func() {
			snap.Artifacts[0].Data["protoDesc"] = "hello"

			p := grpcjson.NewPlugin()
			p.Init(initParams)
			_, err := p.HttpFilters(plugins.Params{
				Snapshot: snap,
			}, hl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(grpcjson.DecodingError(hl.GetOptions().GetGrpcJsonTranscoder().GetProtoDescriptorConfigMap(), "protoDesc").Error()))
		})
	})

})
