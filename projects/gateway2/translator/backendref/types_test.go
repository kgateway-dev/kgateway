package backendref

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestRefIsService(t *testing.T) {
	tests := []struct {
		name     string
		ref      gwv1.BackendObjectReference
		expected bool
	}{
		{
			name: "Valid Service Reference",
			ref: gwv1.BackendObjectReference{
				Kind:  ptrTo(gwv1.Kind("Service")),
				Group: ptrTo(gwv1.Group(corev1.GroupName)),
			},
			expected: true,
		},
		{
			name: "Invalid Kind",
			ref: gwv1.BackendObjectReference{
				Kind:  ptrTo(gwv1.Kind("InvalidKind")),
				Group: ptrTo(gwv1.Group(corev1.GroupName)),
			},
			expected: false,
		},
		{
			name: "Invalid Group",
			ref: gwv1.BackendObjectReference{
				Kind:  ptrTo(gwv1.Kind("Service")),
				Group: ptrTo(gwv1.Group("InvalidGroup")),
			},
			expected: false,
		},
		{
			name: "Invalid Group",
			ref: gwv1.BackendObjectReference{
				Group: ptrTo(gwv1.Group(corev1.GroupName)),
			},
			expected: true, // Default Kind should pass
		},
		{
			name: "No Group",
			ref: gwv1.BackendObjectReference{
				Kind: ptrTo(gwv1.Kind("Service")),
			},
			expected: true, // Default Group should pass
		},
		{
			name:     "No Kind and Group",
			ref:      gwv1.BackendObjectReference{},
			expected: true, // Defaults should pass
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := RefIsService(test.ref)
			if result != test.expected {
				t.Errorf("Test case %q failed: expected %t but got %t", test.name, test.expected, result)
			}
		})
	}
}

// gateway apis uses this to build test examples: https://github.com/kubernetes-sigs/gateway-api/blob/main/pkg/test/cel/main_test.go#L57
func ptrTo[T any](a T) *T {
	return &a
}
