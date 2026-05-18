package backendtlspolicy

import (
	"context"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyproxyprotocolv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/proxy_protocol/v3"
	envoyrawbufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/raw_buffer/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func tlsSocket(t *testing.T, sni string) *envoycorev3.TransportSocket {
	t.Helper()
	tlsAny, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{Sni: sni})
	require.NoError(t, err)
	return &envoycorev3.TransportSocket{
		Name: envoywellknown.TransportSocketTls,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{
			TypedConfig: tlsAny,
		},
	}
}

func proxyProtocolWrappedRawBuffer(t *testing.T) *envoycorev3.TransportSocket {
	t.Helper()
	rawAny, err := utils.MessageToAny(&envoyrawbufferv3.RawBuffer{})
	require.NoError(t, err)
	pp := &envoyproxyprotocolv3.ProxyProtocolUpstreamTransport{
		Config: &envoycorev3.ProxyProtocolConfig{Version: envoycorev3.ProxyProtocolConfig_V2},
		TransportSocket: &envoycorev3.TransportSocket{
			Name: envoywellknown.TransportSocketRawBuffer,
			ConfigType: &envoycorev3.TransportSocket_TypedConfig{
				TypedConfig: rawAny,
			},
		},
	}
	ppAny, err := utils.MessageToAny(pp)
	require.NoError(t, err)
	return &envoycorev3.TransportSocket{
		Name: wellknown.TransportSocketUpstreamProxyProtocol,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{
			TypedConfig: ppAny,
		},
	}
}

// TestProcessBackend_NoExistingSocket: BTP installs its TLS socket when nothing
// is already set.
func TestProcessBackend_NoExistingSocket(t *testing.T) {
	pol := &backendTlsPolicy{transportSocket: tlsSocket(t, "btp.example.com")}
	cluster := &envoyclusterv3.Cluster{}

	processBackend(context.Background(), pol, ir.BackendObjectIR{}, cluster)

	require.NotNil(t, cluster.TransportSocket)
	assert.Equal(t, envoywellknown.TransportSocketTls, cluster.TransportSocket.GetName())
}

// TestProcessBackend_ReplacesInnerOfProxyProtocolWrapper: if BCP already
// wrapped the cluster in upstream proxy protocol with a raw_buffer inner, BTP
// must replace the inner with its TLS socket and keep the wrapper.
func TestProcessBackend_ReplacesInnerOfProxyProtocolWrapper(t *testing.T) {
	pol := &backendTlsPolicy{transportSocket: tlsSocket(t, "btp.example.com")}
	cluster := &envoyclusterv3.Cluster{TransportSocket: proxyProtocolWrappedRawBuffer(t)}

	processBackend(context.Background(), pol, ir.BackendObjectIR{}, cluster)

	require.NotNil(t, cluster.TransportSocket)
	require.Equal(t, wellknown.TransportSocketUpstreamProxyProtocol, cluster.TransportSocket.GetName(),
		"BTP must preserve the upstream_proxy_protocol wrapper")

	pp := &envoyproxyprotocolv3.ProxyProtocolUpstreamTransport{}
	require.NoError(t, cluster.TransportSocket.GetTypedConfig().UnmarshalTo(pp))
	require.NotNil(t, pp.TransportSocket)
	assert.Equal(t, envoywellknown.TransportSocketTls, pp.TransportSocket.GetName(),
		"inner socket should be replaced with BTP's TLS socket")

	inner := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, pp.TransportSocket.GetTypedConfig().UnmarshalTo(inner))
	assert.Equal(t, "btp.example.com", inner.GetSni(), "inner TLS context should come from BTP")
}

// TestProcessBackend_NilPolicySocket: a BTP that did not produce a transport
// socket (e.g. translation error) must not touch the cluster's existing socket.
func TestProcessBackend_NilPolicySocket(t *testing.T) {
	pol := &backendTlsPolicy{}
	existing := proxyProtocolWrappedRawBuffer(t)
	cluster := &envoyclusterv3.Cluster{TransportSocket: existing}

	processBackend(context.Background(), pol, ir.BackendObjectIR{}, cluster)
	assert.Same(t, existing, cluster.TransportSocket)
}
