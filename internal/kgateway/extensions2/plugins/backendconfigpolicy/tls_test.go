package backendconfigpolicy

import (
	"fmt"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	gwv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	eiutils "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

// MockSecretGetter implements SecretGetter for testing
type MockSecretGetter struct {
	secrets map[string]*ir.Secret
}

func NewMockSecretGetter() *MockSecretGetter {
	return &MockSecretGetter{
		secrets: make(map[string]*ir.Secret),
	}
}

func (m *MockSecretGetter) AddSecret(name, namespace string, secret *ir.Secret) {
	key := namespace + "/" + name
	m.secrets[key] = secret
}

func (m *MockSecretGetter) GetSecret(name, namespace string) (*ir.Secret, error) {
	key := namespace + "/" + name
	if secret, ok := m.secrets[key]; ok {
		return secret, nil
	}
	return nil, fmt.Errorf("secret %s/%s not found", namespace, name)
}

// openssl req -x509 -newkey rsa:2048 -keyout test.key -out test.crt -days 365 -nodes -subj "/CN=test.example.com" -addext "subjectAltName=DNS:test.example.com"
var CACert = `-----BEGIN CERTIFICATE-----
MIIDNDCCAhygAwIBAgIUL6jJHHVicPbTrxNXjTX2ti/2swgwDQYJKoZIhvcNAQEL
BQAwGzEZMBcGA1UEAwwQdGVzdC5leGFtcGxlLmNvbTAeFw0yNTA1MjkxNTQ5MTha
Fw0yNjA1MjkxNTQ5MThaMBsxGTAXBgNVBAMMEHRlc3QuZXhhbXBsZS5jb20wggEi
MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQC0zE9AuZN4Uc5VOsUbLYHZaEh/
db2HiHsYdyxpuLx1C2aXYUpIyGjwVSs84+TwS46XRCstZHsTDrSvlM6hwU2x+B7E
FksEM5TPU/0e6+lUde0yUweiYCYIKnJU1PzWO7pldS8K8ayvTYbIMSWawzCgWeq1
OWPgwCfSK0GF2MyfhfAqMYazZB9rZhGycyaaE1iKX97JyYU79klhnaEdZE3bhCNr
wH2s5h55jbIrizUAbjz6+t5B+euakUrfKCeGXfCb3TNz48IEWdNIMPmyfgSWzXlz
MXKpfZ0tza6SzeqrDLZN2nl/YydM1yHmI7MALrIXJo0hXk4N469f/MIdCKZdAgMB
AAGjcDBuMB0GA1UdDgQWBBS1oJXQN8/QuWWlo+UfZe2SKxy2ezAfBgNVHSMEGDAW
gBS1oJXQN8/QuWWlo+UfZe2SKxy2ezAPBgNVHRMBAf8EBTADAQH/MBsGA1UdEQQU
MBKCEHRlc3QuZXhhbXBsZS5jb20wDQYJKoZIhvcNAQELBQADggEBAFtjff8nA/+I
2vLVq6SE3eLe/x4w09RtpdNZ+qirAQsbu0DrI1F9/MNxSYhKMA+4DCj1OXpUaaPO
mwZIwEtFklUyDqz8aaBK8xCBjzvc++rbaiY2XVDo+/e6ML0c90LXyGI3pDK6bUU1
15dFeYikl+7iVf4L+DrWgj7imK5LtWqKS7VTUX/+yFnA19d7LJF2/uOnprIeEHsj
LSlVx4yPJjGQYighFyK6VQKi3rsiuFU/LsedNEq2kJonn/NfT9pCvoReQqjijlyS
D8sD7wlIiyowZO09KIU7MUfPUqGlGsNXQ9Hy9sHJgPmsz4ZM4NofSOdt8MGETulJ
Tr8dXUTlbn0=
-----END CERTIFICATE-----
`

var TLSKey = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC0zE9AuZN4Uc5V
OsUbLYHZaEh/db2HiHsYdyxpuLx1C2aXYUpIyGjwVSs84+TwS46XRCstZHsTDrSv
lM6hwU2x+B7EFksEM5TPU/0e6+lUde0yUweiYCYIKnJU1PzWO7pldS8K8ayvTYbI
MSWawzCgWeq1OWPgwCfSK0GF2MyfhfAqMYazZB9rZhGycyaaE1iKX97JyYU79klh
naEdZE3bhCNrwH2s5h55jbIrizUAbjz6+t5B+euakUrfKCeGXfCb3TNz48IEWdNI
MPmyfgSWzXlzMXKpfZ0tza6SzeqrDLZN2nl/YydM1yHmI7MALrIXJo0hXk4N469f
/MIdCKZdAgMBAAECggEAFLPqhVZauSXg8yiCJo0M9+i1mIbSd6Ecu132I3sIdXyj
OEVnPLNaNN8Dzvqnng6A2vhu20lMwI9oCE0JZkNc0rq/RyPoXihL63vKGc7Yzpec
XC1ey+ynnjrCEc270ApR20lSZDXtWLuPagAatsCQImR5eFwEgFlwlePnIl0DfWan
JQQYf5hbayLXwcoaDXxCB8rmkGpwBsamYVDjLgxjxmQwjMf809jWi16OM6mIgXot
H4ZowMj26HbKhBZqpM85hzliHNAsNuCnfSQJGSeJzMvJR/UBRnnofDPKhCdeoIMt
7iu5uMMd42h1tYIgk9KFw3S24G3GjRYIb3VpqfaMPwKBgQDa1iZA1BLb2rzWayrE
Tq43dMM9n3seMOx12VaA9MPfGMJh/uJEgQXH8MxvbsRhTw2IUzT4eJ/3SxHQF1ru
G8421IZQPShE3/1J1vRngE9EUMKfO8fLKVIM7VzugFTBN4HB/raDbtH6go2Qg/t+
UDzFLv1qt3Mjmbluwvr2Cw0kLwKBgQDTgHNm53VSHUyrgVkAl6BvFO6S07pNsKIe
LCWcIIXDLjat7kYgnaMXkmNuSsfPesYeq9kyLPh4YYUTJlSpWZPkm+8NtjPSLwa2
phxX5AIiC/ZZEutTkQy+a3KERjE5sW4dJbjeFXNjqO4f1v/L03hcfaTeFK6zkz0v
LpJhXpNfMwKBgQCVcVcQQINcdoUs3GSJSL36ixdltspqNLjWRgSn7f7xFMRyDZDR
fVbIUq4Zjwg297hjF4d+A0oio7ZXaAulvYFWuk267/jXCCu9yDiBkgMPwSMXgMiQ
+ffZciNbkHHQvSo0o9BZ800cCRnJzgfqG7tUYSGYRg0wC6Oxex/M9IEV6wKBgD8t
B0udp7W3esdgA63hnNKRdhH1nJjIQiSxGyrfrBT5IOwjWF81txm7aGfxfm3DRpqy
ylXqiO2sc4ucz30mfL60tVtrKV+HHIJCbAT03o489ID23cRAd4YJolNQhDOvhCzA
r8/mqGkEdNyd5BqGOFWoUi7kDqslOAl359Gd5ndxAoGAK3TVwhuLR9XoicDjmo6b
6qtYp/ln1Sx61ERo4Vaz/EdMCeVBD/DH3g0trdjI6XgFBJjuMvrz6LaRpvPkIxul
8VsYXhVwMPnyJzEd+wpEsIgIh5W9YluY0f5TxqcQkRGPUW5Sb5dXuk9BXRtaBzR0
35NY368cSzjvlBCisA91TbY=
-----END PRIVATE KEY-----
` // must have this newline at the end

func TestTranslateTLSConfig(t *testing.T) {
	tests := []struct {
		name      string
		tlsConfig *v1alpha1.TLS
		secret    *ir.Secret
		wantErr   bool
		expected  *envoytlsv3.UpstreamTlsContext
	}{
		{
			name: "secret-based TLS config",
			tlsConfig: &v1alpha1.TLS{
				SecretRef: &corev1.LocalObjectReference{
					Name: "test-secret",
				},
				Sni:                ptr.To("test.example.com"),
				AllowRenegotiation: ptr.To(true),
				AlpnProtocols:      []string{"h2", "http/1.1"},
			},
			secret: &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
					Namespace: "default",
					Name:      "test-secret",
				},
				Obj: &corev1.Secret{},
				Data: map[string][]byte{
					"tls.crt": []byte(CACert),
					"tls.key": []byte(TLSKey),
					"ca.crt":  []byte(CACert),
				},
			},
			wantErr: false,
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					AlpnProtocols: []string{"h2", "http/1.1"},
					TlsCertificates: []*envoytlsv3.TlsCertificate{{
						CertificateChain: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_InlineString{InlineString: CACert}},
						PrivateKey:       &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_InlineString{InlineString: TLSKey}},
					}},
					ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{
						ValidationContext: &envoytlsv3.CertificateValidationContext{
							TrustedCa: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_InlineString{InlineString: CACert}},
						},
					},
				},
				Sni:                "test.example.com",
				AllowRenegotiation: true,
			},
		},
		{
			name: "file-based TLS config",
			tlsConfig: &v1alpha1.TLS{
				TLSFiles: &v1alpha1.TLSFiles{
					TLSCertificate: ptr.To(CACert),
					TLSKey:         ptr.To(TLSKey),
					RootCA:         ptr.To(CACert),
				},
				Sni:                ptr.To("test.example.com"),
				AllowRenegotiation: ptr.To(true),
			},
			wantErr: false,
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					TlsCertificates: []*envoytlsv3.TlsCertificate{{
						CertificateChain: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: CACert}},
						PrivateKey:       &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: TLSKey}},
					}},
					ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{
						ValidationContext: &envoytlsv3.CertificateValidationContext{
							TrustedCa: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: CACert}},
						},
					},
				},
				Sni:                "test.example.com",
				AllowRenegotiation: true,
			},
		},
		{
			name: "TLS config with parameters",
			tlsConfig: &v1alpha1.TLS{
				TLSFiles: &v1alpha1.TLSFiles{
					TLSCertificate: ptr.To(CACert),
					TLSKey:         ptr.To(TLSKey),
				},
				Parameters: &v1alpha1.Parameters{
					TLSMinVersion: ptr.To(v1alpha1.TLSVersion1_2),
					TLSMaxVersion: ptr.To(v1alpha1.TLSVersion1_3),
					CipherSuites:  []string{"TLS_AES_128_GCM_SHA256"},
					EcdhCurves:    []string{"X25519"},
				},
				AllowRenegotiation: ptr.To(true),
			},
			wantErr: false,
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					TlsCertificates: []*envoytlsv3.TlsCertificate{{
						CertificateChain: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: CACert}},
						PrivateKey:       &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: TLSKey}},
					}},
					TlsParams: &envoytlsv3.TlsParameters{
						TlsMinimumProtocolVersion: envoytlsv3.TlsParameters_TLSv1_2,
						TlsMaximumProtocolVersion: envoytlsv3.TlsParameters_TLSv1_3,
						CipherSuites:              []string{"TLS_AES_128_GCM_SHA256"},
						EcdhCurves:                []string{"X25519"},
					},
				},
				AllowRenegotiation: true,
			},
		},
		{
			name: "invalid TLS config - missing secret",
			tlsConfig: &v1alpha1.TLS{
				SecretRef: &corev1.LocalObjectReference{
					Name: "non-existent-secret",
				},
				AllowRenegotiation: ptr.To(true),
			},
			wantErr: true,
		},
		{
			name: "should not error with only rootca",
			tlsConfig: &v1alpha1.TLS{
				TLSFiles: &v1alpha1.TLSFiles{
					RootCA: ptr.To(CACert),
				},
			},
			wantErr: false,
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{
						ValidationContext: &envoytlsv3.CertificateValidationContext{
							TrustedCa: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: CACert}},
						},
					},
				},
			},
		},
		{
			name: "should error with san and no rootca",
			tlsConfig: &v1alpha1.TLS{
				TLSFiles: &v1alpha1.TLSFiles{
					TLSCertificate: ptr.To(CACert),
					TLSKey:         ptr.To(TLSKey),
				},
				VerifySubjectAltName: []string{"test.example.com"},
			},
			wantErr: true,
		},
		{
			name: "should error with only cert and no key",
			tlsConfig: &v1alpha1.TLS{
				TLSFiles: &v1alpha1.TLSFiles{
					TLSCertificate: ptr.To(CACert),
				},
			},
			wantErr: true,
		},
		{
			name: "TLS config with only private key provided",
			tlsConfig: &v1alpha1.TLS{
				TLSFiles: &v1alpha1.TLSFiles{
					TLSKey: ptr.To(TLSKey),
				},
			},
			wantErr: true,
		},
		{
			name: "SimpleTLS with SAN verification and root CA",
			tlsConfig: &v1alpha1.TLS{
				TLSFiles: &v1alpha1.TLSFiles{
					TLSCertificate: ptr.To(CACert),
					TLSKey:         ptr.To(TLSKey),
					RootCA:         ptr.To(CACert),
				},
				SimpleTLS:            ptr.To(true),
				VerifySubjectAltName: []string{"test.example.com"},
			},
			wantErr: false,
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{
						ValidationContext: &envoytlsv3.CertificateValidationContext{
							TrustedCa: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: CACert}},
							MatchTypedSubjectAltNames: []*envoytlsv3.SubjectAltNameMatcher{{
								SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
								Matcher: &envoymatcher.StringMatcher{MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "test.example.com"}},
							}},
						},
					},
				},
			},
		},
		{
			name: "should only have validation context if simple tls",
			tlsConfig: &v1alpha1.TLS{
				TLSFiles: &v1alpha1.TLSFiles{
					TLSCertificate: ptr.To(CACert),
					TLSKey:         ptr.To(TLSKey),
					RootCA:         ptr.To(CACert),
				},
				SimpleTLS: ptr.To(true),
			},
			wantErr: false,
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{
						ValidationContext: &envoytlsv3.CertificateValidationContext{
							TrustedCa: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: CACert}},
						},
					},
				},
			},
		},
		{
			name: "should not have validation context if no rootca",
			tlsConfig: &v1alpha1.TLS{
				SecretRef: &corev1.LocalObjectReference{
					Name: "test-secret",
				},
			},
			secret: &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
					Namespace: "default",
					Name:      "test-secret",
				},
				Obj: &corev1.Secret{},
				Data: map[string][]byte{
					"tls.crt": []byte(CACert),
					"tls.key": []byte(TLSKey),
				},
			},
			wantErr: false,
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					TlsCertificates: []*envoytlsv3.TlsCertificate{{
						CertificateChain: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_InlineString{InlineString: CACert}},
						PrivateKey:       &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_InlineString{InlineString: TLSKey}},
					}},
				},
			},
		},
		{
			name: "TLS config with SAN verification",
			tlsConfig: &v1alpha1.TLS{
				TLSFiles: &v1alpha1.TLSFiles{
					TLSCertificate: ptr.To(CACert),
					TLSKey:         ptr.To(TLSKey),
					RootCA:         ptr.To(CACert),
				},
				VerifySubjectAltName: []string{"test.example.com", "api.example.com"},
				Sni:                  ptr.To("test.example.com"),
			},
			wantErr: false,
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					TlsCertificates: []*envoytlsv3.TlsCertificate{{
						CertificateChain: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: CACert}},
						PrivateKey:       &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: TLSKey}},
					}},
					ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{
						ValidationContext: &envoytlsv3.CertificateValidationContext{
							TrustedCa: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: CACert}},
							MatchTypedSubjectAltNames: []*envoytlsv3.SubjectAltNameMatcher{
								{SanType: envoytlsv3.SubjectAltNameMatcher_DNS, Matcher: &envoymatcher.StringMatcher{MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "test.example.com"}}},
								{SanType: envoytlsv3.SubjectAltNameMatcher_DNS, Matcher: &envoymatcher.StringMatcher{MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "api.example.com"}}},
							},
						},
					},
				},
				Sni: "test.example.com",
			},
		},
		{
			name: "TLS config with system ca",
			tlsConfig: &v1alpha1.TLS{
				WellKnownCACertificates: ptr.To(gwv1alpha3.WellKnownCACertificatesSystem),
			},
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					ValidationContextType: &envoytlsv3.CommonTlsContext_CombinedValidationContext{
						CombinedValidationContext: &envoytlsv3.CommonTlsContext_CombinedCertificateValidationContext{
							DefaultValidationContext:         &envoytlsv3.CertificateValidationContext{},
							ValidationContextSdsSecretConfig: &envoytlsv3.SdsSecretConfig{Name: eiutils.SystemCaSecretName},
						},
					},
				},
			},
		},
		{
			name: "TLS config with system ca and san",
			tlsConfig: &v1alpha1.TLS{
				WellKnownCACertificates: ptr.To(gwv1alpha3.WellKnownCACertificatesSystem),
				VerifySubjectAltName:    []string{"test.example.com", "api.example.com"},
			},
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					ValidationContextType: &envoytlsv3.CommonTlsContext_CombinedValidationContext{
						CombinedValidationContext: &envoytlsv3.CommonTlsContext_CombinedCertificateValidationContext{
							DefaultValidationContext: &envoytlsv3.CertificateValidationContext{
								MatchTypedSubjectAltNames: []*envoytlsv3.SubjectAltNameMatcher{
									{SanType: envoytlsv3.SubjectAltNameMatcher_DNS, Matcher: &envoymatcher.StringMatcher{MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "test.example.com"}}},
									{SanType: envoytlsv3.SubjectAltNameMatcher_DNS, Matcher: &envoymatcher.StringMatcher{MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "api.example.com"}}},
								},
							},
							ValidationContextSdsSecretConfig: &envoytlsv3.SdsSecretConfig{Name: eiutils.SystemCaSecretName},
						},
					},
				},
			},
		},
		{
			name: "TLS config with insecure skip verify",
			tlsConfig: &v1alpha1.TLS{
				InsecureSkipVerify: ptr.To(true),
				Sni:                ptr.To("test.example.com"),
			},
			wantErr: false,
			expected: &envoytlsv3.UpstreamTlsContext{
				CommonTlsContext: &envoytlsv3.CommonTlsContext{
					ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{},
				},
				Sni: "test.example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock secret getter
			secretGetter := NewMockSecretGetter()

			// Add secret to mock if provided
			if tt.secret != nil {
				secretGetter.AddSecret(tt.secret.Name, tt.secret.Namespace, tt.secret)
			}

			// Call the function
			result, err := translateTLSConfig(secretGetter, tt.tlsConfig, "default")

			// Check error
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			diff := cmp.Diff(tt.expected, result, protocmp.Transform())
			assert.Empty(t, diff)
		})
	}
}

func TestVerifySanListToTypedMatchSanList(t *testing.T) {
	tests := []struct {
		name     string
		sanList  []string
		expected []*envoytlsv3.SubjectAltNameMatcher
	}{
		{
			name:     "empty SAN list",
			sanList:  []string{},
			expected: []*envoytlsv3.SubjectAltNameMatcher{},
		},
		{
			name:    "single SAN",
			sanList: []string{"example.com"},
			expected: []*envoytlsv3.SubjectAltNameMatcher{
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "example.com"},
					},
				},
			},
		},
		{
			name:    "multiple SANs",
			sanList: []string{"example.com", "api.example.com", "www.example.com"},
			expected: []*envoytlsv3.SubjectAltNameMatcher{
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "example.com"},
					},
				},
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "api.example.com"},
					},
				},
				{
					SanType: envoytlsv3.SubjectAltNameMatcher_DNS,
					Matcher: &envoymatcher.StringMatcher{
						MatchPattern: &envoymatcher.StringMatcher_Exact{Exact: "www.example.com"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := verifySanListToTypedMatchSanList(tt.sanList)

			assert.Len(t, result, len(tt.expected))

			for i, expected := range tt.expected {
				assert.Equal(t, expected.SanType, result[i].SanType)
				assert.Equal(t, expected.Matcher.GetExact(), result[i].Matcher.GetExact())
			}
		})
	}
}
