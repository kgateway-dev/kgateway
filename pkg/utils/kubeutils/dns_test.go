package kubeutils

import (
	"testing"
)

func TestSafeGatewayLabelValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		want    string
	}{
		{
			name:    "short name returns as-is",
			input:   "my-gateway",
			wantLen: 10,
			want:    "my-gateway",
		},
		{
			name:    "long name returns truncated with hash",
			input:   "this-is-a-very-long-gateway-name-that-exceeds-sixty-three-characters-long",
			wantLen: 63,
			want:    "this-is-a-very-long-gateway-name-that-exceeds-sixt-42d6992a3ffd",
		},
		{
			name:    "long name with trailing dash at truncation boundary",
			input:   "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqr-extra-chars-here-12345",
			wantLen: 63,
			want:    "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqr-extra-43c57a89f7ba",
		},
		{
			name:    "exact 63 char name returns as-is",
			input:   "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz12345678901",
			wantLen: 63,
			want:    "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz12345678901",
		},
		{
			name:    "exact 64 char name returns truncated with hash",
			input:   "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz1234567890123",
			wantLen: 63,
			want:    "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwx-fa41f1dd8585",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeGatewayLabelValue(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("SafeGatewayLabelValue(%q) len = %d, want %d", tt.input, len(got), tt.wantLen)
			}
			if got != tt.want {
				t.Errorf("SafeGatewayLabelValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
