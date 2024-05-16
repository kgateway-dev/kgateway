package backendref

import (
	"testing"

	"github.com/solo-io/gloo/pkg/utils"
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
				Kind:  utils.PointerTo(gwv1.Kind("Service")),
				Group: utils.PointerTo(gwv1.Group(corev1.GroupName)),
			},
			expected: true,
		},
		{
			name: "Invalid Kind",
			ref: gwv1.BackendObjectReference{
				Kind:  utils.PointerTo(gwv1.Kind("InvalidKind")),
				Group: utils.PointerTo(gwv1.Group(corev1.GroupName)),
			},
			expected: false,
		},
		{
			name: "Invalid Group",
			ref: gwv1.BackendObjectReference{
				Kind:  utils.PointerTo(gwv1.Kind("Service")),
				Group: utils.PointerTo(gwv1.Group("InvalidGroup")),
			},
			expected: false,
		},
		{
			name: "Invalid Group",
			ref: gwv1.BackendObjectReference{
				Group: utils.PointerTo(gwv1.Group(corev1.GroupName)),
			},
			expected: true, // Default Kind should pass
		},
		{
			name: "No Group",
			ref: gwv1.BackendObjectReference{
				Kind: utils.PointerTo(gwv1.Kind("Service")),
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
