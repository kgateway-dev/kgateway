package listener

import (
	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
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
func (p *Plugin) ProcessListener(_ plugins.Params, in *v1.Listener, out *envoy_api_v2.Listener) error {
	if in.GetOptions().GetPerConnectionBufferLimitBytes().GetValue() != 0 {
		out.PerConnectionBufferLimitBytes = gogoutils.UInt32GogoToProto(in.GetOptions().GetPerConnectionBufferLimitBytes())
	}

	if in.GetOptions().GetSocketOptions() != nil {
		out.SocketOptions = translateSocketOptions(in.GetOptions().GetSocketOptions())
	}

	return nil
}

func translateSocketOptions(sos []*core.SocketOption) []*envoy_api_v2_core.SocketOption {
	var socketOptions []*envoy_api_v2_core.SocketOption
	for _, so := range sos {
		socketOptions = append(socketOptions, translateSocketOption(so))
	}
	return socketOptions
}

func translateSocketOption(so *core.SocketOption) *envoy_api_v2_core.SocketOption {

	ret := &envoy_api_v2_core.SocketOption{
		Description: so.GetDescription(),
		Level:       so.GetLevel(),
		Name:        so.GetName(),
		State:       translateSocketState(so.GetState()),
	}

	switch typed := so.GetValue().(type) {
	case *core.SocketOption_BufValue:
		ret.Value = &envoy_api_v2_core.SocketOption_BufValue{BufValue: typed.BufValue}
	case *core.SocketOption_IntValue:
		ret.Value = &envoy_api_v2_core.SocketOption_IntValue{IntValue: typed.IntValue}
	}

	return ret
}

func translateSocketState(state core.SocketOption_SocketState) envoy_api_v2_core.SocketOption_SocketState {
	switch stateStr := state.String(); stateStr {
	case core.SocketOption_SocketState_name[int32(core.SocketOption_STATE_PREBIND)]:
		return envoy_api_v2_core.SocketOption_STATE_PREBIND
	case core.SocketOption_SocketState_name[int32(core.SocketOption_STATE_BOUND)]:
		return envoy_api_v2_core.SocketOption_STATE_BOUND
	case core.SocketOption_SocketState_name[int32(core.SocketOption_STATE_LISTENING)]:
		return envoy_api_v2_core.SocketOption_STATE_LISTENING
	}
	return envoy_api_v2_core.SocketOption_STATE_PREBIND
}
