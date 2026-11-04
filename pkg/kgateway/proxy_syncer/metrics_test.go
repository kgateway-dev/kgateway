package proxy_syncer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
)

func setupTest() {
	ResetMetrics()
	kmetrics.ResetMetrics()
}

func TestGetDetailsFromXDSClientResourceName(t *testing.T) {
	testCases := []struct {
		name     string
		resource string
		expected struct {
			role      string
			gateway   string
			namespace string
		}
	}{
		{
			name:     "Valid resource name",
			resource: "kgateway-kube-gateway-api~ns~test",
			expected: struct {
				role      string
				gateway   string
				namespace string
			}{
				role:      "kgateway-kube-gateway-api",
				gateway:   "test",
				namespace: "ns",
			},
		},
		{
			name:     "Invalid resource name",
			resource: "invalid-resource-name",
			expected: struct {
				role      string
				gateway   string
				namespace string
			}{
				role:      "unknown",
				gateway:   "unknown",
				namespace: "unknown",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cd := getDetailsFromXDSClientResourceName(tc.resource)
			assert.Equal(t, tc.expected.gateway, cd.Gateway)
			assert.Equal(t, tc.expected.namespace, cd.Namespace)
		})
	}
}
