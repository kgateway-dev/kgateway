package ir

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	extensions "github.com/solo-io/gloo/projects/gateway2/extensions2"
	"github.com/solo-io/gloo/projects/gateway2/model"
	"github.com/solo-io/gloo/projects/gateway2/reports"
)

type Translator struct {
	Plugins extensions.Plugin
}

type TranslationResult struct {
	Routes    []*envoy_config_route_v3.RouteConfiguration
	Listeners []*envoy_config_listener_v3.Listener
}

func (t *Translator) Translate(gw model.GatewayIR, reporter reports.Reporter) TranslationResult {

	panic("TODO")
}
