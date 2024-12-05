package ir

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/model"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type filterChainTranslator struct {
	gw       model.GatewayIR
	listener model.ListenerIR

	parentRef                gwv1.ParentReference
	routeConfigName          string
	reporter                 reports.Reporter
	requireTlsOnVirtualHosts bool
	PluginPass               map[schema.GroupKind]extensions.ProxyTranslationPass
}

func (h *filterChainTranslator) ComputeFilterChains(l model.ListenerIR, reporter reports.GatewayReporter) []*envoy_config_listener_v3.FilterChain {
	reporter.Gateway()
	for _, hfc := range l.HttpFilterChain {
		h.computeHttpFilterChain(hfc)
	}
	panic("TODO")
}

func (h *filterChainTranslator) computeHttpFilterChain(l model.HttpFilterChainIR, reporter reports.Reporter) []*envoy_config_listener_v3.FilterChain {

	for _, hfc := range l.HttpFilterChain {
		h.computeHttpFilterChain(hfc)
	}
	panic("TODO")
}
