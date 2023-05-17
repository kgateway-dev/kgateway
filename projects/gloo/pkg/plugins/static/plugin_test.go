package static

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	proxyproto "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/proxy_protocol/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core1 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/core"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var _ = Describe("Plugin", func() {

	var (
		p            *plugin
		initParams   plugins.InitParams
		params       plugins.Params
		upstream     *v1.Upstream
		upstreamSpec *v1static.UpstreamSpec
		out          *envoy_config_cluster_v3.Cluster
	)

	BeforeEach(func() {
		p = new(plugin)
		out = new(envoy_config_cluster_v3.Cluster)

		initParams = plugins.InitParams{}
		upstreamSpec = &v1static.UpstreamSpec{
			Hosts: []*v1static.Host{{
				Addr: "localhost",
				Port: 1234,
			}},
		}
		upstream = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "extauth-server",
				Namespace: "default",
			},
			UpstreamType: &v1.Upstream_Static{
				Static: upstreamSpec,
			},
		}

	})

	JustBeforeEach(func() {
		p.Init(initParams)
	})

	Context("h2", func() {

		It("should not http 2 by default", func() {
			p.ProcessUpstream(params, upstream, out)
			Expect(out.Http2ProtocolOptions).To(BeNil())
		})
	})

	Context("cluster type", func() {

		It("use strict dns", func() {
			p.ProcessUpstream(params, upstream, out)
			Expect(out.GetType()).To(Equal(envoy_config_cluster_v3.Cluster_STRICT_DNS))
		})

		It("use static if only has ips", func() {
			upstreamSpec.Hosts = []*v1static.Host{{
				Addr: "1.2.3.4",
				Port: 1234,
			}, {
				Addr: "2603:3005:b0b:1d00::b7aa",
				Port: 1234,
			}}
			upstreamSpec.AutoSniRewrite = &wrappers.BoolValue{Value: false}
			p.ProcessUpstream(params, upstream, out)
			Expect(out.GetType()).To(Equal(envoy_config_cluster_v3.Cluster_STATIC))
			expected := []*envoy_config_endpoint_v3.LocalityLbEndpoints{
				{
					LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
						{
							HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
								Endpoint: &envoy_config_endpoint_v3.Endpoint{
									Hostname: "1.2.3.4",
									Address: &envoy_config_core_v3.Address{
										Address: &envoy_config_core_v3.Address_SocketAddress{
											SocketAddress: &envoy_config_core_v3.SocketAddress{
												Protocol: envoy_config_core_v3.SocketAddress_TCP,
												Address:  "1.2.3.4",
												PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
													PortValue: 1234,
												},
											},
										},
									},
									HealthCheckConfig: &envoy_config_endpoint_v3.Endpoint_HealthCheckConfig{
										Hostname: "1.2.3.4",
									},
								},
							},
						},
						{
							HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
								Endpoint: &envoy_config_endpoint_v3.Endpoint{
									Hostname: "2603:3005:b0b:1d00::b7aa",
									Address: &envoy_config_core_v3.Address{
										Address: &envoy_config_core_v3.Address_SocketAddress{
											SocketAddress: &envoy_config_core_v3.SocketAddress{
												Protocol: envoy_config_core_v3.SocketAddress_TCP,
												Address:  "2603:3005:b0b:1d00::b7aa",
												PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
													PortValue: 1234,
												},
											},
										},
									},
									HealthCheckConfig: &envoy_config_endpoint_v3.Endpoint_HealthCheckConfig{
										Hostname: "2603:3005:b0b:1d00::b7aa",
									},
								},
							},
						},
					},
				},
			}
			Expect(out.GetLoadAssignment().Endpoints).To(Equal(expected))
		})

		It("use dns if has mixed addresses", func() {
			upstreamSpec.Hosts = []*v1static.Host{{
				Addr: "test.solo.io",
				Port: 1234,
			}, {
				Addr: "1.2.3.4",
				Port: 1234,
			}}

			p.ProcessUpstream(params, upstream, out)
			Expect(out.GetType()).To(Equal(envoy_config_cluster_v3.Cluster_STRICT_DNS))
		})
	})

	Context("health check config", func() {
		It("health check config gets propagated", func() {
			upstreamSpec.Hosts[0].HealthCheckConfig = &v1static.Host_HealthCheckConfig{
				Path: "/foo",
			}
			p.ProcessUpstream(params, upstream, out)
			Expect(out.LoadAssignment.Endpoints[0].LbEndpoints[0].Metadata.FilterMetadata[AdvancedHttpCheckerName].Fields[PathFieldName].GetStringValue()).To(Equal("/foo"))
			Expect(out.LoadAssignment.Endpoints[0].LbEndpoints[0].GetEndpoint().GetHealthCheckConfig().GetHostname()).To(Equal(upstreamSpec.Hosts[0].GetAddr()))
		})

		It("Should prefer top-level health-check hostnames, when available", func() {
			upstream.HealthChecks = []*core1.HealthCheck{{
				HealthChecker: &core1.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &core1.HealthCheck_HttpHealthCheck{
						Host: "test.host.path",
					},
				},
			}}
			p.ProcessUpstream(params, upstream, out)
			Expect(out.LoadAssignment.Endpoints[0].LbEndpoints[0].GetEndpoint().GetHealthCheckConfig().GetHostname()).To(Equal("test.host.path"))
		})
	})

	Context("load balancing weight config", func() {
		It("load balancing weight config gets propagated", func() {
			upstreamSpec.Hosts[0].LoadBalancingWeight = &wrappers.UInt32Value{Value: 3}
			p.ProcessUpstream(params, upstream, out)
			Expect(out.LoadAssignment.Endpoints[0].LbEndpoints[0].LoadBalancingWeight.Value).To(Equal(uint32(3)))
		})

	})
	Context("ssl", func() {
		tlsContext := func() *envoyauth.UpstreamTlsContext {
			if out.TransportSocket == nil {
				return nil
			}
			return utils.MustAnyToMessage(out.TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
		}
		It("doesn't have ssl by default", func() {
			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).To(BeNil())
		})

		It("should autodetect ssl", func() {
			upstreamSpec.Hosts[0].Port = 443
			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).ToNot(BeNil())
		})

		It("should not autoset ssl if usetls is false", func() {
			upstreamSpec.UseTls = wrapperspb.Bool(false)
			upstreamSpec.Hosts[0].Port = 443
			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).To(BeNil())
		})

		It("should allow configuring ssl", func() {
			upstreamSpec.UseTls = wrapperspb.Bool(true)
			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).ToNot(BeNil())
		})

		Context("should allow configuring ssl without settings.UpstreamOptions", func() {
			BeforeEach(func() {
				upstreamSpec.UseTls = wrapperspb.Bool(true)
				initParams.Settings = &v1.Settings{}
			})

			It("should configure CommonTlsContext without TlsParams", func() {
				err := p.ProcessUpstream(params, upstream, out)
				Expect(err).NotTo(HaveOccurred())

				commonTlsContext := tlsContext().GetCommonTlsContext()
				Expect(commonTlsContext).NotTo(BeNil())

				tlsParams := commonTlsContext.GetTlsParams()
				Expect(tlsParams).To(BeNil())
			})
		})

		Context("should allow configuring ssl with settings.UpstreamOptions", func() {
			BeforeEach(func() {
				upstreamSpec.UseTls = wrapperspb.Bool(true)
				initParams.Settings = &v1.Settings{
					UpstreamOptions: &v1.UpstreamOptions{
						SslParameters: &ssl.SslParameters{
							MinimumProtocolVersion: ssl.SslParameters_TLSv1_1,
							MaximumProtocolVersion: ssl.SslParameters_TLSv1_2,
							CipherSuites:           []string{"cipher-test"},
							EcdhCurves:             []string{"ec-dh-test"},
						},
					},
				}
			})

			It("should configure CommonTlsContext", func() {
				err := p.ProcessUpstream(params, upstream, out)
				Expect(err).NotTo(HaveOccurred())

				tlsParams := tlsContext().GetCommonTlsContext().GetTlsParams()
				Expect(tlsParams).NotTo(BeNil())
				Expect(tlsParams.GetCipherSuites()).To(Equal([]string{"cipher-test"}))
				Expect(tlsParams.GetEcdhCurves()).To(Equal([]string{"ec-dh-test"}))
				Expect(tlsParams.GetTlsMinimumProtocolVersion()).To(Equal(envoyauth.TlsParameters_TLSv1_1))
				Expect(tlsParams.GetTlsMaximumProtocolVersion()).To(Equal(envoyauth.TlsParameters_TLSv1_2))
			})
		})

		Context("should error while configuring ssl with invalid tls versions in settings.UpstreamOptions", func() {
			var invalidProtocolVersion ssl.SslParameters_ProtocolVersion = 5 // INVALID

			BeforeEach(func() {
				upstreamSpec.UseTls = wrapperspb.Bool(true)
				initParams.Settings = &v1.Settings{
					UpstreamOptions: &v1.UpstreamOptions{
						SslParameters: &ssl.SslParameters{
							MinimumProtocolVersion: invalidProtocolVersion,
							MaximumProtocolVersion: ssl.SslParameters_TLSv1_2,
							CipherSuites:           []string{"cipher-test"},
							EcdhCurves:             []string{"ec-dh-test"},
						},
					},
				}
			})

			It("should not ProcessUpstream", func() {
				err := p.ProcessUpstream(params, upstream, out)
				Expect(err).To(HaveOccurred())
			})
		})

		It("should not override existing tls config", func() {
			existing := &envoyauth.UpstreamTlsContext{}
			typedConfig, err := utils.MessageToAny(existing)
			Expect(err).ToNot(HaveOccurred())
			out.TransportSocket = &envoy_config_core_v3.TransportSocket{
				Name:       wellknown.TransportSocketTls,
				ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
			}
			upstreamSpec.UseTls = wrapperspb.Bool(true)
			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).To(Equal(existing))
		})

		It("should set proxy protocol", func() {
			upstreamSpec.UseTls = wrapperspb.Bool(true)
			upstreamSpec.Hosts[0].SniAddr = "test"
			upstream.ProxyProtocolVersion = &wrapperspb.StringValue{Value: "V1"}
			initParams.Settings = &v1.Settings{
				UpstreamOptions: &v1.UpstreamOptions{
					SslParameters: &ssl.SslParameters{
						MinimumProtocolVersion: ssl.SslParameters_TLSv1_1,
						MaximumProtocolVersion: ssl.SslParameters_TLSv1_2,
						CipherSuites:           []string{"cipher-test"},
						EcdhCurves:             []string{"ec-dh-test"},
					},
				},
			}
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TransportSocketMatches[0].Match).To(BeEquivalentTo(out.LoadAssignment.Endpoints[0].LbEndpoints[0].Metadata.FilterMetadata[TransportSocketMatchKey]))
			Expect(out.TransportSocketMatches[0].TransportSocket.Name).To(Equal("envoy.transport_sockets.upstream_proxy_protocol"))

			pMsg := utils.MustAnyToMessage(out.TransportSocketMatches[0].GetTransportSocket().GetTypedConfig()).(*proxyproto.ProxyProtocolUpstreamTransport)
			tlsMsg := utils.MustAnyToMessage(pMsg.GetTransportSocket().GetTypedConfig()).(*envoyauth.UpstreamTlsContext)

			Expect(tlsMsg.Sni).To(Equal("test"))

		})

		ExpectSniMatchesToMatch := func() {
			// make sure sni match the transport sockers
			Expect(out.TransportSocketMatches[0].Match).To(BeEquivalentTo(out.LoadAssignment.Endpoints[0].LbEndpoints[0].Metadata.FilterMetadata[TransportSocketMatchKey]))
			Expect(out.TransportSocketMatches[1].Match).To(BeEquivalentTo(out.LoadAssignment.Endpoints[0].LbEndpoints[1].Metadata.FilterMetadata[TransportSocketMatchKey]))
			// make sure 0 & 1 are different
			Expect(out.TransportSocketMatches[0].Match).NotTo(BeEquivalentTo(out.TransportSocketMatches[1].Match))
			Expect(out.TransportSocketMatches[0].Name).NotTo(Equal(out.TransportSocketMatches[1].Name))
		}

		It("should have sni per host", func() {
			upstreamSpec.UseTls = wrapperspb.Bool(true)
			upstreamSpec.AutoSniRewrite = &wrappers.BoolValue{Value: false}
			upstreamSpec.Hosts[0].SniAddr = "test"
			upstreamSpec.Hosts = append(upstreamSpec.Hosts, &v1static.Host{
				Addr:    "1.2.3.5",
				Port:    1234,
				SniAddr: "test2",
			}, &v1static.Host{
				// add a host with no sni to see that it doesn't get translated
				Addr: "1.2.3.6",
				Port: 1234,
			})

			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).ToNot(BeNil())

			Expect(out.TransportSocketMatches).To(HaveLen(2))
			match := utils.MustAnyToMessage(out.TransportSocketMatches[0].TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
			Expect(match.Sni).To(Equal("test"))
			match = utils.MustAnyToMessage(out.TransportSocketMatches[1].TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
			Expect(match.Sni).To(Equal("test2"))

			// make sure sni match the transport sockers
			ExpectSniMatchesToMatch()
		})

		It("should have sni per host by default", func() {
			upstreamSpec.UseTls = wrapperspb.Bool(true)
			upstreamSpec.Hosts[0].SniAddr = "test"
			upstreamSpec.Hosts = append(upstreamSpec.Hosts, &v1static.Host{
				Addr: "1.2.3.5",
				Port: 1234,
			})

			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).ToNot(BeNil())

			Expect(out.TransportSocketMatches).To(HaveLen(2))
			// make sure sni addr overrides
			match := utils.MustAnyToMessage(out.TransportSocketMatches[0].TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
			Expect(match.Sni).To(Equal("test"))
			// make sure that by default address is used
			match = utils.MustAnyToMessage(out.TransportSocketMatches[1].TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
			Expect(match.Sni).To(Equal("1.2.3.5"))

			// make sure sni match the transport sockets
			ExpectSniMatchesToMatch()
		})

	})
})
