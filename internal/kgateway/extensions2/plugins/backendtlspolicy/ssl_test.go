package backendtlspolicy

import (
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var CA_CERT = `-----BEGIN CERTIFICATE-----
MIIC1jCCAb4CCQCJczLyBBZ1GTANBgkqhkiG9w0BAQsFADAtMRUwEwYDVQQKDAxl
eGFtcGxlIEluYy4xFDASBgNVBAMMC2V4YW1wbGUuY29tMB4XDTI1MDMwNzE0Mjkx
NloXDTI2MDMwNzE0MjkxNlowLTEVMBMGA1UECgwMZXhhbXBsZSBJbmMuMRQwEgYD
VQQDDAtleGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
AN0U6TVYECkwqnxh1Kt3dS+LialrXBOXKagj9tE582T6dwmqThD75VZPrNKkRoYO
aUzCctfDkUBXRemOTMut7ES5xoAtSAhr2GAnqgM3+yBCLOxooSjEFdlpFT7dhi1w
jOPa5iMh6ve/pHuRHvEuaF/J6P8tr83wGutx/xFZVuGA9V1AmBmYhePM+JhdcwaB
1+IbJp30gGyPfY4vdRQ9VQWbThE8psEzah+3SgTKJSIT7NAdwiIu3O3rXORbaYYU
oycgXUHdOKRbJnbvy3pTnFZJ50sg1HIA4yBdX7c0diy8Zz3Suoondg3DforWr0pB
Hs6tySAQoz2RiAqDqcE2rbMCAwEAATANBgkqhkiG9w0BAQsFAAOCAQEAWPkz3dJW
b+LFtnv7MlOVM79Y4PqeiHnazP1G9FwnWBHARkjISsax3b0zX8/RHnU83c3tLP5D
VwenYb9B9mzXbLiWI8aaX0UXP//D593ti15y0Od7yC2hQszlqIbxYnkFVwXoT9fQ
bdQ9OtpCt8EZnKEyCxck+hlKEyYTcH2PqZ7Ndp0M8I2znz3Kut/uYHLUddfoPF/m
O0V6fbyB/Mx/G1uLiv/BVpx3AdP+3ygJyKtelXkD+IdlY3y110fzmVr6NgxAbz/h
n9KpuK4SEloIycZUaKVXAaX7T42SFYw7msmB+Uu7z5oLOijsjX6TjeofdFBZ/Byl
SxODgqhtaPnOxQ==
-----END CERTIFICATE-----`

func TestUpstreamTlsConfig(t *testing.T) {
	tests := []struct {
		name          string
		cm            *corev1.ConfigMap
		sni           string
		expectedError string
		expectedTls   *envoyauth.UpstreamTlsContext
	}{
		{
			name: "Basic config",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-ca",
					Namespace: "default",
				},
				Data: map[string]string{
					"ca.crt": CA_CERT,
				},
			},
			sni:           "example.com",
			expectedError: "",
			expectedTls: &envoyauth.UpstreamTlsContext{
				CommonTlsContext: &envoyauth.CommonTlsContext{
					ValidationContextType: &envoyauth.CommonTlsContext_ValidationContext{
						ValidationContext: &envoyauth.CertificateValidationContext{
							TrustedCa: &corev3.DataSource{
								Specifier: &corev3.DataSource_InlineString{
									InlineString: CA_CERT,
								},
							},
						},
					},
				},
				Sni: "example.com",
			},
		},
		{
			name: "Missing ca.crt in configmap",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-ca",
					Namespace: "default",
				},
				Data: map[string]string{},
			},
			sni:           "example.com",
			expectedError: noKeyFoundMsg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation := &envoyauth.CertificateValidationContext{}
			tlsCtx, err := ResolveUpstreamSslConfig(tt.cm, validation, tt.sni)
			if tt.expectedError != "" && err == nil {
				t.Fatalf("expected error but got nil")
			} else if tt.expectedError != "" && err != nil {
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Fatalf("expected error %v but got %v", tt.expectedError, err)
				}
			} else if tt.expectedError == "" && err != nil {
				t.Fatalf("expected no error but got %v", err)
			}

			if tt.expectedTls == nil && tlsCtx == nil {
				// no tls expected and found, exit early
				return
			}

			if tt.expectedTls != nil && tlsCtx == nil {
				t.Fatal("expected tls config but found none")
			}
			if tlsCtx.Sni != string(tt.expectedTls.Sni) {
				t.Errorf("expected sni '%v' but got '%v'", tt.expectedTls.Sni, tlsCtx.Sni)
			}
			caStr := tlsCtx.GetCommonTlsContext().GetValidationContext().GetTrustedCa().GetInlineString()
			expectedCaStr := tt.expectedTls.GetCommonTlsContext().GetValidationContext().GetTrustedCa().GetInlineString()
			if caStr != expectedCaStr {
				t.Error("expected CA:", expectedCaStr, "found CA:", caStr)
			}
		})
	}
}
