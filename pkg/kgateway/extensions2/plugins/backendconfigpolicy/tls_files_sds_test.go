package backendconfigpolicy

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
)

func TestSplitFileTLS(t *testing.T) {
	fileCert := &envoytlsv3.TlsCertificate{
		CertificateChain: pluginutils.FileDataSource("/etc/mtls/tls.crt"),
		PrivateKey:       pluginutils.FileDataSource("/etc/mtls/tls.key"),
	}
	fileCA := &envoytlsv3.CertificateValidationContext{
		TrustedCa: pluginutils.FileDataSource("/etc/mtls/ca.crt"),
	}

	tests := []struct {
		name        string
		in          *envoytlsv3.UpstreamTlsContext
		wantCtx     *envoytlsv3.UpstreamTlsContext
		wantSecrets []*envoytlsv3.Secret
	}{
		{
			name: "inline material is left in place",
			in: &envoytlsv3.UpstreamTlsContext{CommonTlsContext: &envoytlsv3.CommonTlsContext{
				TlsCertificates: []*envoytlsv3.TlsCertificate{{
					CertificateChain: pluginutils.InlineStringDataSource(CACert),
					PrivateKey:       pluginutils.InlineStringDataSource(TLSKey),
				}},
			}},
		},
		{
			name: "file cert and trust bundle move to per-cluster secrets",
			in: &envoytlsv3.UpstreamTlsContext{
				Sni: "backend.example.com",
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					TlsCertificates:       []*envoytlsv3.TlsCertificate{fileCert},
					ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{ValidationContext: fileCA},
				},
			},
			wantCtx: &envoytlsv3.UpstreamTlsContext{
				Sni: "backend.example.com",
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					TlsCertificateSdsSecretConfigs: []*envoytlsv3.SdsSecretConfig{adsSecretConfig("tls_files_c1_cert")},
					ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
						ValidationContextSdsSecretConfig: adsSecretConfig("tls_files_c1_ca"),
					},
				},
			},
			wantSecrets: []*envoytlsv3.Secret{
				{Name: "tls_files_c1_cert", Type: &envoytlsv3.Secret_TlsCertificate{TlsCertificate: fileCert}},
				{Name: "tls_files_c1_ca", Type: &envoytlsv3.Secret_ValidationContext{ValidationContext: fileCA}},
			},
		},
		{
			name: "file trust bundle alone keeps its SAN matchers in the secret",
			in: &envoytlsv3.UpstreamTlsContext{CommonTlsContext: &envoytlsv3.CommonTlsContext{
				ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{ValidationContext: &envoytlsv3.CertificateValidationContext{
					TrustedCa: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: "/etc/ssl/certs/ca-certificates.crt"}},
					MatchTypedSubjectAltNames: []*envoytlsv3.SubjectAltNameMatcher{{
						SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					}},
				}},
			}},
			wantCtx: &envoytlsv3.UpstreamTlsContext{CommonTlsContext: &envoytlsv3.CommonTlsContext{
				ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
					ValidationContextSdsSecretConfig: adsSecretConfig("tls_files_c1_ca"),
				},
			}},
			wantSecrets: []*envoytlsv3.Secret{{
				Name: "tls_files_c1_ca",
				Type: &envoytlsv3.Secret_ValidationContext{ValidationContext: &envoytlsv3.CertificateValidationContext{
					TrustedCa: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: "/etc/ssl/certs/ca-certificates.crt"}},
					MatchTypedSubjectAltNames: []*envoytlsv3.SubjectAltNameMatcher{{
						SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					}},
				}},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCtx, gotSecrets := splitFileTLS(tt.in, "c1")
			wantCtx := tt.wantCtx
			if wantCtx == nil {
				wantCtx = tt.in
				assert.Same(t, tt.in, gotCtx, "unchanged input should be returned as-is")
			}
			assert.Empty(t, cmp.Diff(wantCtx, gotCtx, protocmp.Transform()))
			assert.Empty(t, cmp.Diff(tt.wantSecrets, gotSecrets, protocmp.Transform()))
		})
	}
}
