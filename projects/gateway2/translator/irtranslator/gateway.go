package irtranslator

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Translator struct {
	Plugins extensionsplug.Plugin
}

type TranslationPassPlugins map[schema.GroupKind]*TranslationPass

type TranslationResult struct {
	Routes    []*envoy_config_route_v3.RouteConfiguration
	Listeners []*envoy_config_listener_v3.Listener
}

func (t *Translator) Translate(gw ir.GatewayIR, reporter reports.Reporter) TranslationResult {
	pass := t.newPass()
	var res TranslationResult

	for _, l := range gw.Listeners {
		// TODO: propagate errors so we can allow the retain last config mode
		l, routes := t.ComputeListener(context.TODO(), pass, gw, l, reporter)
		res.Listeners = append(res.Listeners, l)
		res.Routes = append(res.Routes, routes...)
	}

	return res
}

func (h *Translator) ComputeListener(ctx context.Context, pass TranslationPassPlugins, gw ir.GatewayIR, l ir.ListenerIR, reporter reports.Reporter) (*envoy_config_listener_v3.Listener, []*envoy_config_route_v3.RouteConfiguration) {
	hasTls := false
	gwreporter := reporter.Gateway(gw.SourceObject)
	var routes []*envoy_config_route_v3.RouteConfiguration
	ret := &envoy_config_listener_v3.Listener{
		Name:    l.Name,
		Address: computeListenerAddress(l.BindAddress, l.BindPort, gwreporter),
	}
	for _, hfc := range l.HttpFilterChain {
		fct := filterChainTranslator{
			listener:        l,
			routeConfigName: hfc.FilterChainName,
			PluginPass:      pass,
		}

		// compute routes
		hr := httpRouteConfigurationTranslator{
			gw:                       gw,
			listener:                 l,
			routeConfigName:          hfc.FilterChainName,
			fc:                       hfc.FilterChainCommon,
			reporter:                 reporter,
			requireTlsOnVirtualHosts: hfc.FilterChainCommon.TLS != nil,
			PluginPass:               pass,
		}
		rc := hr.ComputeRouteConfiguration(ctx, hfc.Vhosts)
		if rc != nil {
			routes = append(routes, rc)
		}

		// compute chains

		rl := gwreporter.ListenerName(hfc.FilterChainName)
		fc := fct.initFilterChain(ctx, hfc.FilterChainCommon, rl)
		fc.Filters = fct.computeHttpFilters(ctx, hfc, rl)
		ret.FilterChains = append(ret.FilterChains, fc)
		if len(hfc.Matcher.SniDomains) > 0 {
			hasTls = true
		}
	}

	fct := filterChainTranslator{
		listener:   l,
		PluginPass: pass,
	}

	for _, tfc := range l.TcpFilterChain {
		rl := gwreporter.ListenerName(tfc.FilterChainName)
		fc := fct.initFilterChain(ctx, tfc.FilterChainCommon, rl)
		fc.Filters = fct.computeTcpFilters(ctx, tfc, rl)
		ret.FilterChains = append(ret.FilterChains, fc)
		if len(tfc.Matcher.SniDomains) > 0 {
			hasTls = true
		}
	}
	if hasTls {
		ret.ListenerFilters = append(ret.GetListenerFilters(), tlsInspectorFilter())
	}
	return ret, routes
}

func (t *Translator) newPass() TranslationPassPlugins {
	ret := TranslationPassPlugins{}
	for k, v := range t.Plugins.ContributesPolicies {
		tp := v.NewGatewayTranslationPass(context.TODO(), ir.GwTranslationCtx{})
		if tp != nil {
			ret[k] = &TranslationPass{
				ProxyTranslationPass: tp,
				Name:                 v.Name,
			}
		}
	}
	return ret
}

type TranslationPass struct {
	ir.ProxyTranslationPass
	Name string
}
