package irtranslator

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestInitializeHCMStatPrefix(t *testing.T) {
	tests := []struct {
		name string
		fcc  ir.FilterChainCommon
		want string
	}{
		{
			name: "uses StatPrefix, not FilterChainName",
			fcc:  ir.FilterChainCommon{FilterChainName: "listener~443-ignored", StatPrefix: "https_443_web"},
			want: "https_443_web",
		},
		{
			name: "replaces dots",
			fcc:  ir.FilterChainCommon{StatPrefix: "https_443_team-a_shop.v2_web"},
			want: "https_443_team-a_shop_v2_web",
		},
		{
			name: "defaults to http when unset",
			fcc:  ir.FilterChainCommon{FilterChainName: "listener~8080"},
			want: DefaultHttpStatPrefix,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &hcmNetworkFilterTranslator{listener: ir.HttpFilterChainIR{FilterChainCommon: tt.fcc}}
			require.Equal(t, tt.want, h.initializeHCM().GetStatPrefix())
		})
	}
}

func TestStatPrefixOrDefault(t *testing.T) {
	require.Equal(t, "tcp_8080_default_example", statPrefixOrDefault(ir.FilterChainCommon{StatPrefix: "tcp_8080_default_example"}, "unused"))
	require.Equal(t, "listener~8080-default_example-rule-0", statPrefixOrDefault(ir.FilterChainCommon{}, "listener~8080-default.example-rule-0"), "unset falls back to the default, also sanitized")
}

func TestFilterChainInfoDefaultAlpn(t *testing.T) {
	info := &FilterChainInfo{
		TLS: &ir.TLSConfig{
			Certificates: []ir.TLSCertificate{
				{
					CertChain:  []byte("cert"),
					PrivateKey: []byte("key"),
				},
			},
		},
	}

	downstream := extractDownstreamTlsContext(t, info.toTransportSocket())
	require.Equal(t, []string{"h2", "http/1.1"}, downstream.GetCommonTlsContext().GetAlpnProtocols())
}

func TestFilterChainInfoCustomAlpn(t *testing.T) {
	custom := []string{"grpc"}
	info := &FilterChainInfo{
		TLS: &ir.TLSConfig{
			Certificates: []ir.TLSCertificate{
				{
					CertChain:  []byte("cert"),
					PrivateKey: []byte("key"),
				},
			},
			AlpnProtocols: custom,
		},
	}

	downstream := extractDownstreamTlsContext(t, info.toTransportSocket())
	require.Equal(t, custom, downstream.GetCommonTlsContext().GetAlpnProtocols())
}

func TestFilterChainInfoSingleCertificate(t *testing.T) {
	info := &FilterChainInfo{
		TLS: &ir.TLSConfig{
			Certificates: []ir.TLSCertificate{
				{
					CertChain:  []byte("cert"),
					PrivateKey: []byte("key"),
				},
			},
		},
	}

	downstream := extractDownstreamTlsContext(t, info.toTransportSocket())
	require.Equal(
		t,
		[]*envoytlsv3.TlsCertificate{
			{
				CertificateChain: bytesDataSource([]byte("cert")),
				PrivateKey:       bytesDataSource([]byte("key")),
			},
		},
		downstream.GetCommonTlsContext().GetTlsCertificates(),
	)
}

func TestFilterChainInfoMultipleCertificates(t *testing.T) {
	info := &FilterChainInfo{
		TLS: &ir.TLSConfig{
			Certificates: []ir.TLSCertificate{
				{
					CertChain:  []byte("cert1"),
					PrivateKey: []byte("key1"),
				},
				{
					CertChain:  []byte("cert2"),
					PrivateKey: []byte("key2"),
				},
			},
		},
	}

	downstream := extractDownstreamTlsContext(t, info.toTransportSocket())
	require.Equal(
		t,
		[]*envoytlsv3.TlsCertificate{
			{
				CertificateChain: bytesDataSource([]byte("cert1")),
				PrivateKey:       bytesDataSource([]byte("key1")),
			},
			{
				CertificateChain: bytesDataSource([]byte("cert2")),
				PrivateKey:       bytesDataSource([]byte("key2")),
			},
		},
		downstream.GetCommonTlsContext().GetTlsCertificates(),
	)
}

func TestFilterChainInfoFrontendTLSAllowInsecureFallback(t *testing.T) {
	info := &FilterChainInfo{
		TLS: &ir.TLSConfig{
			Certificates: []ir.TLSCertificate{{
				CertChain:  []byte("cert"),
				PrivateKey: []byte("key"),
			}},
			ClientCertificateValidation: &ir.ClientCertificateValidation{
				CACertificates:           [][]byte{[]byte("ca-cert")},
				RequireClientCertificate: false,
				AllowInsecureFallback:    true,
			},
		},
	}

	downstream := extractDownstreamTlsContext(t, info.toTransportSocket())
	require.False(t, downstream.GetRequireClientCertificate().GetValue())
	require.Equal(
		t,
		envoytlsv3.CertificateValidationContext_ACCEPT_UNTRUSTED,
		downstream.GetCommonTlsContext().GetValidationContext().GetTrustChainVerification(),
	)
}

func TestFilterChainInfoOptionalClientValidationStaysStrict(t *testing.T) {
	info := &FilterChainInfo{
		TLS: &ir.TLSConfig{
			Certificates: []ir.TLSCertificate{{
				CertChain:  []byte("cert"),
				PrivateKey: []byte("key"),
			}},
			ClientCertificateValidation: &ir.ClientCertificateValidation{
				CACertificates:           [][]byte{[]byte("ca-cert")},
				RequireClientCertificate: false,
			},
		},
	}

	downstream := extractDownstreamTlsContext(t, info.toTransportSocket())
	require.False(t, downstream.GetRequireClientCertificate().GetValue())
	require.Equal(
		t,
		envoytlsv3.CertificateValidationContext_VERIFY_TRUST_CHAIN,
		downstream.GetCommonTlsContext().GetValidationContext().GetTrustChainVerification(),
	)
}

func extractDownstreamTlsContext(t *testing.T, socket *envoycorev3.TransportSocket) *envoytlsv3.DownstreamTlsContext {
	t.Helper()
	require.NotNil(t, socket)
	typedConfig := socket.GetTypedConfig()
	require.NotNil(t, typedConfig)

	ctx := &envoytlsv3.DownstreamTlsContext{}
	require.NoError(t, typedConfig.UnmarshalTo(ctx))
	return ctx
}
