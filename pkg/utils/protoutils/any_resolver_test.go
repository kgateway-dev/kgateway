package protoutils_test

import (
	"bytes"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_http_buffer_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	envoy_transformation "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
)

var _ = Describe("AnyResolver", func() {

	It("can unmarshal a message with go and gogo anys", func() {
		golang := &envoy_extensions_filters_http_buffer_v3.BufferPerRoute{
			Override: &envoy_extensions_filters_http_buffer_v3.BufferPerRoute_Buffer{
				Buffer: &envoy_extensions_filters_http_buffer_v3.Buffer{
					MaxRequestBytes: &wrappers.UInt32Value{
						Value: 32,
					},
				},
			},
		}
		golangAny, err := ptypes.MarshalAny(golang)
		Expect(err).NotTo(HaveOccurred())

		gogo := &transformation.Transformations{
			RequestTransformation: &envoy_transformation.Transformation{
				TransformationType: &envoy_transformation.Transformation_TransformationTemplate{
					TransformationTemplate: &envoy_transformation.TransformationTemplate{
						AdvancedTemplates: true,
					},
				},
			},
		}
		gogoAny, err := types.MarshalAny(gogo)
		Expect(err).NotTo(HaveOccurred())
		vhosts := []*envoy_config_route_v3.VirtualHost{
			{
				Name:    "golang_filter",
				Domains: []string{"*"},
				TypedPerFilterConfig: map[string]*any.Any{
					"filter_name": {
						TypeUrl: golangAny.GetTypeUrl(),
						Value:   golangAny.GetValue(),
					},
				},
			},
			{
				Name:    "gogo_filter",
				Domains: []string{"*"},
				TypedPerFilterConfig: map[string]*any.Any{
					"filter_name": {
						TypeUrl: gogoAny.GetTypeUrl(),
						Value:   gogoAny.GetValue(),
					},
				},
			},
		}

		rc := &envoy_config_route_v3.RouteConfiguration{VirtualHosts: vhosts}

		buf := &bytes.Buffer{}
		marshaler := &jsonpb.Marshaler{
			AnyResolver: &protoutils.MultiAnyResolver{},
			OrigName:    true,
		}
		Expect(marshaler.Marshal(buf, rc)).NotTo(HaveOccurred())
		json := string(buf.Bytes())
		expected := `{"virtual_hosts":[{"name":"golang_filter","domains":["*"],"typed_per_filter_config":{"filter_name":{"@type":"type.googleapis.com/envoy.extensions.filters.http.buffer.v3.BufferPerRoute","buffer":{"max_request_bytes":32}}}},{"name":"gogo_filter","domains":["*"],"typed_per_filter_config":{"filter_name":{"@type":"type.googleapis.com/transformation.options.gloo.solo.io.Transformations","request_transformation":{"transformation_template":{"advanced_templates":true}}}}}]}`
		ExpectWithOffset(1, json).To(Equal(expected))

	})

})
