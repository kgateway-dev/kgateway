package backendref

import (
	"testing"

	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestRefIsHTTPRoute(t *testing.T) {
	tests := []struct {
		name     string
		ref      gwv1.BackendObjectReference
		expected bool
	}{
		{
			name: "Valid RefIsHTTPRoute Reference",
			ref: gwv1.BackendObjectReference{
				Kind:  ptr.To(gwv1.Kind("HTTPRoute")),
				Group: ptr.To(gwv1.Group(gwv1.GroupName)),
			},
			expected: true,
		},
		{
			name: "Invalid Kind",
			ref: gwv1.BackendObjectReference{
				Kind:  ptr.To(gwv1.Kind("InvalidKind")),
				Group: ptr.To(gwv1.Group(gwv1.GroupName)),
			},
			expected: false,
		},
		{
			name: "Invalid Group",
			ref: gwv1.BackendObjectReference{
				Kind:  ptr.To(gwv1.Kind("HTTPRoute")),
				Group: ptr.To(gwv1.Group("InvalidGroup")),
			},
			expected: false,
		},
		{
			name: "Invalid Group",
			ref: gwv1.BackendObjectReference{
				Group: ptr.To(gwv1.Group(gwv1.GroupName)),
			},
			expected: false, // Default Kind should not pass
		},
		{
			name: "No Group",
			ref: gwv1.BackendObjectReference{
				Kind: ptr.To(gwv1.Kind("HTTPRoute")),
			},
			expected: false, // Default Group should not pass
		},
		{
			name:     "No Kind and Group",
			ref:      gwv1.BackendObjectReference{},
			expected: false, // Defaults should not pass
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := RefIsHTTPRoute(test.ref)
			if result != test.expected {
				t.Errorf("Test case %q failed: expected %t but got %t", test.name, test.expected, result)
			}
		})
	}
}
