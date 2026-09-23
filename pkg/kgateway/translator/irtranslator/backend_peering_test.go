package irtranslator

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	proxy "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/proxy_protocol/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestPeeringPreservesTLS(t *testing.T) {
	config, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{})
	require.NoError(t, err)
	socket := &envoycorev3.TransportSocket{Name: "envoy.transport_sockets.tls", ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: config}}
	base := &envoyclusterv3.Cluster{TransportSocket: socket}
	wrapped, err := utils.MessageToAny(&proxy.ProxyProtocolUpstreamTransport{TransportSocket: socket})
	require.NoError(t, err)
	// Match the socket transformation in a peering transport-socket overlay.
	out := &envoyclusterv3.Cluster{TransportSocketMatches: []*envoyclusterv3.Cluster_TransportSocketMatch{
		{Name: "multinet-upstream-proxy-proto", Match: &structpb.Struct{Fields: map[string]*structpb.Value{"outbound-proxy": structpb.NewBoolValue(true)}}, TransportSocket: &envoycorev3.TransportSocket{Name: "envoy.transport_sockets.upstream_proxy_protocol", ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: wrapped}}},
		{Name: "default", TransportSocket: socket},
	}}
	require.NoError(t, validateGatewayClientIdentityOverlay(base, out), "moving TLS into fallback and PROXY matches preserves TLS")
}

func TestGatewayCertificateInsideProxyWrapper(t *testing.T) {
	config, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{})
	require.NoError(t, err)
	socket := &envoycorev3.TransportSocket{Name: "envoy.transport_sockets.tls", ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: config}}
	wrapped, err := utils.MessageToAny(&proxy.ProxyProtocolUpstreamTransport{TransportSocket: socket})
	require.NoError(t, err)
	outer := &envoycorev3.TransportSocket{Name: "envoy.transport_sockets.upstream_proxy_protocol", ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: wrapped}}
	result, err := injectGatewayBackendClientCertificate(outer, ir.TLSCertificate{CertChain: []byte("gateway-cert"), PrivateKey: []byte("gateway-key")})
	require.NoError(t, err)
	require.NotNil(t, result)
	decoded := &proxy.ProxyProtocolUpstreamTransport{}
	require.NoError(t, result.GetTypedConfig().UnmarshalTo(decoded))
	ctx := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, decoded.TransportSocket.GetTypedConfig().UnmarshalTo(ctx))
	require.Equal(t, "gateway-cert", ctx.GetCommonTlsContext().GetTlsCertificates()[0].GetCertificateChain().GetInlineString())
	require.True(t, proto.Equal(outer.GetTypedConfig(), wrapped), "injection must not mutate shared wrappers")
}

func TestGatewayIdentityRejectsPlaintextMatchBeforeTLSFallback(t *testing.T) {
	socket := peeringTestTLSSocket(t)
	base := &envoyclusterv3.Cluster{TransportSocket: socket}
	out := &envoyclusterv3.Cluster{TransportSocket: socket, TransportSocketMatches: []*envoyclusterv3.Cluster_TransportSocketMatch{{
		Name: "plaintext", Match: &structpb.Struct{Fields: map[string]*structpb.Value{"proxy": structpb.NewBoolValue(true)}},
		TransportSocket: &envoycorev3.TransportSocket{Name: "envoy.transport_sockets.raw_buffer"},
	}}}
	require.ErrorContains(t, validateGatewayClientIdentityOverlay(base, out), "gateway backend client certificate")
}

func TestGatewayIdentityPreservesExistingMixedMatches(t *testing.T) {
	socket := peeringTestTLSSocket(t)
	base := &envoyclusterv3.Cluster{TransportSocketMatches: []*envoyclusterv3.Cluster_TransportSocketMatch{
		{Name: "tls", Match: &structpb.Struct{Fields: map[string]*structpb.Value{"tlsMode": structpb.NewStringValue("istio")}}, TransportSocket: socket},
		{Name: "raw", TransportSocket: &envoycorev3.TransportSocket{Name: "envoy.transport_sockets.raw_buffer"}},
	}}
	require.NoError(t, validateGatewayClientIdentityOverlay(base, proto.Clone(base).(*envoyclusterv3.Cluster)))
}

func peeringTestTLSSocket(t *testing.T) *envoycorev3.TransportSocket {
	t.Helper()
	config, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{})
	require.NoError(t, err)
	return &envoycorev3.TransportSocket{Name: "envoy.transport_sockets.tls", ConfigType: &envoycorev3.TransportSocket_TypedConfig{TypedConfig: config}}
}
