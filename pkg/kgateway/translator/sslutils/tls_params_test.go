package sslutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateTlsStrings(t *testing.T) {
	testCases := []struct {
		name      string
		input     []string
		supported map[string]struct{}
		want      []string
		wantErr   bool
	}{
		{
			name:      "empty input is valid",
			input:     nil,
			supported: SupportedEcdhCurves,
			want:      nil,
			wantErr:   false,
		},
		{
			name:      "valid curves",
			input:     []string{"X25519", "P-256"},
			supported: SupportedEcdhCurves,
			want:      []string{"X25519", "P-256"},
			wantErr:   false,
		},
		{
			name:      "valid cipher suites in both naming conventions",
			input:     []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", "ECDHE-RSA-AES128-GCM-SHA256"},
			supported: SupportedCipherSuites,
			want:      []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", "ECDHE-RSA-AES128-GCM-SHA256"},
			wantErr:   false,
		},
		{
			name:      "unsupported value",
			input:     []string{"hello"},
			supported: SupportedEcdhCurves,
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "unsupported cipher suite",
			input:     []string{"BOGUS_CIPHER_SUITE_1"},
			supported: SupportedCipherSuites,
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "duplicates are deduplicated",
			input:     []string{"P-256", "P-256", "X25519"},
			supported: SupportedEcdhCurves,
			want:      []string{"P-256", "X25519"},
			wantErr:   false,
		},
		{
			name:      "values are trimmed",
			input:     []string{"  X25519  ", "P-256"},
			supported: SupportedEcdhCurves,
			want:      []string{"X25519", "P-256"},
			wantErr:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateTlsStrings(tc.input, tc.supported, "test")
			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}
