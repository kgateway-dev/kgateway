package translator_test

import (
	"context"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/protobuf/types/known/wrapperspb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	corsplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/cors"
	hcmplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tcp"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	mock_utils "github.com/solo-io/gloo/projects/gloo/pkg/utils/mocks"
	gloovalidation "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// Allow each test to define its own set of assertions
// based on the envoy types returned by executing the ListenerTranslator and RouteConfigurationTranslator
type ResourceAssertionHandler func(
	listener *envoy_config_listener_v3.Listener,
	routeConfigurations []*envoy_config_route_v3.RouteConfiguration)

type ReportAssertionHandler func(
	proxyReport *validation.ProxyReport)

var _ = Describe("Listener Subsystem", func() {
	// These tests validate that the ListenerSubsystemTranslatorFactory produces Translators
	// which in turn create Envoy Listeners and RouteConfigurations with expected values
	// The tests are non-exhaustive, as we expect each translator to more rigorously test the
	// edge cases. Instead, these tests focus on the high level Envoy resources that are created.

	var (
		ctx    context.Context
		cancel context.CancelFunc

		translatorFactory *translator.ListenerSubsystemTranslatorFactory

		ctrl          *gomock.Controller
		sslTranslator *mock_utils.MockSslConfigTranslator

		settings *v1.Settings
	)

	BeforeEach(func() {
		// To cover TCP cases we must include the plugin
		ctrl = gomock.NewController(GinkgoT())
		sslTranslator = mock_utils.NewMockSslConfigTranslator(ctrl)

		ctx, cancel = context.WithCancel(context.Background())

		settings = &v1.Settings{
			Gateway: &v1.GatewayOptions{
				Validation: &v1.GatewayOptions_ValidationOptions{
					// set this as it is the default setting initialized by helm
					WarnMissingTlsSecret: &wrapperspb.BoolValue{Value: true},
				},
			},
		}

		// Create a pluginRegistry with a minimal number of plugins
		// This test is not concerned with the functionality of individual plugins
		pluginRegistry := registry.NewPluginRegistry([]plugins.Plugin{
			hcmplugin.NewPlugin(),
			corsplugin.NewPlugin(),
			tcp.NewPlugin(sslTranslator),
		})

		// The translatorFactory expects each of the plugins to be initialized
		// Therefore, to test this component we pre-initialize the plugins
		for _, p := range pluginRegistry.GetPlugins() {
			p.Init(plugins.InitParams{
				Ctx:      ctx,
				Settings: settings,
			})
		}

		translatorFactory = translator.NewListenerSubsystemTranslatorFactory(pluginRegistry, sslutils.NewSslConfigTranslator(), settings)

	})

	AfterEach(func() {
		ctrl.Finish()
		cancel()
	})

	DescribeTable("GetAggregateListenerTranslators (success)",
		func(aggregateListener *v1.AggregateListener, assertionHandler ResourceAssertionHandler) {
			listener := &v1.Listener{
				Name:        "aggregate-listener",
				BindAddress: gatewaydefaults.GatewayBindAddress,
				BindPort:    defaults.HttpPort,
				ListenerType: &v1.Listener_AggregateListener{
					AggregateListener: aggregateListener,
				},
			}
			proxy := &v1.Proxy{
				Metadata: &core.Metadata{
					Name:      "proxy",
					Namespace: defaults.GlooSystem,
				},
				Listeners: []*v1.Listener{listener},
			}

			proxyReport := gloovalidation.MakeReport(proxy)
			listenerReport := proxyReport.GetListenerReports()[0] // 1 Listener -> 1 ListenerReport

			listenerTranslator, routeConfigurationTranslator := translatorFactory.GetAggregateListenerTranslators(
				ctx,
				proxy,
				listener,
				listenerReport)

			params := plugins.Params{
				Ctx: ctx,
				Snapshot: &gloov1snap.ApiSnapshot{
					// To support ssl filter chain
					Secrets: v1.SecretList{createTLSSecret()},
					Upstreams: v1.UpstreamList{{
						Metadata: &core.Metadata{
							Name:      "test",
							Namespace: "gloo-system",
						},
					}},
				},
			}
			envoyListener := listenerTranslator.ComputeListener(params)
			envoyRouteConfigs := routeConfigurationTranslator.ComputeRouteConfiguration(params)

			// Validate that no Errors were encountered during translation
			Expect(gloovalidation.GetProxyError(proxyReport)).NotTo(HaveOccurred())

			// Validate the ResourceAssertionHandler defined by each test
			assertionHandler(envoyListener, envoyRouteConfigs)
		},
		Entry(
			"0 filter chains",
			&v1.AggregateListener{
				HttpResources:    &v1.AggregateListener_HttpResources{},
				HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{},
			},
			func(listener *envoy_config_listener_v3.Listener, routeConfigs []*envoy_config_route_v3.RouteConfiguration) {
				ExpectWithOffset(1, listener.GetFilterChains()).To(BeEmpty())
				ExpectWithOffset(1, routeConfigs).To(BeEmpty())
			},
		),
		Entry(
			"1 insecure filter chain",
			&v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
						},
					},
					VirtualHosts: map[string]*v1.VirtualHost{
						"vhost-ref": {
							Name: "virtual-host",
						},
					},
				},
				HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{{
					Matcher:         nil,
					HttpOptionsRef:  "http-options-ref",
					VirtualHostRefs: []string{"vhost-ref"},
				}},
			},
			func(listener *envoy_config_listener_v3.Listener, routeConfigs []*envoy_config_route_v3.RouteConfiguration) {
				By("1 insecure filter chain")
				ExpectWithOffset(1, listener.GetFilterChains()).To(HaveLen(1))
				filterChain := listener.GetFilterChains()[0]
				ExpectWithOffset(1, filterChain.GetFilterChainMatch()).To(BeNil())

				By("with hcm network filter")
				hcmFilter := filterChain.GetFilters()[0]
				typedConfig, err := sslutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				hcm := typedConfig.(*envoy_http_connection_manager_v3.HttpConnectionManager)
				hcmRouteConfigName := hcm.GetRds().GetRouteConfigName()

				By("1 route configuration, with name matching HCM")
				ExpectWithOffset(1, routeConfigs).To(HaveLen(1))
				routeConfig := routeConfigs[0]
				ExpectWithOffset(1, routeConfig.GetName()).To(Equal(hcmRouteConfigName))
			},
		),
		Entry(
			"1 secure filter chain",
			&v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
						},
					},
					VirtualHosts: map[string]*v1.VirtualHost{
						"vhost-ref": {
							Name: "virtual-host",
						},
					},
				},
				HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{{
					Matcher: &v1.Matcher{
						SslConfig: &ssl.SslConfig{
							SniDomains:    []string{"sni-domain"},
							AlpnProtocols: []string{"h2"},
							SslSecrets: &ssl.SslConfig_SecretRef{
								SecretRef: createTLSSecret().GetMetadata().Ref(),
							},
						},
					},
					HttpOptionsRef:  "http-options-ref",
					VirtualHostRefs: []string{"vhost-ref"},
				}},
			},
			func(listener *envoy_config_listener_v3.Listener, routeConfigs []*envoy_config_route_v3.RouteConfiguration) {
				By("1 secure filter chain")
				ExpectWithOffset(1, listener.GetFilterChains()).To(HaveLen(1))
				filterChain := listener.GetFilterChains()[0]
				ExpectWithOffset(1, filterChain.GetFilterChainMatch()).To(Equal(&envoy_config_listener_v3.FilterChainMatch{
					ServerNames: []string{"sni-domain"},
				}))

				By("with hcm network filter")
				hcmFilter := filterChain.GetFilters()[0]
				typedConfig, err := sslutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				hcm := typedConfig.(*envoy_http_connection_manager_v3.HttpConnectionManager)
				hcmRouteConfigName := hcm.GetRds().GetRouteConfigName()

				By("1 route configuration, with name matching HCM")
				ExpectWithOffset(1, routeConfigs).To(HaveLen(1))
				routeConfig := routeConfigs[0]
				ExpectWithOffset(1, routeConfig.GetName()).To(Equal(hcmRouteConfigName))
			},
		),
		Entry(
			"multiple secure filter chains",
			&v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
						},
					},
					VirtualHosts: map[string]*v1.VirtualHost{
						"vhost-ref": {
							Name: "virtual-host",
						},
					},
				},
				HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{
					{
						Matcher: &v1.Matcher{
							SslConfig: &ssl.SslConfig{
								SniDomains:    []string{"sni-domain"},
								AlpnProtocols: []string{"h2"},
								SslSecrets: &ssl.SslConfig_SecretRef{
									SecretRef: createTLSSecret().GetMetadata().Ref(),
								},
							},
						},
						HttpOptionsRef:  "http-options-ref",
						VirtualHostRefs: []string{"vhost-ref"},
					},
					{
						Matcher: &v1.Matcher{
							SslConfig: &ssl.SslConfig{
								SniDomains:    []string{"other-sni-domain"},
								AlpnProtocols: []string{"h2"},
								SslSecrets: &ssl.SslConfig_SecretRef{
									SecretRef: createTLSSecret().GetMetadata().Ref(),
								},
							},
						},
						HttpOptionsRef:  "http-options-ref",
						VirtualHostRefs: []string{"vhost-ref"},
					},
				},
			},
			func(listener *envoy_config_listener_v3.Listener, routeConfigs []*envoy_config_route_v3.RouteConfiguration) {
				By("2 secure filter chains and route configurations")
				ExpectWithOffset(1, listener.GetFilterChains()).To(HaveLen(2))
				ExpectWithOffset(1, routeConfigs).To(HaveLen(2))

				By("first filter chain")
				filterChain := listener.GetFilterChains()[0]
				ExpectWithOffset(1, filterChain.GetFilterChainMatch()).To(Equal(&envoy_config_listener_v3.FilterChainMatch{
					ServerNames: []string{"sni-domain"},
				}))

				By("with hcm network filter")
				hcmFilter := filterChain.GetFilters()[0]
				typedConfig, err := sslutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				hcm := typedConfig.(*envoy_http_connection_manager_v3.HttpConnectionManager)
				hcmRouteConfigName := hcm.GetRds().GetRouteConfigName()

				By("route config name matches HCM")
				routeConfig := routeConfigs[0]
				ExpectWithOffset(1, routeConfig.GetName()).To(Equal(hcmRouteConfigName))

				By("second filter chain")
				filterChain = listener.GetFilterChains()[1]
				ExpectWithOffset(1, filterChain.GetFilterChainMatch()).To(Equal(&envoy_config_listener_v3.FilterChainMatch{
					ServerNames: []string{"other-sni-domain"},
				}))

				By("with hcm network filter")
				hcmFilter = filterChain.GetFilters()[0]
				typedConfig, err = sslutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				hcm = typedConfig.(*envoy_http_connection_manager_v3.HttpConnectionManager)
				hcmRouteConfigName = hcm.GetRds().GetRouteConfigName()

				By("route config name matches HCM")
				routeConfig = routeConfigs[1]
				ExpectWithOffset(1, routeConfig.GetName()).To(Equal(hcmRouteConfigName))
			},
		),
		Entry(
			"http filter chain matchers",
			&v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
						},
					},
					VirtualHosts: map[string]*v1.VirtualHost{
						"vhost-ref": {
							Name: "virtual-host",
						},
					},
				},
				HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{{
					Matcher: &v1.Matcher{
						SourcePrefixRanges: []*v3.CidrRange{{
							AddressPrefix: "1.2.3.4",
							PrefixLen:     &wrappers.UInt32Value{Value: 32},
						}},
						PrefixRanges: []*v3.CidrRange{{
							AddressPrefix: "5.6.7.8",
							PrefixLen:     &wrappers.UInt32Value{Value: 32},
						}},
						DestinationPort: &wrappers.UInt32Value{Value: 1234},
					},
					HttpOptionsRef:  "http-options-ref",
					VirtualHostRefs: []string{"vhost-ref"},
				}},
			},
			func(listener *envoy_config_listener_v3.Listener, routeConfigs []*envoy_config_route_v3.RouteConfiguration) {
				By("http filter chain matchers")
				ExpectWithOffset(1, listener.GetFilterChains()).To(HaveLen(1))
				filterChain := listener.GetFilterChains()[0]
				ExpectWithOffset(1, filterChain.GetFilterChainMatch()).To(Equal(&envoy_config_listener_v3.FilterChainMatch{
					SourcePrefixRanges: []*corev3.CidrRange{{
						AddressPrefix: "1.2.3.4",
						PrefixLen:     &wrappers.UInt32Value{Value: 32},
					}},
					PrefixRanges: []*corev3.CidrRange{{
						AddressPrefix: "5.6.7.8",
						PrefixLen:     &wrappers.UInt32Value{Value: 32},
					}},
					DestinationPort: &wrappers.UInt32Value{Value: 1234},
				}))
			},
		),
		Entry(
			"tcp filter chain matchers",
			&v1.AggregateListener{
				TcpListeners: []*v1.MatchedTcpListener{{
					Matcher: &v1.Matcher{
						SourcePrefixRanges: []*v3.CidrRange{{
							AddressPrefix: "1.2.3.4",
							PrefixLen:     &wrappers.UInt32Value{Value: 32},
						}},
						PrefixRanges: []*v3.CidrRange{{
							AddressPrefix: "5.6.7.8",
							PrefixLen:     &wrappers.UInt32Value{Value: 32},
						}},
						DestinationPort: &wrappers.UInt32Value{Value: 1234},
					},
					TcpListener: &v1.TcpListener{
						TcpHosts: []*v1.TcpHost{{
							Name: "foobar",
							Destination: &v1.TcpHost_TcpAction{
								Destination: &v1.TcpHost_TcpAction_Single{
									Single: &v1.Destination{
										DestinationType: &v1.Destination_Upstream{
											Upstream: &core.ResourceRef{
												Name:      "test",
												Namespace: "gloo-system",
											},
										},
									},
								},
							},
						}},
					},
				}},
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions:  map[string]*v1.HttpListenerOptions{},
					VirtualHosts: map[string]*v1.VirtualHost{},
				},
			},
			func(listener *envoy_config_listener_v3.Listener, routeConfigs []*envoy_config_route_v3.RouteConfiguration) {
				By("tcp filter chain matchers")
				ExpectWithOffset(1, listener.GetFilterChains()).To(HaveLen(1))
				filterChain := listener.GetFilterChains()[0]
				ExpectWithOffset(1, filterChain.GetFilterChainMatch()).To(Equal(&envoy_config_listener_v3.FilterChainMatch{
					SourcePrefixRanges: []*corev3.CidrRange{{
						AddressPrefix: "1.2.3.4",
						PrefixLen:     &wrappers.UInt32Value{Value: 32},
					}},
					PrefixRanges: []*corev3.CidrRange{{
						AddressPrefix: "5.6.7.8",
						PrefixLen:     &wrappers.UInt32Value{Value: 32},
					}},
					DestinationPort: &wrappers.UInt32Value{Value: 1234},
				}))
			},
		),
	)

	DescribeTable("GetAggregateListenerTranslators (failure)",
		func(aggregateListener *v1.AggregateListener, assertionHandler ReportAssertionHandler) {
			listener := &v1.Listener{
				Name:        "aggregate-listener",
				BindAddress: gatewaydefaults.GatewayBindAddress,
				BindPort:    defaults.HttpPort,
				ListenerType: &v1.Listener_AggregateListener{
					AggregateListener: aggregateListener,
				},
			}
			proxy := &v1.Proxy{
				Metadata: &core.Metadata{
					Name:      "proxy",
					Namespace: defaults.GlooSystem,
				},
				Listeners: []*v1.Listener{listener},
			}

			proxyReport := gloovalidation.MakeReport(proxy)
			listenerReport := proxyReport.GetListenerReports()[0] // 1 Listener -> 1 ListenerReport

			listenerTranslator, routeConfigurationTranslator := translatorFactory.GetAggregateListenerTranslators(
				ctx,
				proxy,
				listener,
				listenerReport)

			params := plugins.Params{
				Ctx: ctx,
				Snapshot: &gloov1snap.ApiSnapshot{
					Secrets: []*v1.Secret{{
						Kind: &v1.Secret_Tls{
							// This is an invalid secret that will generate a listener error when referenced.
							Tls: &v1.TlsSecret{},
						},
						Metadata: &core.Metadata{
							Name:      "exists-but-invalid",
							Namespace: defaults.GlooSystem,
						},
					}},
				},
			}
			_ = listenerTranslator.ComputeListener(params)
			_ = routeConfigurationTranslator.ComputeRouteConfiguration(params)

			// Validate the ReportAssertionHandler defined by each test
			assertionHandler(proxyReport)
		},
		Entry(
			"ListenerError",
			&v1.AggregateListener{

				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
						},
					},
					VirtualHosts: map[string]*v1.VirtualHost{
						"vhost-ref": {
							Name: "virtual-host",
						},
					},
				},
				HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{{
					Matcher: &v1.Matcher{
						SslConfig: &ssl.SslConfig{
							SslSecrets: &ssl.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      "exists-but-invalid",
									Namespace: defaults.GlooSystem,
								},
							},
						},
					},
					HttpOptionsRef:  "http-options-ref",
					VirtualHostRefs: []string{"vhost-ref"},
				}},
			},
			func(proxyReport *validation.ProxyReport) {
				proxyErr := gloovalidation.GetProxyError(proxyReport)
				Expect(proxyErr).To(HaveOccurred())
				Expect(proxyErr.Error()).To(ContainSubstring(validation.ListenerReport_Error_SSLConfigError.String()))
			},
		),
		Entry(
			"ListenerWarning",
			&v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
						},
					},
					VirtualHosts: map[string]*v1.VirtualHost{
						"vhost-ref": {
							Name: "virtual-host",
						},
					},
				},
				HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{{
					Matcher: &v1.Matcher{
						SslConfig: &ssl.SslConfig{
							SslSecrets: &ssl.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      "secret-that-is-not-in-snapshot",
									Namespace: defaults.GlooSystem,
								},
							},
						},
					},
					HttpOptionsRef:  "http-options-ref",
					VirtualHostRefs: []string{"vhost-ref"},
				}},
			},
			func(proxyReport *validation.ProxyReport) {
				proxyErr := gloovalidation.GetProxyWarning(proxyReport)
				Expect(proxyErr).To(ContainElement(ContainSubstring(validation.ListenerReport_Warning_SSLConfigWarning.String())))
			},
		),
		Entry(
			"HttpListenerError",
			&v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
								// Multiple Upgrades that overlap should produce an error when processing the HCM plugin
								Upgrades: []*protocol_upgrade.ProtocolUpgradeConfig{
									{
										UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
											Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
												Enabled: &wrappers.BoolValue{
													Value: true,
												},
											},
										},
									},
									{
										UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
											Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
												Enabled: &wrappers.BoolValue{
													Value: true,
												},
											},
										},
									},
								},
							},
						},
					},
					VirtualHosts: map[string]*v1.VirtualHost{
						"vhost-ref": {
							Name: "virtual-host",
						},
					},
				},
				HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{{
					Matcher:         nil,
					HttpOptionsRef:  "http-options-ref",
					VirtualHostRefs: []string{"vhost-ref"},
				}},
			},
			func(proxyReport *validation.ProxyReport) {
				proxyErr := gloovalidation.GetProxyError(proxyReport)
				Expect(proxyErr).To(HaveOccurred())
				Expect(proxyErr.Error()).To(ContainSubstring(validation.HttpListenerReport_Error_ProcessingError.String()))
				Expect(proxyErr.Error()).To(ContainSubstring("upgrade config websocket is not unique"))
			},
		),
		Entry(
			"VirtualHostError",
			&v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
						},
					},
					VirtualHosts: map[string]*v1.VirtualHost{
						"vhost-ref": {
							Name: "virtual-host",
							Options: &v1.VirtualHostOptions{
								Cors: &cors.CorsPolicy{
									// Empty AllowOrigin and AllowOriginRegex should produce an error when processing the CORS plugin
									AllowOrigin:      []string{},
									AllowOriginRegex: []string{},
								},
							},
						},
					},
				},
				HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{{
					Matcher:         nil,
					HttpOptionsRef:  "http-options-ref",
					VirtualHostRefs: []string{"vhost-ref"},
				}},
			},
			func(proxyReport *validation.ProxyReport) {
				proxyErr := gloovalidation.GetProxyError(proxyReport)
				Expect(proxyErr).To(HaveOccurred())
				Expect(proxyErr.Error()).To(ContainSubstring(validation.VirtualHostReport_Error_ProcessingError.String()))
				Expect(proxyErr.Error()).To(ContainSubstring("must provide at least one of AllowOrigin or AllowOriginRegex"))
			},
		),
		Entry(
			"RouteError",
			&v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
						},
					},
					VirtualHosts: map[string]*v1.VirtualHost{
						"vhost-ref": {
							Name: "virtual-host",
							Routes: []*v1.Route{{
								Name: "route",
								Matchers: []*matchers.Matcher{{
									// A nil PathSpecifier should produce an error when initializing the route
									PathSpecifier: nil,
								}},
							}},
						},
					},
				},
				HttpFilterChains: []*v1.AggregateListener_HttpFilterChain{{
					Matcher:         nil,
					HttpOptionsRef:  "http-options-ref",
					VirtualHostRefs: []string{"vhost-ref"},
				}},
			},
			func(proxyReport *validation.ProxyReport) {
				proxyErr := gloovalidation.GetProxyError(proxyReport)
				Expect(proxyErr).To(HaveOccurred())
				Expect(proxyErr.Error()).To(ContainSubstring(validation.RouteReport_Error_InvalidMatcherError.String()))
				Expect(proxyErr.Error()).To(ContainSubstring("no path specifier provided"))
			},
		),
	)

	Describe("hybrid listener chains", func() {
		It("doesnt crash with unknown types", func() {
			listener := &v1.Listener{
				Name:        "aggregate-listener",
				BindAddress: gatewaydefaults.GatewayBindAddress,
				BindPort:    defaults.HttpPort,
				ListenerType: &v1.Listener_HybridListener{
					HybridListener: &v1.HybridListener{
						MatchedListeners: []*v1.MatchedListener{
							{},
						},
					},
				},
			}
			proxy := &v1.Proxy{
				Metadata: &core.Metadata{
					Name:      "proxy",
					Namespace: defaults.GlooSystem,
				},
				Listeners: []*v1.Listener{listener},
			}

			proxyReport := gloovalidation.MakeReport(proxy)
			listenerReport := proxyReport.GetListenerReports()[0] // 1 Listener -> 1 ListenerReport

			listenerTranslator, routeConfigurationTranslator := translatorFactory.GetHybridListenerTranslators(
				ctx,
				proxy,
				listener,
				listenerReport)
			params := plugins.Params{
				Ctx: ctx,
				Snapshot: &gloov1snap.ApiSnapshot{
					// To support ssl filter chain
					Secrets: v1.SecretList{createTLSSecret()},
				},
			}
			li := listenerTranslator.ComputeListener(params)
			_ = routeConfigurationTranslator.ComputeRouteConfiguration(params)

			Expect(li.GetFilterChains()).To(BeEmpty())
		})
	})
})

func createTLSSecret() *v1.Secret {
	return &v1.Secret{
		Metadata: &core.Metadata{
			Name:      "tls",
			Namespace: defaults.GlooSystem,
		},
		Kind: &v1.Secret_Tls{
			Tls: &v1.TlsSecret{
				CertChain:  gloohelpers.Certificate(),
				PrivateKey: gloohelpers.PrivateKey(),
			},
		},
	}
}
