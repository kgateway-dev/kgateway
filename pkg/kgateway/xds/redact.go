package xds

import (
	"slices"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

// RedactedValue replaces credential values in diagnostic output.
const RedactedValue = "[REDACTED]"

// RedactListenerCredentials redacts CONNECT headers missed by Envoy's sensitive
// annotations. It clones the listener only when redaction is needed.
func RedactListenerCredentials(l *envoylistenerv3.Listener) *envoylistenerv3.Listener {
	var out *envoylistenerv3.Listener
	for i, chain := range filterChains(l) {
		for j, f := range chain.GetFilters() {
			config := redactTCPProxyHeaders(f)
			if config == nil {
				continue
			}
			if out == nil {
				out = proto.Clone(l).(*envoylistenerv3.Listener)
			}
			filterChains(out)[i].Filters[j].ConfigType = &envoylistenerv3.Filter_TypedConfig{TypedConfig: config}
		}
	}
	if out == nil {
		return l
	}
	return out
}

func filterChains(l *envoylistenerv3.Listener) []*envoylistenerv3.FilterChain {
	return append(slices.Clone(l.GetFilterChains()), l.GetDefaultFilterChain())
}

// redactTCPProxyHeaders redacts CONNECT headers, suppressing unreadable configs
// rather than risking credential disclosure.
func redactTCPProxyHeaders(f *envoylistenerv3.Filter) *anypb.Any {
	if f.GetName() != envoywellknown.TCPProxy || f.GetTypedConfig() == nil {
		return nil
	}
	unreadable := &anypb.Any{TypeUrl: f.GetTypedConfig().GetTypeUrl()}
	tcpProxy := &envoytcp.TcpProxy{}
	if err := f.GetTypedConfig().UnmarshalTo(tcpProxy); err != nil {
		return unreadable
	}
	headers := tcpProxy.GetTunnelingConfig().GetHeadersToAdd()
	if len(headers) == 0 {
		return nil
	}
	for _, h := range headers {
		if h.GetHeader() != nil {
			h.Header.Value = RedactedValue
			h.Header.RawValue = nil
		}
	}
	config, err := utils.MessageToAny(tcpProxy)
	if err != nil {
		return unreadable
	}
	return config
}
