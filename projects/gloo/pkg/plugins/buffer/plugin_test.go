package buffer_test

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoybuffer "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/buffer"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var _ = Describe("Plugin", func() {
	It("copies the buffer config from the listener to the filter", func() {
		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Buffer: &v3.Buffer{
					MaxRequestBytes: &types.UInt32Value{
						Value: 2048,
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			plugins.StagedHttpFilter{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: wellknown.Buffer,
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: utils.MustMessageToAny(&envoybuffer.Buffer{
							MaxRequestBytes: &wrappers.UInt32Value{Value: 2048.000000},
						}),
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
				BufferPerRoute: &v3.BufferPerRoute{
					Override: &v3.BufferPerRoute_Disabled{
						Disabled: true,
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetDisabled()).To(Equal(true))
	})

	It("allows route specific buffer config", func() {
		p := NewPlugin()
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				BufferPerRoute: &v3.BufferPerRoute{
					Override: &v3.BufferPerRoute_Buffer{
						Buffer: &v3.Buffer{
							MaxRequestBytes: &types.UInt32Value{
								Value: 4098,
							},
						},
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetBuffer().GetMaxRequestBytes().GetValue()).To(Equal(uint32(4098)))
	})

	It("allows vhost specific disabling of buffer", func() {
		p := NewPlugin()
		out := &envoyroute.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				BufferPerRoute: &v3.BufferPerRoute{
					Override: &v3.BufferPerRoute_Disabled{
						Disabled: true,
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetDisabled()).To(Equal(true))
	})

	It("allows vhost specific buffer config", func() {
		p := NewPlugin()
		out := &envoyroute.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				BufferPerRoute: &v3.BufferPerRoute{
					Override: &v3.BufferPerRoute_Buffer{
						Buffer: &v3.Buffer{
							MaxRequestBytes: &types.UInt32Value{
								Value: 4098,
							},
						},
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetBuffer().GetMaxRequestBytes().GetValue()).To(Equal(uint32(4098)))
	})

	It("allows weighted destination specific disabling of buffer", func() {
		p := NewPlugin()
		out := &envoyroute.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				BufferPerRoute: &v3.BufferPerRoute{
					Override: &v3.BufferPerRoute_Disabled{
						Disabled: true,
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetDisabled()).To(Equal(true))
	})

	It("allows weighted destination specific buffer config", func() {
		p := NewPlugin()
		out := &envoyroute.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				BufferPerRoute: &v3.BufferPerRoute{
					Override: &v3.BufferPerRoute_Buffer{
						Buffer: &v3.Buffer{
							MaxRequestBytes: &types.UInt32Value{
								Value: 4098,
							},
						},
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = conversion.StructToMessage(out.GetPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetBuffer().GetMaxRequestBytes().GetValue()).To(Equal(uint32(4098)))
	})

})
