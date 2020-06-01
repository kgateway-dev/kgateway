package buffer_test

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoybuffer "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/buffer/v2"
	envoy_config_filter_network_http_connection_manager_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/gogo/protobuf/types"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v2 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/filter/http/buffer/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/buffer"
)

// We need to test three possible input values for the BufferPerRoute.Disabled field
// - Undefined: no config is provided
// - Enabled: config explicitly enables buffer for this route
// - Disabled: config explicitly disables buffer for this route
type ConfigState int

const (
	Undefined ConfigState = iota
	Enabled
	Disabled
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

	It("allows route specific disabling of buffer", func() {
		p := NewPlugin()
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				BufferPerRoute: &v2.BufferPerRoute{
					Override: &v2.BufferPerRoute_Disabled{
						Disabled: true,
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[FilterName], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetDisabled()).To(Equal(true))
	})

	It("allows route specific buffer config", func() {
		p := NewPlugin()
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				BufferPerRoute: &v2.BufferPerRoute{
					Override: &v2.BufferPerRoute_Buffer{
						Buffer: &v2.Buffer{
							MaxRequestBytes: &types.UInt32Value{
								Value: 4098,
							},
						},
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[FilterName], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetBuffer().GetMaxRequestBytes().GetValue()).To(Equal(uint32(4098)))
	})

	It("allows vhost specific disabling of buffer", func() {
		p := NewPlugin()
		out := &envoyroute.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				BufferPerRoute: &v2.BufferPerRoute{
					Override: &v2.BufferPerRoute_Buffer{
						Buffer: &v2.Buffer{
							MaxRequestBytes: &types.UInt32Value{
								Value: 4098,
							},
						},
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[FilterName], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetBuffer().GetMaxRequestBytes().GetValue()).To(Equal(uint32(4098)))
	})

	It("allows vhost specific buffer config", func() {
		p := NewPlugin()
		out := &envoyroute.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				BufferPerRoute: &v2.BufferPerRoute{
					Override: &v2.BufferPerRoute_Disabled{
						Disabled: true,
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[FilterName], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetDisabled()).To(Equal(true))
	})

	It("allows weighted destination specific buffer config", func() {
		p := NewPlugin()
		out := &envoyroute.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				BufferPerRoute: &v2.BufferPerRoute{
					Override: &v2.BufferPerRoute_Disabled{
						Disabled: true,
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[FilterName], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetDisabled()).To(Equal(true))
	})

	It("allows weighted destination specific disabling of buffer", func() {
		p := NewPlugin()
		out := &envoyroute.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				BufferPerRoute: &v2.BufferPerRoute{
					Override: &v2.BufferPerRoute_Buffer{
						Buffer: &v2.Buffer{
							MaxRequestBytes: &types.UInt32Value{
								Value: 4098,
							},
						},
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[FilterName], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetBuffer().GetMaxRequestBytes().GetValue()).To(Equal(uint32(4098)))
	})

})
