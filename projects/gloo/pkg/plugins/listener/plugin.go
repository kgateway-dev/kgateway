package listener

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	tcp_stats_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tcp_stats/v3"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"

	socketsRaw "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/raw_buffer/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.ListenerPlugin = new(plugin)
)

const (
	ExtensionName = "listener"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

// Used to set config that are directly on the [Envoy listener](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/listener/v3/listener.proto)
func (p *plugin) ProcessListener(_ plugins.Params, in *v1.Listener, out *envoy_config_listener_v3.Listener) error {
	if in.GetOptions().GetPerConnectionBufferLimitBytes().GetValue() != 0 {
		out.PerConnectionBufferLimitBytes = in.GetOptions().GetPerConnectionBufferLimitBytes()
	}

	if in.GetOptions().GetSocketOptions() != nil {
		out.SocketOptions = translateSocketOptions(in.GetOptions().GetSocketOptions())
	}

	if connectionBalanceConfig := in.GetOptions().GetConnectionBalanceConfig(); connectionBalanceConfig != nil {
		if connectionBalanceConfig.GetExactBalance() != nil {
			out.ConnectionBalanceConfig = &envoy_config_listener_v3.Listener_ConnectionBalanceConfig{
				BalanceType: &envoy_config_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance_{
					ExactBalance: &envoy_config_listener_v3.Listener_ConnectionBalanceConfig_ExactBalance{},
				},
			}
		} else {
			return errors.New("connection balancer does not specify balancer type")
		}
	}

	if tcpStatsWrap := in.GetOptions().GetTcpStats().GetValue(); tcpStatsWrap {
		for _, chain := range out.GetFilterChains() {
			if chain != nil {
				tSock := chain.GetTransportSocket()
				newS, err := wrapWithTcpStats(tSock)
				if err != nil {
					return err
				}
				chain.TransportSocket = newS
			}
		}
	}

	return nil
}

// wrapWithTcpStats wraps the existing transport socket with a tcp_stats transport socket
// if there is no transport socket defined, will create a new raw buffer transport
// (what envoy would use anyway) and wraps that
func wrapWithTcpStats(oldTs *envoy_config_core_v3.TransportSocket) (*envoy_config_core_v3.TransportSocket, error) {
	// if unset envoy uses a raw buffer transport socket
	// so explicitly make it here
	if oldTs == nil {
		typedConfig, _ := utils.MessageToAny(&socketsRaw.RawBuffer{})
		oldTs = &envoy_config_core_v3.TransportSocket{Name: wellknown.TransportSocketRawBuffer,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
		}
	}

	tcpStats := &tcp_stats_v3.Config{
		TransportSocket: oldTs,
	}
	// Convert so it can be set as typed config
	typCfg, err := utils.MessageToAny(tcpStats)
	if err != nil {
		return nil, err
	}
	typCfg.TypeUrl = "type.googleapis.com/" + "envoy.extensions.transport_sockets.tcp_stats.v3.Config" // As of writing this is not in go-control-plane's well known

	newTs := &envoy_config_core_v3.TransportSocket{
		Name: "envoy.transport_sockets.tcp_stats",
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
			TypedConfig: typCfg,
		},
	}
	return newTs, nil
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
