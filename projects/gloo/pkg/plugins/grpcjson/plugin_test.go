package grpcjson_test

import (
	envoy_extensions_filters_http_grpc_json_transcoder_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_json_transcoder/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpcjson"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/test/matchers"
)

var _ = Describe("GrpcJson", func() {

	var (
		initParams     plugins.InitParams
		expectedFilter []plugins.StagedHttpFilter
	)

	envoyGrpcJsonConf := &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder{
		DescriptorSet: &envoy_extensions_filters_http_grpc_json_transcoder_v3.GrpcJsonTranscoder_ProtoDescriptor{ProtoDescriptor: "/path/to/file"},
		Services:      []string{"main.Bookstore"},
	}

	BeforeEach(func() {

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

	It("should add filter and translate fields", func() {
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

})
