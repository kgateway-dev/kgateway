package registry

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/retries"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/shadowing"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tracing"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"

	v2 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/filter/http/gzip/v2"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
	v32 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/dynamic_forward_proxy"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_web"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/healthcheck"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func TestPlugins(t *testing.T) {
	opts := bootstrap.Opts{}
	allPlugins := Plugins(opts)
	pluginTypes := make(map[reflect.Type]int)
	for index, plugin := range allPlugins {
		pluginType := reflect.TypeOf(plugin)
		pluginTypes[pluginType] = index
	}
	if len(allPlugins) != len(pluginTypes) {
		t.Errorf("Multiple plugins with the same type.")
	}
}

func TestPluginsHttpFilterUsefulness(t *testing.T) {
	opts := bootstrap.Opts{}
	pluginRegistryFactory := GetPluginRegistryFactory(opts)
	pluginRegistry := pluginRegistryFactory(context.TODO())
	t.Run("Http Filters are only added if needed", func(t *testing.T) {

		ctx := context.Background()
		emptyRoute := &gloov1.Route{
			Name:    "empty-route",
			Options: &gloov1.RouteOptions{},
		}
		emptyVirtualHost := &gloov1.VirtualHost{
			Name:    "empty-virtual-host",
			Domains: []string{"*"},
			Routes:  []*gloov1.Route{emptyRoute},
			Options: &gloov1.VirtualHostOptions{},
		}
		emptyListener := &gloov1.Listener{
			Name: "empty-listener",
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					Options:      &gloov1.HttpListenerOptions{},
					VirtualHosts: []*gloov1.VirtualHost{emptyVirtualHost},
				},
			},
		}

		configuredRoute := &gloov1.Route{
			Name:     "configured-route",
			Matchers: []*matchers.Matcher{},
			Action: &gloov1.Route_DirectResponseAction{
				DirectResponseAction: &gloov1.DirectResponseAction{
					Status: 200,
					Body:   "ok",
				},
			},
			Options: &gloov1.RouteOptions{
				Retries:    &retries.RetryPolicy{},
				Extensions: &gloov1.Extensions{},
				Tracing:    &tracing.RouteTracingSettings{},
				Shadowing: &shadowing.RouteShadowing{
					Upstream: &core.ResourceRef{
						Name:      "upstream-name",
						Namespace: "upstream-namespace",
					},
				},
				HeaderManipulation: &headers.HeaderManipulation{},
				Cors: &cors.CorsPolicy{
					AllowOrigin: []string{"origin"},
				},
				Upgrades: []*protocol_upgrade.ProtocolUpgradeConfig{},

				BufferPerRoute: &v3.BufferPerRoute{},
				Csrf:           &v32.CsrfPolicy{},
				StagedTransformations: &transformation.TransformationStages{
					Early: &transformation.RequestResponseTransformations{
						RequestTransforms:  []*transformation.RequestMatch{},
						ResponseTransforms: []*transformation.ResponseMatch{},
					},
					Regular: &transformation.RequestResponseTransformations{
						RequestTransforms:  []*transformation.RequestMatch{},
						ResponseTransforms: []*transformation.ResponseMatch{},
					},
				},
			},
		}
		configuredVirtualHost := &gloov1.VirtualHost{
			Name:    "cofigured-virtual-host",
			Domains: []string{"*"},
			Routes:  []*gloov1.Route{configuredRoute},
		}
		configuredListener := &gloov1.Listener{
			Name: "configured-listener",
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					Options: &gloov1.HttpListenerOptions{
						// We do not include options that are only supported in enterprise
						GrpcWeb:                       &grpc_web.GrpcWeb{},
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
						HealthCheck: &healthcheck.HealthCheck{
							Path: "/",
						},
						Extensions:          &gloov1.Extensions{},
						Gzip:                &v2.Gzip{},
						Buffer:              &v3.Buffer{},
						Csrf:                &v32.CsrfPolicy{},
						GrpcJsonTranscoder:  &grpc_json.GrpcJsonTranscoder{},
						DynamicForwardProxy: &dynamic_forward_proxy.FilterConfig{},
					},
					VirtualHosts: []*gloov1.VirtualHost{configuredVirtualHost},
				},
			},
		}
		proxy := &gloov1.Proxy{
			Metadata: &core.Metadata{
				Name:      "proxy",
				Namespace: "default",
			},
			Listeners: []*gloov1.Listener{
				emptyListener,
				configuredListener,
			},
		}

		params := plugins.Params{
			Ctx: ctx,
			Snapshot: &gloov1snap.ApiSnapshot{
				Proxies: gloov1.ProxyList{proxy},
			},
		}
		// Filters should not be added to this map without due consideration
		// In general we should strive not to add any new default filters going forwards
		knownBaseFilters := map[string]struct{}{}

		t.Run("Http Filters without override value", func(t *testing.T) {
			for _, p := range pluginRegistry.GetPlugins() {
				// Many plugins require safety via an init which is outside of the creation step
				p.Init(plugins.InitParams{
					Ctx: ctx,
					Settings: &gloov1.Settings{
						Gateway: &gloov1.GatewayOptions{
							Validation: &gloov1.GatewayOptions_ValidationOptions{
								DisableTransformationValidation: &wrapperspb.BoolValue{Value: true},
							},
						},
						Gloo: &gloov1.GlooOptions{
							RemoveUnusedFilters: &wrapperspb.BoolValue{Value: true},
						},
					},
				})
			}

			potentiallyNonConformingFilters := []plugins.StagedHttpFilter{}
			for _, httpPlug := range pluginRegistry.GetHttpFilterPlugins() {
				filters, err := httpPlug.HttpFilters(params, emptyListener.GetHttpListener())
				if err != nil {
					t.Fatalf("plugin http filter failed %v", err)
				}
				if len(filters) > 0 {
					potentiallyNonConformingFilters = append(potentiallyNonConformingFilters, filters...)
				}
			}

			if len(potentiallyNonConformingFilters) != len(knownBaseFilters) {
				hNames := []string{}

				for _, httpF := range potentiallyNonConformingFilters {
					if _, ok := knownBaseFilters[httpF.HttpFilter.Name]; ok {
						continue
					}
					hNames = append(hNames, httpF.HttpFilter.Name)
				}
				t.Fatalf("Found a set of filters that were added by default %v", hNames)
			}
		})
		t.Run("Http Filters with override value", func(t *testing.T) {
			for _, p := range pluginRegistry.GetPlugins() {
				// Many plugins require safety via an init which is outside of the creation step
				p.Init(plugins.InitParams{
					Ctx: ctx,
					Settings: &gloov1.Settings{
						Gateway: &gloov1.GatewayOptions{
							Validation: &gloov1.GatewayOptions_ValidationOptions{
								DisableTransformationValidation: &wrapperspb.BoolValue{Value: true},
							},
						},
						Gloo: &gloov1.GlooOptions{
							RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false},
						},
					},
				})
			}

			filterCount := 0
			for _, httpPlug := range pluginRegistry.GetHttpFilterPlugins() {
				filters, err := httpPlug.HttpFilters(params, emptyListener.GetHttpListener())
				if err != nil {
					t.Fatalf("plugin http filter failed %v", err)
				}
				filterCount += len(filters)
			}
			if len(knownBaseFilters) >= filterCount {
				t.Fatalf("reinstating to old behavior for unused filters failed with to have more filters than %d", filterCount)
			}
		})

		t.Run("Http Filters with route configuration and multiple listeners", func(t *testing.T) {
			for _, p := range pluginRegistry.GetPlugins() {
				// Many plugins require safety via an init which is outside of the creation step
				p.Init(plugins.InitParams{
					Ctx: ctx,
					Settings: &gloov1.Settings{
						Gateway: &gloov1.GatewayOptions{
							Validation: &gloov1.GatewayOptions_ValidationOptions{
								DisableTransformationValidation: &wrapperspb.BoolValue{Value: true},
							},
						},
						Gloo: &gloov1.GlooOptions{
							RemoveUnusedFilters: &wrapperspb.BoolValue{Value: true},
						},
					},
				})
			}

			// Process the configuredListener
			configuredListenerFilterCount := 0
			virtualHostParams := plugins.VirtualHostParams{
				Params:       params,
				Proxy:        proxy,
				Listener:     configuredListener,
				HttpListener: configuredListener.GetHttpListener(),
			}
			routeParams := plugins.RouteParams{
				VirtualHostParams: virtualHostParams,
				VirtualHost:       configuredVirtualHost,
			}
			for _, routePlugin := range pluginRegistry.GetRoutePlugins() {
				err := routePlugin.ProcessRoute(routeParams, configuredRoute, &envoy_config_route_v3.Route{})
				if err != nil {
					t.Fatalf("plugin route filter failed %v", err)
				}
			}
			for _, httpPlug := range pluginRegistry.GetHttpFilterPlugins() {
				filters, err := httpPlug.HttpFilters(params, configuredListener.GetHttpListener())
				if err != nil {
					t.Fatalf("plugin http filter failed %v", err)
				}
				configuredListenerFilterCount += len(filters)
			}

			// Process the emptyListener
			emptyListenerFilterCount := 0
			virtualHostParams = plugins.VirtualHostParams{
				Params:       params,
				Proxy:        proxy,
				Listener:     emptyListener,
				HttpListener: emptyListener.GetHttpListener(),
			}
			routeParams = plugins.RouteParams{
				VirtualHostParams: virtualHostParams,
				VirtualHost:       emptyVirtualHost,
			}
			for _, routePlugin := range pluginRegistry.GetRoutePlugins() {
				err := routePlugin.ProcessRoute(routeParams, emptyRoute, &envoy_config_route_v3.Route{})
				if err != nil {
					Fail(fmt.Sprintf("plugin route filter failed %v", err))
				}
			}
			for _, httpPlug := range pluginRegistry.GetHttpFilterPlugins() {
				filters, err := httpPlug.HttpFilters(params, emptyListener.GetHttpListener())
				if err != nil {
					Fail(fmt.Sprintf("plugin http filter failed %v", err))
				}
				emptyListenerFilterCount += len(filters)
			}

			// Validate that the emptyListener filter count and configuredListener filter count are different
			if emptyListenerFilterCount != len(knownBaseFilters) {
				Fail(fmt.Sprintf("Found %d filters that were configured, but expected %d", emptyListenerFilterCount, len(knownBaseFilters)))
			}

			if configuredListenerFilterCount <= len(knownBaseFilters) {
				Fail(fmt.Sprintf("Found %d filters that were configured, but expected at least %d", configuredListenerFilterCount, len(knownBaseFilters)))
			}

		})

	})

}
