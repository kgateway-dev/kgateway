package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
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

func createTestBackendObjectIR(trafficDist wellknown.TrafficDistribution) BackendObjectIR {
	return BackendObjectIR{
		ObjectSource: ObjectSource{
			Namespace: "default",
			Name:      "test-service",
			Group:     "",
			Kind:      "Service",
		},
		Port: 8080,
		Obj: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-service",
				Namespace:       "default",
				UID:             "test-uid",
				ResourceVersion: "1",
				Generation:      1,
			},
		},
		TrafficDistribution: trafficDist,
	}
}

func TestBackendObjectIREquals(t *testing.T) {
	tests := []struct {
		name     string
		backend1 func() BackendObjectIR
		backend2 func() BackendObjectIR
		want     bool
	}{
		{
			name:     "same backend objects should be equal",
			backend1: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionAny) },
			backend2: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionAny) },
			want:     true,
		},
		{
			name:     "backends with different traffic distribution should not be equal",
			backend1: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionAny) },
			backend2: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferSameZone) },
			want:     false,
		},
		{
			name:     "backends with different traffic distribution PreferSameZone vs PreferNetwork should not be equal",
			backend1: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferSameZone) },
			backend2: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferNetwork) },
			want:     false,
		},
		{
			name:     "backends with different traffic distribution PreferSameNode vs PreferNetwork should not be equal",
			backend1: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferSameNode) },
			backend2: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferNetwork) },
			want:     false,
		},
		{
			name:     "backends with same PreferNetwork traffic distribution should be equal",
			backend1: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferNetwork) },
			backend2: func() BackendObjectIR { return createTestBackendObjectIR(wellknown.TrafficDistributionPreferNetwork) },
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)
			backend1 := tt.backend1()
			backend2 := tt.backend2()

			// Test forward equality
			result := backend1.Equals(backend2)
			a.Equal(tt.want, result, "BackendObjectIR.Equals() result mismatch")

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := backend2.Equals(backend1)
			a.Equal(result, reverseResult, "symmetry check failed: a.Equals(b) != b.Equals(a)")

			// Test reflexivity: x.Equals(x) should always be true
			a.True(backend1.Equals(backend1), "reflexivity check failed for backend1")
			a.True(backend2.Equals(backend2), "reflexivity check failed for backend2")
		})
	}
}
