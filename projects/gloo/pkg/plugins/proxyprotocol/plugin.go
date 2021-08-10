package proxyprotocol

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_listener_proxy_protocol "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

func NewPlugin() *plugin {
	return &plugin{}
}

// Compile-time assertion
var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.ListenerPlugin = new(plugin)
)

type plugin struct{}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoy_config_listener_v3.Listener) error {
	if !in.GetUseProxyProto().GetValue() {
		// If UseProxyProto is not defined on the listener or it is false, do not append the filter
		return nil
	}

	envoyProxyProtocol := &envoy_listener_proxy_protocol.ProxyProtocol{}
	msg, err := utils.MessageToAny(envoyProxyProtocol)
	if err != nil {
		return err
	}

	listenerFilter := &envoy_config_listener_v3.ListenerFilter{
		Name: wellknown.ProxyProtocol,
		ConfigType: &envoy_config_listener_v3.ListenerFilter_TypedConfig{
			TypedConfig: msg,
		},
	}

	out.ListenerFilters = append(out.GetListenerFilters(), listenerFilter)
	return nil
}
