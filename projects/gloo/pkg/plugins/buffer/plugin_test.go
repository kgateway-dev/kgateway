package buffer_test

import (
	envoy_config_filter_network_http_connection_manager_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/gogo/protobuf/types"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v2 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/filter/http/buffer/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/buffer"
)

var _ = Describe("Plugin", func() {
	It("copies the buffer config from the listener to the filter", func() {
		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Buffer: &v2.Buffer{
					MaxRequestBytes: &types.UInt32Value{
						Value: 2048,
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			plugins.StagedHttpFilter{
				HttpFilter: &envoy_config_filter_network_http_connection_manager_v2.HttpFilter{
					Name: "envoy.buffer",
					ConfigType: &envoy_config_filter_network_http_connection_manager_v2.HttpFilter_Config{
						Config: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"maxRequestBytes": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 2048.000000,
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
