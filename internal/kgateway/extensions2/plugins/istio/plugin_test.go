package istio

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TestIsDisabledForUpstream(t *testing.T) {
	tests := []struct {
		name     string
		backend  ir.BackendObjectIR
		expected bool
	}{
		{
			name: "no annotations - should not be disabled",
			backend: ir.BackendObjectIR{
				Obj: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
			},
			expected: false,
		},
		{
			name: "disable auto-mtls annotation true - should be disabled",
			backend: ir.BackendObjectIR{
				Obj: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							DisableIstioAutoMtlsAnnotation: "true",
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "disable auto-mtls annotation false - should not be disabled",
			backend: ir.BackendObjectIR{
				Obj: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							DisableIstioAutoMtlsAnnotation: "false",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "nil obj - should not be disabled",
			backend: ir.BackendObjectIR{
				Obj: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDisabledForUpstream(tt.backend)
			if result != tt.expected {
				t.Errorf("isDisabledForUpstream() = %v, want %v", result, tt.expected)
			}
		})
	}
}
