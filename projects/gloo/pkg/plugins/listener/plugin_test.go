package listener_test

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/listener"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Plugin", func() {

	var (
		plugin plugins.ListenerPlugin
		out    *envoy_config_listener_v3.Listener
	)

	BeforeEach(func() {
		out = new(envoy_config_listener_v3.Listener)
		plugin = NewPlugin()
	})

	Context("should wrap socket with TCP stats listener", func() {

		var (
			in                *v1.Listener
			hcm               *envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager
			stub_filterchains []*envoy_config_listener_v3.FilterChain
		)

		BeforeEach(func() {
			in = &v1.Listener{
				Options: &v1.ListenerOptions{
					TcpStats: &wrappers.BoolValue{
						Value: true,
					},
				},
			}

			hcm = &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
				StatPrefix: "placeholder",
				RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_Rds{
					Rds: &envoy_extensions_filters_network_http_connection_manager_v3.Rds{
						RouteConfigName: "routeName",
					},
				},
			}

			hcmAny, err := utils.MessageToAny(hcm)
			Expect(err).NotTo(HaveOccurred())

			stub_filterchains = []*envoy_config_listener_v3.FilterChain{
				{
					Name: "placeholder_filter_chain",
					Filters: []*envoy_config_listener_v3.Filter{
						{
							ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
								TypedConfig: hcmAny,
							},
							Name: wellknown.HTTPConnectionManager,
						},
					},
				},
			}

		})

		It("should not wrap if TcpStats is false", func() {
			in.Options.TcpStats.Value = false

			out.FilterChains = stub_filterchains

			err := plugin.ProcessListener(plugins.Params{}, in, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(out.FilterChains)).To(BeEquivalentTo(1))

			for _, chain := range out.FilterChains {
				tSock := chain.GetTransportSocket()
				Expect(tSock).To(BeNil())
			}
		})

		It("wrapping all filter chains if multiple", func() {

			hcmAny, err := utils.MessageToAny(hcm)
			Expect(err).NotTo(HaveOccurred())

			out.FilterChains = append(stub_filterchains, &envoy_config_listener_v3.FilterChain{
				Name: "another_placeholder_filter_chain",
				Filters: []*envoy_config_listener_v3.Filter{
					{
						ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
							TypedConfig: hcmAny,
						},
						Name: wellknown.HTTPConnectionManager,
					},
				},
			})

			err = plugin.ProcessListener(plugins.Params{}, in, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(out.FilterChains)).To(BeEquivalentTo(2))

			for _, chain := range out.FilterChains {
				tSock := chain.GetTransportSocket()
				Expect(tSock.Name).To(BeEquivalentTo("envoy.transport_sockets.tcp_stats"))
			}
		})

		It("wrapping existing transport_socket if filter chain has transport_socket", func() {
			cfg := &envoyauth.DownstreamTlsContext{}
			typedConfig, err := utils.MessageToAny(cfg)
			Expect(err).NotTo(HaveOccurred())
			ts := &envoy_config_core_v3.TransportSocket{
				Name:       wellknown.TransportSocketTls,
				ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
			}
			stub_filterchains[0].TransportSocket = ts
			out.FilterChains = stub_filterchains
			err = plugin.ProcessListener(plugins.Params{}, in, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(out.FilterChains)).To(BeEquivalentTo(1))

			for _, chain := range out.FilterChains {
				tSock := chain.GetTransportSocket()
				Expect(tSock.Name).To(BeEquivalentTo("envoy.transport_sockets.tcp_stats"))
			}
		})

		It("not wrapping existing transport_socket if TcpStats disabled", func() {
			in.Options.TcpStats.Value = false

			cfg := &envoyauth.DownstreamTlsContext{}
			typedConfig, err := utils.MessageToAny(cfg)
			Expect(err).NotTo(HaveOccurred())
			ts := &envoy_config_core_v3.TransportSocket{
				Name:       wellknown.TransportSocketTls,
				ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
			}
			stub_filterchains[0].TransportSocket = ts
			out.FilterChains = stub_filterchains
			err = plugin.ProcessListener(plugins.Params{}, in, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(out.FilterChains)).To(BeEquivalentTo(1))

			for _, chain := range out.FilterChains {
				tSock := chain.GetTransportSocket()
				Expect(tSock.Name).To(BeEquivalentTo("envoy.transport_sockets.tls"))
			}
		})

		It("defaulting to wrapping raw_buffer transport_socket if filter chain has no transport_socket", func() {
			hcmAny, err := utils.MessageToAny(hcm)
			Expect(err).NotTo(HaveOccurred())
			out.FilterChains = []*envoy_config_listener_v3.FilterChain{
				{
					Name: "placeholder_filter_chain",
					Filters: []*envoy_config_listener_v3.Filter{
						{
							ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
								TypedConfig: hcmAny,
							},
							Name: wellknown.HTTPConnectionManager,
						},
					},
				},
			}

			err = plugin.ProcessListener(plugins.Params{}, in, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(out.FilterChains)).To(BeEquivalentTo(1))

			for _, chain := range out.FilterChains {
				tSock := chain.GetTransportSocket()
				Expect(tSock.Name).To(BeEquivalentTo("envoy.transport_sockets.tcp_stats"))
			}
		})

	})

	It("should set perConnectionBufferLimitBytes", func() {

		in := &v1.Listener{
			Options: &v1.ListenerOptions{
				PerConnectionBufferLimitBytes: &wrappers.UInt32Value{
					Value: uint32(4096),
				},
			},
		}
		err := plugin.ProcessListener(plugins.Params{}, in, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.PerConnectionBufferLimitBytes.Value).To(BeEquivalentTo(uint32(4096)))
	})

	It("should set socket options", func() {
		in := &v1.Listener{
			Options: &v1.ListenerOptions{
				SocketOptions: []*core.SocketOption{
					{
						Description: "desc",
						Level:       1,
						Name:        2,
						Value:       &core.SocketOption_IntValue{IntValue: 123},
						State:       core.SocketOption_STATE_LISTENING,
					},
				},
			},
		}
		err := plugin.ProcessListener(plugins.Params{}, in, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.SocketOptions).To(BeEquivalentTo([]*envoy_config_core_v3.SocketOption{
			{
				Description: "desc",
				Level:       1,
				Name:        2,
				Value:       &envoy_config_core_v3.SocketOption_IntValue{IntValue: 123},
				State:       envoy_config_core_v3.SocketOption_STATE_LISTENING,
			},
		}))
	})

	Context("should set connection balance config", func() {
		It("should fail if no balancer set", func() {
			in := &v1.Listener{
				Options: &v1.ListenerOptions{
					ConnectionBalanceConfig: &v1.ConnectionBalanceConfig{},
				},
			}
			err := plugin.ProcessListener(plugins.Params{}, in, out)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("connection balancer does not specify balancer type"))
		})

		It("should set Exact balance", func() {
			in := &v1.Listener{
				Options: &v1.ListenerOptions{
					ConnectionBalanceConfig: &v1.ConnectionBalanceConfig{
						ExactBalance: &v1.ConnectionBalanceConfig_ExactBalance{},
					},
				},
			}
			err := plugin.ProcessListener(plugins.Params{}, in, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.ConnectionBalanceConfig).To(matchers.MatchProto(&envoy_config_listener_v3.Listener_ConnectionBalanceConfig{
				BalanceType: &envoy_config_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance_{
					ExactBalance: &envoy_config_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance{},
				},
			}))
		})
	})
})
