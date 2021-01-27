package listener

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.ListenerPlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(_ plugins.InitParams) error {
	return nil
}

// Used to set config that are directly on the [Envoy listener](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/listener.proto)
func (p *Plugin) ProcessListener(_ plugins.Params, in *v1.Listener, out *envoy_config_listener_v3.Listener) error {
	if in.GetOptions().GetPerConnectionBufferLimitBytes().GetValue() != 0 {
		out.PerConnectionBufferLimitBytes = in.GetOptions().GetPerConnectionBufferLimitBytes()
	}

	if in.GetOptions().GetSocketOptions() != nil {
		out.SocketOptions = translateSocketOptions(in.GetOptions().GetSocketOptions())
	}

	return nil
}

func translateSocketOptions(sos []*core.SocketOption) []*envoy_config_core_v3.SocketOption {
	var socketOptions []*envoy_config_core_v3.SocketOption
	for _, so := range sos {
		socketOptions = append(socketOptions, translateSocketOption(so))
	}
	return socketOptions
}

func translateSocketOption(so *core.SocketOption) *envoy_config_core_v3.SocketOption {

	ret := &envoy_config_core_v3.SocketOption{
		Description: so.GetDescription(),
		Level:       so.GetLevel(),
		Name:        so.GetName(),
		State:       translateSocketState(so.GetState()),
	}

	switch typed := so.GetValue().(type) {
	case *core.SocketOption_BufValue:
		ret.Value = &envoy_config_core_v3.SocketOption_BufValue{BufValue: typed.BufValue}
	case *core.SocketOption_IntValue:
		ret.Value = &envoy_config_core_v3.SocketOption_IntValue{IntValue: typed.IntValue}
	}

	return ret
}

func translateSocketState(state core.SocketOption_SocketState) envoy_config_core_v3.SocketOption_SocketState {
	if state.Enum() == nil {
		return envoy_config_core_v3.SocketOption_STATE_PREBIND
	}

	switch *state.Enum() {
	case core.SocketOption_STATE_PREBIND:
		return envoy_config_core_v3.SocketOption_STATE_PREBIND
	case core.SocketOption_STATE_BOUND:
		return envoy_config_core_v3.SocketOption_STATE_BOUND
	case core.SocketOption_STATE_LISTENING:
		return envoy_config_core_v3.SocketOption_STATE_LISTENING
	}
	return envoy_config_core_v3.SocketOption_STATE_PREBIND
}
