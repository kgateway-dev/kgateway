package translator_test

import (
	"context"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	envoy_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	routerV1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/router"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	hcmplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	gloovalidation "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Router filter test", func() {
	// These tests validate the router filter that's generated from the network_filters translator. It
	// would be ideal if that filter could be broken out into its own separate plugin, but for now
	// it's a bit shoehorned into the HTTP connection manager translator

	var (
		ctx    context.Context
		cancel context.CancelFunc

		translatorFactory *translator.ListenerSubsystemTranslatorFactory
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Create a pluginRegistry with a minimal number of plugins
		// This test is not concerned with the functionality of individual plugins
		pluginRegistry := registry.NewPluginRegistry([]plugins.Plugin{
			hcmplugin.NewPlugin(),
		})

		// The translatorFactory expects each of the plugins to be initialized
		// Therefore, to test this component we pre-initialize the plugins
		for _, p := range pluginRegistry.GetPlugins() {
			p.Init(plugins.InitParams{
				Ctx:      ctx,
				Settings: &v1.Settings{},
			})
		}

		translatorFactory = translator.NewListenerSubsystemTranslatorFactory(pluginRegistry, sslutils.NewSslConfigTranslator())
	})

	AfterEach(func() {
		cancel()
	})

	// FIXME remove focus
	FDescribeTable("GetAggregateListenerTranslators (success)",
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
			"Set dynamic_stats to false and suppress_envoy_headers to true",
			&v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
							Router: &routerV1.Router{
								DynamicStats: &wrappers.BoolValue{
									Value: false,
								},
								SuppressEnvoyHeaders: &wrappers.BoolValue{
									Value: true,
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
			func(listener *envoy_config_listener_v3.Listener, routeConfigs []*envoy_config_route_v3.RouteConfiguration) {
				By("configuring the envoy router from gloo settings")
				filterChain := listener.GetFilterChains()[0]
				hcmFilter := filterChain.GetFilters()[0]
				_, err := sslutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
				Expect(err).NotTo(HaveOccurred())

				hcm := &envoy_http_connection_manager_v3.HttpConnectionManager{}
				err = translator.ParseTypedConfig(hcmFilter, hcm)
				Expect(err).NotTo(HaveOccurred())
				Expect(hcm.HttpFilters).To(HaveLen(1))

				routeFilter := hcm.GetHttpFilters()[0]
				typedRouterFilter := routerv3.Router{}
				routeFilter.GetTypedConfig().UnmarshalTo(&typedRouterFilter)
				Expect(typedRouterFilter.GetDynamicStats().GetValue()).To(BeFalse())
				Expect(typedRouterFilter.GetSuppressEnvoyHeaders()).To(BeTrue())
			},
		),

		Entry(
			"Leave envoy's dynamic_stats as nil if not specified in gloo",
			&v1.AggregateListener{
				HttpResources: &v1.AggregateListener_HttpResources{
					HttpOptions: map[string]*v1.HttpListenerOptions{
						"http-options-ref": {
							HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
							Router:                        &routerV1.Router{},
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
				By("configuring the envoy router from gloo settings")
				filterChain := listener.GetFilterChains()[0]
				hcmFilter := filterChain.GetFilters()[0]
				_, err := sslutils.AnyToMessage(hcmFilter.GetConfigType().(*envoy_config_listener_v3.Filter_TypedConfig).TypedConfig)
				Expect(err).NotTo(HaveOccurred())

				hcm := &envoy_http_connection_manager_v3.HttpConnectionManager{}
				err = translator.ParseTypedConfig(hcmFilter, hcm)
				Expect(err).NotTo(HaveOccurred())
				Expect(hcm.HttpFilters).To(HaveLen(1))

				typedRouterFiler := routerv3.Router{}
				routeFilter := hcm.GetHttpFilters()[0]
				routeFilter.GetTypedConfig().UnmarshalTo(&typedRouterFiler)
				Expect(typedRouterFiler.GetDynamicStats()).To(BeNil())
			},
		),
	)
})
