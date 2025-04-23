package backendref

import (
	"testing"

	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      gwv1.BackendObjectReference
		refFn    func(ref gwv1.BackendObjectReference) bool
		expected bool
	}{
		{
			name: "Valid IsDelegatedHTTPRoute Reference",
			ref: gwv1.BackendObjectReference{
				Kind:  ptr.To(gwv1.Kind("HTTPRoute")),
				Group: ptr.To(gwv1.Group(gwv1.GroupName)),
			},
			refFn:    IsDelegatedHTTPRoute,
			expected: true,
		},
		{
			name: "Invalid Kind",
			ref: gwv1.BackendObjectReference{
				Kind:  ptr.To(gwv1.Kind("InvalidKind")),
				Group: ptr.To(gwv1.Group(gwv1.GroupName)),
			},
			refFn:    IsDelegatedHTTPRoute,
			expected: false,
		},
		{
			name: "Invalid Group",
			ref: gwv1.BackendObjectReference{
				Kind:  ptr.To(gwv1.Kind("HTTPRoute")),
				Group: ptr.To(gwv1.Group("InvalidGroup")),
			},
			refFn:    IsDelegatedHTTPRoute,
			expected: false,
		},
		{
			name: "Invalid Group",
			ref: gwv1.BackendObjectReference{
				Group: ptr.To(gwv1.Group(gwv1.GroupName)),
			},
			refFn:    IsDelegatedHTTPRoute,
			expected: false, // Default Kind should not pass
		},
		{
			name: "No Group",
			ref: gwv1.BackendObjectReference{
				Kind: ptr.To(gwv1.Kind("HTTPRoute")),
			},
			refFn:    IsDelegatedHTTPRoute,
			expected: false, // Default Group should not pass
		},
		{
			name:     "No Kind and Group",
			ref:      gwv1.BackendObjectReference{},
			refFn:    IsDelegatedHTTPRoute,
			expected: false, // Defaults should not pass
		},
		{
			name: "Delegation label selector",
			ref: gwv1.BackendObjectReference{
				Kind:  ptr.To(gwv1.Kind("label")),
				Group: ptr.To(gwv1.Group("delegation.kgateway.dev")),
			},
			refFn:    IsDelegatedHTTPRoute,
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.refFn(test.ref)
			if result != test.expected {
				t.Errorf("Test case %q failed: expected %t but got %t", test.name, test.expected, result)
			}
		})
	}
}
