package irtranslator

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func tlsSecret(name string, cert []byte) *envoytlsv3.Secret {
	return &envoytlsv3.Secret{
		Name: name,
		Type: &envoytlsv3.Secret_TlsCertificate{
			TlsCertificate: &envoytlsv3.TlsCertificate{
				CertificateChain: &envoycorev3.DataSource{
					Specifier: &envoycorev3.DataSource_InlineBytes{InlineBytes: cert},
				},
			},
		},
	}
}

// TestTranslationResultMarshalRoundTripPreservesSecrets guards the JSON codec
// symmetry: Secrets are populated during translation (see gateway.go), read by
// UnmarshalJSON, and therefore must also be emitted by MarshalJSON. A previous
// asymmetry dropped Secrets on marshal, so a marshal->unmarshal round-trip
// silently lost every translated SDS secret.
func TestTranslationResultMarshalRoundTripPreservesSecrets(t *testing.T) {
	original := &TranslationResult{
		Secrets: []*envoytlsv3.Secret{
			tlsSecret("server-cert", []byte("cert-a")),
			tlsSecret("client-cert", []byte("cert-b")),
		},
	}

	data, err := original.MarshalJSON()
	require.NoError(t, err)

	var got TranslationResult
	require.NoError(t, got.UnmarshalJSON(data))

	require.Len(t, got.Secrets, len(original.Secrets), "secrets must survive the marshal->unmarshal round-trip")
	if diff := cmp.Diff(original.Secrets, got.Secrets, protocmp.Transform()); diff != "" {
		t.Fatalf("secrets differ after round-trip (-want +got):\n%s", diff)
	}
}

// TestTranslationResultMarshalRoundTrip ensures every field of TranslationResult
// survives a marshal->unmarshal round-trip, so MarshalJSON and UnmarshalJSON
// stay symmetric as fields are added.
func TestTranslationResultMarshalRoundTrip(t *testing.T) {
	original := &TranslationResult{
		Routes:        []*envoyroutev3.RouteConfiguration{{Name: "route"}},
		Listeners:     []*envoylistenerv3.Listener{{Name: "listener"}},
		ExtraClusters: []*envoyclusterv3.Cluster{{Name: "cluster"}},
		Secrets:       []*envoytlsv3.Secret{tlsSecret("server-cert", []byte("cert"))},
	}

	data, err := original.MarshalJSON()
	require.NoError(t, err)

	var got TranslationResult
	require.NoError(t, got.UnmarshalJSON(data))

	if diff := cmp.Diff(original, &got, protocmp.Transform()); diff != "" {
		t.Fatalf("TranslationResult differs after round-trip (-want +got):\n%s", diff)
	}
}
