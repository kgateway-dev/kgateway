package trafficpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func TestBufferForSpec(t *testing.T) {
	tests := []struct {
		name     string
		spec     v1alpha1.TrafficPolicySpec
		expected *BufferIR
	}{
		{
			name:     "nil buffer spec",
			spec:     v1alpha1.TrafficPolicySpec{},
			expected: nil,
		},
		{
			name: "valid buffer spec",
			spec: v1alpha1.TrafficPolicySpec{
				Buffer: &v1alpha1.Buffer{
					MaxRequestSize: ptr.To(resource.MustParse("1Ki")),
				},
			},
			expected: &BufferIR{
				maxRequestBytes: 1024,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &trafficPolicySpecIr{}
			bufferForSpec(tt.spec, out)

			if tt.expected == nil {
				assert.Nil(t, out.buffer)
			} else {
				assert.NotNil(t, out.buffer)
				assert.Equal(t, tt.expected.maxRequestBytes, out.buffer.maxRequestBytes)
			}
		})
	}
}

func TestBufferIREquals(t *testing.T) {
	tests := []struct {
		name     string
		b1       *BufferIR
		b2       *BufferIR
		expected bool
	}{
		{
			name:     "both nil",
			b1:       nil,
			b2:       nil,
			expected: true,
		},
		{
			name: "one nil",
			b1:   nil,
			b2: &BufferIR{
				maxRequestBytes: 1024,
			},
			expected: false,
		},
		{
			name: "equal buffers",
			b1: &BufferIR{
				maxRequestBytes: 1024,
			},
			b2: &BufferIR{
				maxRequestBytes: 1024,
			},
			expected: true,
		},
		{
			name: "different max request bytes",
			b1: &BufferIR{
				maxRequestBytes: 1024,
			},
			b2: &BufferIR{
				maxRequestBytes: 2048,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.b1.Equals(tt.b2)
			assert.Equal(t, tt.expected, result)
		})
	}
}
