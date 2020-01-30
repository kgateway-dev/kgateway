package gzip_test

import (
	envoy_config_filter_network_http_connection_manager_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/gogo/protobuf/types"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v2 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/filter/http/gzip/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/gzip"
)

var _ = Describe("Plugin", func() {
	It("copies the gzip config from the listener to the filter", func() {
		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Gzip: &v2.Gzip{
					MemoryLevel: &types.UInt32Value{
						Value: 10,
					},
					ContentLength: &types.UInt32Value{
						Value: 10,
					},
					CompressionLevel:    10,
					CompressionStrategy: 10,
					WindowBits: &types.UInt32Value{
						Value: 10,
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			plugins.StagedHttpFilter{
				HttpFilter: &envoy_config_filter_network_http_connection_manager_v2.HttpFilter{
					Name: "envoy.gzip",
					ConfigType: &envoy_config_filter_network_http_connection_manager_v2.HttpFilter_Config{
						Config: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"memoryLevel": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 10.000000,
									},
								},
								"contentLength": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 10.000000,
									},
								},
								"compressionLevel": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 10.000000,
									},
								},
								"compressionStrategy": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 10.000000,
									},
								},
								"windowBits": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 10.000000,
									},
								},
							},
						},
					},
				},
				Stage: plugins.FilterStage{
					RelativeTo: 8,
					Weight:     0,
				},
			},
		}))
	})
})
