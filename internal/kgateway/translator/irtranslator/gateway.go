package irtranslator

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/runtime/schema"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
)

type Translator struct {
	ContributedPolicies map[schema.GroupKind]extensionsplug.PolicyPlugin
}

type TranslationPassPlugins map[schema.GroupKind]*TranslationPass

type TranslationResult struct {
	Routes        []*envoy_config_route_v3.RouteConfiguration
	Listeners     []*envoy_config_listener_v3.Listener
	ExtraClusters []*envoy_config_cluster_v3.Cluster
}

// Translate IR to gateway. IR is self contained, so no need for krt context
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

func (t *Translator) ComputeListener(ctx context.Context, pass TranslationPassPlugins, gw ir.GatewayIR, l ir.ListenerIR, reporter reports.Reporter) (*envoy_config_listener_v3.Listener, []*envoy_config_route_v3.RouteConfiguration) {
	hasTls := false
	gwreporter := reporter.Gateway(gw.SourceObject)
	var routes []*envoy_config_route_v3.RouteConfiguration
	ret := &envoy_config_listener_v3.Listener{
		Name:    l.Name,
		Address: computeListenerAddress(l.BindAddress, l.BindPort, gwreporter),
	}
	t.runListenerPlugins(ctx, pass, gw, l, ret)

	for _, hfc := range l.HttpFilterChain {
		fct := filterChainTranslator{
			listener:        l,
			gateway:         gw,
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

		// TODO: make sure that all matchers are unique

		rl := gwreporter.ListenerName(hfc.FilterChainName)
		fc := fct.initFilterChain(ctx, hfc.FilterChainCommon, rl)
		fc.Filters = fct.computeHttpFilters(ctx, hfc, rl)
		ret.FilterChains = append(ret.GetFilterChains(), fc)
		if len(hfc.Matcher.SniDomains) > 0 {
			hasTls = true
		}
	}

	fct := filterChainTranslator{
		listener:   l,
		gateway:    gw,
		PluginPass: pass,
	}

	for _, tfc := range l.TcpFilterChain {
		rl := gwreporter.ListenerName(tfc.FilterChainName)
		fc := fct.initFilterChain(ctx, tfc.FilterChainCommon, rl)
		fc.Filters = fct.computeTcpFilters(ctx, tfc, rl)
		ret.FilterChains = append(ret.GetFilterChains(), fc)
		if len(tfc.Matcher.SniDomains) > 0 {
			hasTls = true
		}
	}
	if hasTls {
		ret.ListenerFilters = append(ret.GetListenerFilters(), tlsInspectorFilter())
	}

	return ret, routes
}

func (t *Translator) runListenerPlugins(ctx context.Context, pass TranslationPassPlugins, gw ir.GatewayIR, l ir.ListenerIR, out *envoy_config_listener_v3.Listener) {
	attachedPoliciesSlice := []ir.AttachedPolicies{
		l.AttachedPolicies,
		gw.AttachedPolicies,
	}
	for _, attachedPolicies := range attachedPoliciesSlice {
		for gk, pols := range attachedPolicies.Policies {
			pass := pass[gk]
			if pass == nil {
				// TODO: report user error - they attached a non http policy
				continue
			}
			for _, pol := range pols {
				pctx := &ir.ListenerContext{
					Policy: pol.PolicyIr,
				}
				pass.ApplyListenerPlugin(ctx, pctx, out)
				// TODO: check return value, if error returned, log error and report condition
			}
		}
	}
}

func (t *Translator) newPass() TranslationPassPlugins {
	ret := TranslationPassPlugins{}
	for k, v := range t.ContributedPolicies {
		if v.NewGatewayTranslationPass == nil {
			continue
		}
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
