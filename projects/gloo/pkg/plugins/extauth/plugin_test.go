package extauth_test

import (
	"context"

	"github.com/golang/protobuf/ptypes/wrappers"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	"github.com/golang/protobuf/ptypes/any"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/extauth"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// We need to test three possible input values for the ext auth config (the value of the `*Plugins` attributes):
// - Undefined: no config is provided
// - Enabled: a valid custom auth config
// - Disabled: config explicitly disables auth
type ConfigState int

const (
	Undefined ConfigState = iota
	Enabled
	Disabled
)

func (c ConfigState) String() string {
	return [...]string{"Undefined", "Enabled", "Disabled"}[c]
}

// Maps an expected TypedPerFilterConfig value to a function that can be used to assert it.
var validationFuncForConfigValue = map[ConfigState]func(e envoyTypedPerFilterConfig) bool{
	Undefined: IsNotSet,
	Enabled:   IsEnabled,
	Disabled:  IsDisabled,
}

// These tests are aimed at verifying that each resource that supports extauth configurations (virtual hosts, routes, weighted destinations)
// results in the expected TypedPerFilterConfiguration on the corresponding Envoy resource (virtual hosts, routes, weighted cluster).
//
// Since the outcome on one resource is currently independent from the outcome on its parent (or children), we currently
// only test the three different input types (enabled, disabled, undefined) on each of the three resources (9 tests).
// It should be relatively easy to update these tests cover more scenarios (potentially all 3^3=27 possible
// combinations of resources and input types), should the need ever arise in the future.
var _ = Describe("Process Custom Extauth configuration", func() {

	allTests := func(globalSettings bool) {
		DescribeTable("virtual host extauth filter configuration",
			func(input, expected ConfigState) {
				pluginContext := getPluginContext(globalSettings, input, Undefined, Undefined)

				var out envoy_config_route_v3.VirtualHost
				err := pluginContext.PluginInstance.(plugins.VirtualHostPlugin).ProcessVirtualHost(pluginContext.VirtualHostParams, pluginContext.VirtualHost, &out)
				Expect(err).NotTo(HaveOccurred())
				Expect(validationFuncForConfigValue[expected](&out)).To(BeTrue())
			},
			Entry("undefined -> disable", Undefined, Disabled), // This is a special case for virtual hosts
			Entry("disabled -> disable", Disabled, Disabled),
			Entry("enabled -> enable", Enabled, Enabled),
		)

		DescribeTable("route extauth filter configuration",
			func(input, expected ConfigState) {
				pluginContext := getPluginContext(globalSettings, Undefined, input, Undefined)

				var out envoy_config_route_v3.Route
				err := pluginContext.PluginInstance.(plugins.RoutePlugin).ProcessRoute(pluginContext.RouteParams, pluginContext.Route, &out)
				Expect(err).NotTo(HaveOccurred())
				Expect(validationFuncForConfigValue[expected](&out)).To(BeTrue())
			},
			Entry("undefined -> don't set", Undefined, Undefined),
			Entry("disabled -> disable", Disabled, Disabled),
			Entry("enabled -> enable", Enabled, Enabled),
		)

		DescribeTable("weighted destination extauth filter configuration",
			func(input, expected ConfigState) {
				pluginContext := getPluginContext(globalSettings, Undefined, Undefined, input)

				var out envoy_config_route_v3.WeightedCluster_ClusterWeight
				err := pluginContext.PluginInstance.(plugins.WeightedDestinationPlugin).ProcessWeightedDestination(pluginContext.RouteParams, pluginContext.WeightedDestination, &out)
				Expect(err).NotTo(HaveOccurred())
				Expect(validationFuncForConfigValue[expected](&out)).To(BeTrue())
			},
			Entry("undefined -> don't set", Undefined, Undefined),
			Entry("disabled -> disable", Disabled, Disabled),
			Entry("enabled -> enable", Enabled, Enabled),
		)
	}

	Context("with global extauth settings", func() {
		allTests(true)
	})

	Context("with gateway-level extauth settings", func() {
		allTests(false)
	})
})

type pluginContext struct {
	PluginInstance      plugins.Plugin
	VirtualHost         *gloov1.VirtualHost
	VirtualHostParams   plugins.VirtualHostParams
	Route               *gloov1.Route
	RouteParams         plugins.RouteParams
	WeightedDestination *gloov1.WeightedDestination
}

func getPluginContext(globalSettings bool, authOnVirtualHost, authOnRoute, authOnWeightedDest ConfigState) *pluginContext {
	ctx := context.TODO()

	extAuthServerUpstream := &gloov1.Upstream{
		Metadata: &core.Metadata{
			Name:      "extauth",
			Namespace: "default",
		},
		UpstreamType: &gloov1.Upstream_Static{
			Static: &static.UpstreamSpec{
				Hosts: []*static.Host{{
					Addr: "test",
					Port: 1234,
				}},
			},
		},
	}

	// ----------------------------------------------------------------------------
	// Build auth configurations objects. Which objects are set on which resources
	// is determined by the arguments passed to this function.
	// ----------------------------------------------------------------------------
	enableCustomAuth := &extauthv1.ExtAuthExtension{
		Spec: &extauthv1.ExtAuthExtension_CustomAuth{
			CustomAuth: &v1.CustomAuth{
				ContextExtensions: map[string]string{
					"some": "context",
				},
			},
		},
	}

	disableAuth := &extauthv1.ExtAuthExtension{
		Spec: &extauthv1.ExtAuthExtension_Disable{
			Disable: true,
		},
	}

	// ----------------------------------------------------------------------------
	// Weighted destination (we just need one)
	// ----------------------------------------------------------------------------
	weightedDestination := &gloov1.WeightedDestination{
		Destination: &gloov1.Destination{
			DestinationType: &gloov1.Destination_Upstream{
				Upstream: extAuthServerUpstream.Metadata.Ref(),
			},
		},
		Weight:  &wrappers.UInt32Value{Value: 1},
		Options: &gloov1.WeightedDestinationOptions{}, // will be set below
	}

	// ----------------------------------------------------------------------------
	// Route
	// ----------------------------------------------------------------------------
	route := &gloov1.Route{
		Matchers: []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: "/",
			},
		}},
		Action: &gloov1.Route_RouteAction{
			RouteAction: &gloov1.RouteAction{
				Destination: &gloov1.RouteAction_Multi{
					Multi: &gloov1.MultiDestination{
						Destinations: []*gloov1.WeightedDestination{weightedDestination},
					},
				},
			},
		},
		Options: &gloov1.RouteOptions{}, // will be set below
	}

	// ----------------------------------------------------------------------------
	// Virtual Host
	// ----------------------------------------------------------------------------
	virtualHost := &gloov1.VirtualHost{
		Name:    "virt1",
		Domains: []string{"*"},
		Routes:  []*gloov1.Route{route},
		Options: &gloov1.VirtualHostOptions{}, // will be set below
	}

	// ----------------------------------------------------------------------------
	// Set extauth plugins based on the input arguments
	// ----------------------------------------------------------------------------

	switch authOnWeightedDest {
	case Enabled:
		weightedDestination.Options = &gloov1.WeightedDestinationOptions{Extauth: enableCustomAuth}
	case Disabled:
		weightedDestination.Options = &gloov1.WeightedDestinationOptions{Extauth: disableAuth}
	}

	switch authOnRoute {
	case Enabled:
		route.Options.Extauth = enableCustomAuth
	case Disabled:
		route.Options.Extauth = disableAuth
	}

	switch authOnVirtualHost {
	case Enabled:
		virtualHost.Options.Extauth = enableCustomAuth
	case Disabled:
		virtualHost.Options.Extauth = disableAuth
	}

	usRef := extAuthServerUpstream.Metadata.Ref()
	settings := &extauthv1.Settings{
		ExtauthzServerRef: usRef,
	}
	// ----------------------------------------------------------------------------
	// Proxy
	// ----------------------------------------------------------------------------
	proxy := &gloov1.Proxy{
		Metadata: &core.Metadata{
			Name:      "proxy",
			Namespace: "default",
		},
		Listeners: []*gloov1.Listener{{
			Name: "default",
			ListenerType: &gloov1.Listener_HttpListener{
				HttpListener: &gloov1.HttpListener{
					Options:      &gloov1.HttpListenerOptions{},
					VirtualHosts: []*gloov1.VirtualHost{virtualHost},
				},
			},
		}},
	}

	if !globalSettings {
		httpListener := proxy.Listeners[0].GetHttpListener()
		httpListener.Options.Extauth = settings
	}

	// ----------------------------------------------------------------------------
	// Define the different plugin param objects
	// that will be passed to the Process* functions
	// ----------------------------------------------------------------------------
	params := plugins.Params{
		Ctx: ctx,
		Snapshot: &v1snap.ApiSnapshot{
			Proxies:   gloov1.ProxyList{proxy},
			Upstreams: gloov1.UpstreamList{extAuthServerUpstream},
		},
	}
	virtualHostParams := plugins.VirtualHostParams{
		Params:       params,
		Proxy:        proxy,
		Listener:     proxy.Listeners[0],
		HttpListener: proxy.Listeners[0].GetHttpListener(),
	}
	routeParams := plugins.RouteParams{
		VirtualHostParams: virtualHostParams,
		VirtualHost:       virtualHost,
	}

	plugin := NewPlugin()
	initParams := plugins.InitParams{Ctx: ctx}
	initParams.Settings = &gloov1.Settings{}

	if globalSettings {
		initParams.Settings.Extauth = settings
	}

	plugin.Init(initParams)

	return &pluginContext{
		PluginInstance:      plugin,
		VirtualHost:         virtualHost,
		VirtualHostParams:   virtualHostParams,
		Route:               route,
		RouteParams:         routeParams,
		WeightedDestination: weightedDestination,
	}
}

type envoyTypedPerFilterConfig interface {
	GetTypedPerFilterConfig() map[string]*any.Any
}

// Returns true if the ext_authz filter is explicitly disabled
func IsDisabled(e envoyTypedPerFilterConfig) bool {
	if e.GetTypedPerFilterConfig() == nil {
		return false
	}
	if _, ok := e.GetTypedPerFilterConfig()[wellknown.HTTPExternalAuthorization]; !ok {
		return false
	}
	msg, err := glooutils.AnyToMessage(e.GetTypedPerFilterConfig()[wellknown.HTTPExternalAuthorization])
	cfg := msg.(*envoyauth.ExtAuthzPerRoute)
	Expect(err).NotTo(HaveOccurred())

	return cfg.GetDisabled()
}

// Returns true if the ext_authz filter is enabled and if the ContextExtensions have the expected number of entries.
func IsEnabled(e envoyTypedPerFilterConfig) bool {
	if e.GetTypedPerFilterConfig() == nil {
		return false
	}
	if _, ok := e.GetTypedPerFilterConfig()[wellknown.HTTPExternalAuthorization]; !ok {
		return false
	}
	msg, err := glooutils.AnyToMessage(e.GetTypedPerFilterConfig()[wellknown.HTTPExternalAuthorization])
	cfg := msg.(*envoyauth.ExtAuthzPerRoute)
	Expect(err).NotTo(HaveOccurred())

	if cfg.GetCheckSettings() == nil {
		return false
	}

	ctxExtensions := cfg.GetCheckSettings().ContextExtensions
	return len(ctxExtensions) == 1 && ctxExtensions["some"] == "context"
}

// Returns true if no TypedPerFilterConfig is set for the ext_authz filter
func IsNotSet(e envoyTypedPerFilterConfig) bool {
	if e.GetTypedPerFilterConfig() == nil {
		return true
	}
	_, ok := e.GetTypedPerFilterConfig()[wellknown.HTTPExternalAuthorization]
	return !ok
}
