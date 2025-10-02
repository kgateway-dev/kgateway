package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestParseAppProtocol(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected AppProtocol
	}{
		{
			name:     "http2",
			input:    ptr.To("http2"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "grpc",
			input:    ptr.To("grpc"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "grpc-web",
			input:    ptr.To("grpc-web"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "kubernetes.io/h2c",
			input:    ptr.To("kubernetes.io/h2c"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "kubernetes.io/ws",
			input:    ptr.To("kubernetes.io/ws"),
			expected: WebSocketAppProtocol,
		},
		{
			name:     "HTTP2",
			input:    ptr.To("HTTP2"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "(empty)",
			input:    nil,
			expected: DefaultAppProtocol,
		},
		{
			name:     "unknown",
			input:    ptr.To("unknown"),
			expected: DefaultAppProtocol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)
			actual := ParseAppProtocol(tt.input)
			a.Equal(tt.expected, actual)
		})
	}
}
