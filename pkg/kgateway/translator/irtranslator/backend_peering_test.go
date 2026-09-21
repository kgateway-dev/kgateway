package irtranslator

import (
	"testing"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	proxy "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/proxy_protocol/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestPeeringPreservesTLS(t *testing.T) {
	config, err := anypb.New(&tls.UpstreamTlsContext{})
	require.NoError(t, err)
	socket := &core.TransportSocket{Name: "envoy.transport_sockets.tls", ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: config}}
	base := &cluster.Cluster{TransportSocket: socket}
	wrapped, err := anypb.New(&proxy.ProxyProtocolUpstreamTransport{TransportSocket: socket})
	require.NoError(t, err)
	// Match the socket transformation in a peering transport-socket overlay.
	out := &cluster.Cluster{TransportSocketMatches: []*cluster.Cluster_TransportSocketMatch{
		{Name: "multinet-upstream-proxy-proto", Match: &structpb.Struct{Fields: map[string]*structpb.Value{"outbound-proxy": structpb.NewBoolValue(true)}}, TransportSocket: &core.TransportSocket{Name: "envoy.transport_sockets.upstream_proxy_protocol", ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: wrapped}}},
		{Name: "default", TransportSocket: socket},
	}}
	require.NoError(t, validateGatewayClientIdentityOverlay(base, out), "moving TLS into fallback and PROXY matches preserves TLS")
}

func TestGatewayCertificateInsideProxyWrapper(t *testing.T) {
	config, err := anypb.New(&tls.UpstreamTlsContext{})
	require.NoError(t, err)
	socket := &core.TransportSocket{Name: "envoy.transport_sockets.tls", ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: config}}
	wrapped, err := anypb.New(&proxy.ProxyProtocolUpstreamTransport{TransportSocket: socket})
	require.NoError(t, err)
	outer := &core.TransportSocket{Name: "envoy.transport_sockets.upstream_proxy_protocol", ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: wrapped}}
	result, err := injectGatewayBackendClientCertificate(outer, ir.TLSCertificate{CertChain: []byte("gateway-cert"), PrivateKey: []byte("gateway-key")})
	require.NoError(t, err)
	require.NotNil(t, result)
	decoded := &proxy.ProxyProtocolUpstreamTransport{}
	require.NoError(t, result.GetTypedConfig().UnmarshalTo(decoded))
	ctx := &tls.UpstreamTlsContext{}
	require.NoError(t, decoded.TransportSocket.GetTypedConfig().UnmarshalTo(ctx))
	require.Equal(t, "gateway-cert", ctx.GetCommonTlsContext().GetTlsCertificates()[0].GetCertificateChain().GetInlineString())
	require.True(t, proto.Equal(outer.GetTypedConfig(), wrapped), "injection must not mutate shared wrappers")
}

func TestGatewayIdentityRejectsPlaintextMatchBeforeTLSFallback(t *testing.T) {
	socket := peeringTestTLSSocket(t)
	base := &cluster.Cluster{TransportSocket: socket}
	out := &cluster.Cluster{TransportSocket: socket, TransportSocketMatches: []*cluster.Cluster_TransportSocketMatch{{
		Name: "plaintext", Match: &structpb.Struct{Fields: map[string]*structpb.Value{"proxy": structpb.NewBoolValue(true)}},
		TransportSocket: &core.TransportSocket{Name: "envoy.transport_sockets.raw_buffer"},
	}}}
	require.ErrorContains(t, validateGatewayClientIdentityOverlay(base, out), "gateway backend client certificate")
}

func TestGatewayIdentityPreservesExistingMixedMatches(t *testing.T) {
	socket := peeringTestTLSSocket(t)
	base := &cluster.Cluster{TransportSocketMatches: []*cluster.Cluster_TransportSocketMatch{
		{Name: "tls", Match: &structpb.Struct{Fields: map[string]*structpb.Value{"tlsMode": structpb.NewStringValue("istio")}}, TransportSocket: socket},
		{Name: "raw", TransportSocket: &core.TransportSocket{Name: "envoy.transport_sockets.raw_buffer"}},
	}}
	require.NoError(t, validateGatewayClientIdentityOverlay(base, proto.Clone(base).(*cluster.Cluster)))
}

func peeringTestTLSSocket(t *testing.T) *core.TransportSocket {
	t.Helper()
	config, err := anypb.New(&tls.UpstreamTlsContext{})
	require.NoError(t, err)
	return &core.TransportSocket{Name: "envoy.transport_sockets.tls", ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: config}}
}
