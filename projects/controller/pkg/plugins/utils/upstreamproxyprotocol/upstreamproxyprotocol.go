package upstream_proxy_protocol

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	proxyproto "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/proxy_protocol/v3"
	"github.com/pkg/errors"

	socketsRaw "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/raw_buffer/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

const (
	proxyProtocolUpstreamClusterName = "envoy.extensions.transport_sockets.proxy_protocol.v3.ProxyProtocolUpstreamTransport"
	// UpstreamProxySocketName is the base name of the transport socket for proxy protocol on an upstream
	UpstreamProxySocketName = "envoy.transport_sockets.upstream_proxy_protocol"
)

// WrapWithPPortocol wraps the upstream with a proxy protocol transport socket
// this is different from the listener level proxy protocol filter as it ends up on the cluster
func WrapWithPProtocol(oldTs *envoy_config_core_v3.TransportSocket, pPVerValStr string) (*envoy_config_core_v3.TransportSocket, error) {
	if pPVerValStr == "" {
		return oldTs, nil
	}
	pPVerVal, ok := envoy_config_core_v3.ProxyProtocolConfig_Version_value[pPVerValStr]
	if !ok {
		// attempt to unroll in case of implementor errors such as using .String()
		if len(pPVerValStr) >= 2 && pPVerValStr[:1] == "\"" {
			pPVerVal, ok = envoy_config_core_v3.ProxyProtocolConfig_Version_value[pPVerValStr[1:len(pPVerValStr)-1]]

		}
		if !ok {
			return oldTs, errors.Errorf("proxy protocol version %s is not supported", pPVerValStr[1:len(pPVerValStr)-1])
		}
	}

	// if unset envoy uses a raw buffer transport socket
	// so explicitly make it here
	if oldTs == nil {
		typedConfig, _ := utils.MessageToAny(&socketsRaw.RawBuffer{})
		oldTs = &envoy_config_core_v3.TransportSocket{Name: wellknown.TransportSocketRawBuffer,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
		}
	}

	pput := &proxyproto.ProxyProtocolUpstreamTransport{
		TransportSocket: oldTs,
		Config: &envoy_config_core_v3.ProxyProtocolConfig{
			Version: envoy_config_core_v3.ProxyProtocolConfig_Version(pPVerVal),
		},
	}
	// Convert so it can be set as typed config
	typCfg, err := utils.MessageToAny(pput)
	if err != nil {
		return nil, err
	}
	typCfg.TypeUrl = "type.googleapis.com/" + proxyProtocolUpstreamClusterName // As of writing this is not in go-control-plane's well known

	newTs := &envoy_config_core_v3.TransportSocket{
		Name: UpstreamProxySocketName,
		// https://github.com/envoyproxy/envoy/blob/29b46144739578a72a8f18eb8eb0855e23426f6e/api/envoy/extensions/transport_sockets/proxy_protocol/v3/upstream_proxy_protocol.proto#L21
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
			TypedConfig: typCfg,
		},
	}
	return newTs, nil
}
