package registry

import (
	"context"
	"reflect"
	"testing"

	v2 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/filter/http/gzip/v2"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/buffer/v3"
	v32 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/filters/http/csrf/v3"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/proxylatency"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/dlp"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/waf"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/dynamic_forward_proxy"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_web"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/healthcheck"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/wasm"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func TestPlugins(t *testing.T) {
	opts := bootstrap.Opts{}
	plugins := Plugins(opts)
	pluginTypes := make(map[reflect.Type]int)
	for index, plugin := range plugins {
		pluginType := reflect.TypeOf(plugin)
		pluginTypes[pluginType] = index
	}
	if len(plugins) != len(pluginTypes) {
		t.Errorf("Multiple plugins with the same type.")
	}
}

func TestPluginsHttpFilterUsefulness(t *testing.T) {
	opts := bootstrap.Opts{}
	pluginRegistryFactory := GetPluginRegistryFactory(opts)
	pluginRegistry := pluginRegistryFactory(context.TODO())
	t.Run("Http Filters are only added if needed", func(t *testing.T) {

		ctx := context.Background()
		virtualHost := &gloov1.VirtualHost{
			Name:    "virt1",
			Domains: []string{"*"},
		}
		emptyListener := &gloov1.Listener{
			Name: "empty-listener",
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					Options:      &gloov1.HttpListenerOptions{},
					VirtualHosts: []*gloov1.VirtualHost{virtualHost},
				},
			},
		}
		fullyConfiguredListener := &gloov1.Listener{
			Name: "fully-configured-listener",
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					Options: &gloov1.HttpListenerOptions{
						GrpcWeb:                       &grpc_web.GrpcWeb{},
						HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{},
						HealthCheck:                   &healthcheck.HealthCheck{},
						Extensions:                    &gloov1.Extensions{},
						Waf:                           &waf.Settings{},
						Dlp:                           &dlp.FilterConfig{},
						Wasm:                          &wasm.PluginSource{},
						Extauth:                       &v1.Settings{},
						RatelimitServer:               &ratelimit.Settings{},
						Gzip:                          &v2.Gzip{},
						ProxyLatency:                  &proxylatency.ProxyLatency{},
						Buffer:                        &v3.Buffer{},
						Csrf:                          &v32.CsrfPolicy{},
						GrpcJsonTranscoder:            &grpc_json.GrpcJsonTranscoder{},
						SanitizeClusterHeader:         &wrapperspb.BoolValue{},
						LeftmostXffAddress:            &wrapperspb.BoolValue{},
						DynamicForwardProxy:           &dynamic_forward_proxy.FilterConfig{},
					},
					VirtualHosts: []*gloov1.VirtualHost{virtualHost},
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
				fullyConfiguredListener,
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
				p.Init(plugins.InitParams{Ctx: ctx, Settings: &gloov1.Settings{Gloo: &gloov1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: true}}}})
			}

			potentiallyNonConformingFilters := []plugins.StagedHttpFilter{}
			for _, httpPlug := range pluginRegistry.GetHttpFilterPlugins() {
				filters, err := httpPlug.HttpFilters(params, nil)
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
				p.Init(plugins.InitParams{Ctx: ctx, Settings: &gloov1.Settings{Gloo: &gloov1.GlooOptions{RemoveUnusedFilters: &wrapperspb.BoolValue{Value: false}}}})
			}

			filterCount := 0
			for _, httpPlug := range pluginRegistry.GetHttpFilterPlugins() {
				filters, err := httpPlug.HttpFilters(params, nil)
				if err != nil {
					t.Fatalf("plugin http filter failed %v", err)
				}
				filterCount += len(filters)
			}
			if len(knownBaseFilters) >= filterCount {
				t.Fatalf("reinstating to old behavior for unused filters failed with to have more filters than %d", filterCount)
			}
		})

	})

}
