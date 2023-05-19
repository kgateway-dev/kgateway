package buffer_test

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoybuffer "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/buffer"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Plugin", func() {
	It("copies the buffer config from the listener to the filter", func() {
		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Buffer: &v3.Buffer{
					MaxRequestBytes: &wrappers.UInt32Value{
						Value: 2048,
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		typedConfig, err := utils.MessageToAny(&envoybuffer.Buffer{
			MaxRequestBytes: &wrappers.UInt32Value{Value: 2048.000000},
		})
		Expect(err).NotTo(HaveOccurred())
		expectedStageFilter := plugins.StagedHttpFilter{
			HttpFilter: &envoyhcm.HttpFilter{
				Name: wellknown.Buffer,
				ConfigType: &envoyhcm.HttpFilter_TypedConfig{
					TypedConfig: typedConfig,
				},
			},

			Stage: plugins.DuringStage(plugins.RouteStage),
		}
		Expect(filters[0].HttpFilter).To(matchers.MatchProto(expectedStageFilter.HttpFilter))
		Expect(filters[0].Stage).To(Equal(expectedStageFilter.Stage))
	})

	It("allows route specific disabling of buffer", func() {
		p := NewPlugin()
		out := &envoy_config_route_v3.Route{}
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
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetDisabled()).To(Equal(true))
	})

	It("allows route specific buffer config", func() {
		p := NewPlugin()
		out := &envoy_config_route_v3.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				BufferPerRoute: &v3.BufferPerRoute{
					Override: &v3.BufferPerRoute_Buffer{
						Buffer: &v3.Buffer{
							MaxRequestBytes: &wrappers.UInt32Value{
								Value: 4098,
							},
						},
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetBuffer().GetMaxRequestBytes().GetValue()).To(Equal(uint32(4098)))
	})

	It("allows vhost specific disabling of buffer", func() {
		p := NewPlugin()
		out := &envoy_config_route_v3.VirtualHost{}
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
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetDisabled()).To(Equal(true))
	})

	It("allows vhost specific buffer config", func() {
		p := NewPlugin()
		out := &envoy_config_route_v3.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				BufferPerRoute: &v3.BufferPerRoute{
					Override: &v3.BufferPerRoute_Buffer{
						Buffer: &v3.Buffer{
							MaxRequestBytes: &wrappers.UInt32Value{
								Value: 4098,
							},
						},
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetBuffer().GetMaxRequestBytes().GetValue()).To(Equal(uint32(4098)))
	})

	It("allows weighted destination specific disabling of buffer", func() {
		p := NewPlugin()
		out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
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
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetDisabled()).To(Equal(true))
	})

	It("allows weighted destination specific buffer config", func() {
		p := NewPlugin()
		out := &envoy_config_route_v3.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				BufferPerRoute: &v3.BufferPerRoute{
					Override: &v3.BufferPerRoute_Buffer{
						Buffer: &v3.Buffer{
							MaxRequestBytes: &wrappers.UInt32Value{
								Value: 4098,
							},
						},
					},
				},
			},
		}, out)

		var cfg envoybuffer.BufferPerRoute
		err = ptypes.UnmarshalAny(out.GetTypedPerFilterConfig()[wellknown.Buffer], &cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.GetBuffer().GetMaxRequestBytes().GetValue()).To(Equal(uint32(4098)))
	})

})
