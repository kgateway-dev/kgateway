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

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
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

const testCAPEM = "-----BEGIN CERTIFICATE-----\ntest-ca\n-----END CERTIFICATE-----"

// tlsSocketWithCA returns the socket buildTranslateFunc produces for a policy that names a CA
// bundle: a TLS context validating against that bundle inline, under the given SNI.
func tlsSocketWithCA(t *testing.T, sni, caPEM string) *envoycorev3.TransportSocket {
	t.Helper()
	tlsCtx, err := pluginutils.ResolveUpstreamSslConfigFromCA(caPEM, &envoytlsv3.CertificateValidationContext{}, sni)
	require.NoError(t, err)
	tlsAny, err := utils.MessageToAny(tlsCtx)
	require.NoError(t, err)
	return &envoycorev3.TransportSocket{
		Name: envoywellknown.TransportSocketTls,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{
			TypedConfig: tlsAny,
		},
	}
}

// systemCASocket returns the socket buildTranslateFunc produces for a policy selecting the
// well-known system CA set: a combined validation context that names no trusted CA inline.
func systemCASocket(t *testing.T, sni string) *envoycorev3.TransportSocket {
	t.Helper()
	tlsAny, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{
		CommonTlsContext: &envoytlsv3.CommonTlsContext{
			ValidationContextType: &envoytlsv3.CommonTlsContext_CombinedValidationContext{
				CombinedValidationContext: &envoytlsv3.CommonTlsContext_CombinedCertificateValidationContext{
					DefaultValidationContext: &envoytlsv3.CertificateValidationContext{},
				},
			},
		},
		Sni: sni,
	})
	require.NoError(t, err)
	return &envoycorev3.TransportSocket{
		Name: envoywellknown.TransportSocketTls,
		ConfigType: &envoycorev3.TransportSocket_TypedConfig{
			TypedConfig: tlsAny,
		},
	}
}

// TestUpstreamTLSValidation: the CA this policy validates the backend against is exposed so
// that a control-plane client — OIDC discovery — can trust it too, instead of being stuck
// with the system trust store. See kgateway-dev/kgateway#14062.
func TestUpstreamTLSValidation(t *testing.T) {
	tests := []struct {
		name string
		pol  *backendTlsPolicy
		want *ir.UpstreamTLSValidation
	}{
		{
			name: "nil policy",
			pol:  nil,
			want: nil,
		},
		{
			name: "policy that failed to translate configures nothing",
			pol:  &backendTlsPolicy{},
			want: nil,
		},
		{
			name: "explicit CA bundle",
			pol:  &backendTlsPolicy{transportSocket: tlsSocketWithCA(t, "example.com", testCAPEM)},
			want: &ir.UpstreamTLSValidation{CAPEM: testCAPEM},
		},
		{
			// Only trust anchors are exposed. The policy's hostname is the identity Envoy
			// verifies for the backend, which is not necessarily the issuer a control-plane
			// client dials, so it deliberately does not travel with the CA.
			name: "the policy's hostname is not exposed as an identity",
			pol:  &backendTlsPolicy{transportSocket: tlsSocketWithCA(t, "sni.example.com", testCAPEM)},
			want: &ir.UpstreamTLSValidation{CAPEM: testCAPEM},
		},
		{
			name: "system CA certificates resolve to an empty bundle",
			pol:  &backendTlsPolicy{transportSocket: systemCASocket(t, "example.com")},
			want: &ir.UpstreamTLSValidation{},
		},
		{
			name: "a TLS context with no validation at all resolves to an empty bundle",
			pol:  &backendTlsPolicy{transportSocket: tlsSocket(t, "example.com")},
			want: &ir.UpstreamTLSValidation{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.pol.UpstreamTLSValidation())
		})
	}
}

// TestEqualsObservesCARotation guards KRT change detection: rotating the CA has to be observed,
// or the discovery cache keeps serving a config obtained under the old trust material. The CA
// lives inside the transport socket, so comparing the socket is what observes it.
func TestEqualsObservesCARotation(t *testing.T) {
	base := &backendTlsPolicy{transportSocket: tlsSocketWithCA(t, "example.com", testCAPEM)}

	assert.True(t, base.Equals(&backendTlsPolicy{transportSocket: tlsSocketWithCA(t, "example.com", testCAPEM)}),
		"identical policies should compare equal")
	assert.False(t, base.Equals(&backendTlsPolicy{transportSocket: tlsSocketWithCA(t, "example.com", "rotated")}),
		"a rotated CA must not compare equal")
	assert.False(t, base.Equals(&backendTlsPolicy{transportSocket: systemCASocket(t, "example.com")}),
		"moving to the system CA set must not compare equal")
	assert.False(t, base.Equals(&backendTlsPolicy{transportSocket: tlsSocketWithCA(t, "other.example.com", testCAPEM)}),
		"a changed hostname must not compare equal")
}
