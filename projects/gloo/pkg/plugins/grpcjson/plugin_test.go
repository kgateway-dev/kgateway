package grpcjson_test

import (
	envoy_extensions_filters_http_grpc_json_transcoder_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_json_transcoder/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpcjson"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("GrpcJson", func() {

	var (
		initParams plugins.InitParams
	)

	It("should add filter and translate fields", func() {
		envoyGrpcJsonConf := &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder{
			DescriptorSet: &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_ProtoDescriptor{ProtoDescriptor: "/path/to/file"},
			Services:      []string{"main.Bookstore"},
		}
		any, err := utils.MessageToAny(envoyGrpcJsonConf)
		Expect(err).ToNot(HaveOccurred())
		expectedFilter := []plugins.StagedHttpFilter{
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

		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
					DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptor{ProtoDescriptor: "/path/to/file"},
					Services:      []string{"main.Bookstore"},
				},
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
