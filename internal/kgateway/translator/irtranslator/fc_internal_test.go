package irtranslator

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

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

func extractDownstreamTlsContext(t *testing.T, socket *envoycorev3.TransportSocket) *envoytlsv3.DownstreamTlsContext {
	t.Helper()
	require.NotNil(t, socket)
	typedConfig := socket.GetTypedConfig()
	require.NotNil(t, typedConfig)

	ctx := &envoytlsv3.DownstreamTlsContext{}
	require.NoError(t, typedConfig.UnmarshalTo(ctx))
	return ctx
}
