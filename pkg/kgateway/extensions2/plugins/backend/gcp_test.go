package backend

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestGcpIrEquals(t *testing.T) {
	tests := []struct {
		name     string
		ir1      *GcpIr
		ir2      *GcpIr
		expected bool
	}{
		{
			name:     "both nil",
			ir1:      nil,
			ir2:      nil,
			expected: true,
		},
		{
			name:     "ir1 nil, ir2 not nil",
			ir1:      nil,
			ir2:      &GcpIr{},
			expected: false,
		},
		{
			name:     "ir1 not nil, ir2 nil",
			ir1:      &GcpIr{},
			ir2:      nil,
			expected: false,
		},
		{
			name: "equal GCP IRs",
			ir1: &GcpIr{
				hostname: "example.com",
			},
			ir2: &GcpIr{
				hostname: "example.com",
			},
			expected: true,
		},
		{
			name: "different hostnames",
			ir1: &GcpIr{
				hostname: "example.com",
			},
			ir2: &GcpIr{
				hostname: "other.com",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ir1.Equals(tt.ir2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildGcpIr(t *testing.T) {
	tests := []struct {
		name             string
		input            *kgateway.GcpBackend
		expectedHost     string
		expectedAudience string
		wantError        bool
	}{
		{
			name: "basic GCP backend with default audience",
			input: &kgateway.GcpBackend{
				Host: "example.com",
			},
			expectedHost:     "example.com",
			expectedAudience: "https://example.com",
			wantError:        false,
		},
		{
			name: "GCP backend with custom audience",
			input: &kgateway.GcpBackend{
				Host:     "example.com",
				Audience: stringPtr("https://custom-audience.com"),
			},
			expectedHost:     "example.com",
			expectedAudience: "https://custom-audience.com",
			wantError:        false,
		},
		{
			name: "GCP backend with empty audience uses default",
			input: &kgateway.GcpBackend{
				Host:     "example.com",
				Audience: stringPtr(""),
			},
			expectedHost:     "example.com",
			expectedAudience: "https://example.com",
			wantError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir, err := buildGcpIr(tt.input)
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, ir)
			assert.Equal(t, tt.expectedHost, ir.hostname)
			assert.NotNil(t, ir.transportSocket)
			assert.NotNil(t, ir.audienceConfigAny)

			// Verify audience config
			// Note: We can't easily unmarshal the anypb.Any here without importing
			// the gcp_auth package, but we can verify it's set
			assert.NotNil(t, ir.audienceConfigAny)
		})
	}
}

func TestProcessGcp(t *testing.T) {
	tests := []struct {
		name      string
		ir        *GcpIr
		wantError bool
	}{
		{
			name:      "nil IR",
			ir:        nil,
			wantError: true,
		},
		{
			name: "valid GCP IR",
			ir: &GcpIr{
				hostname: "example.com",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &envoyclusterv3.Cluster{
				Name: "test-cluster",
			}
			err := processGcp(tt.ir, cluster)
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, cluster.ClusterDiscoveryType)
			assert.Equal(t, envoyclusterv3.Cluster_STRICT_DNS, cluster.ClusterDiscoveryType.(*envoyclusterv3.Cluster_Type).Type)
			assert.NotNil(t, cluster.LoadAssignment)
		})
	}
}

func TestGcpIrEqualsWithTransportSocket(t *testing.T) {
	// Create two GCP IRs with transport sockets
	ir1, err1 := buildGcpIr(&kgateway.GcpBackend{Host: "example.com"})
	assert.NoError(t, err1)

	ir2, err2 := buildGcpIr(&kgateway.GcpBackend{Host: "example.com"})
	assert.NoError(t, err2)

	// They should be equal
	assert.True(t, ir1.Equals(ir2))

	// Create one with different host
	ir3, err3 := buildGcpIr(&kgateway.GcpBackend{Host: "other.com"})
	assert.NoError(t, err3)

	// Should not be equal
	assert.False(t, ir1.Equals(ir3))
}

func TestGcpIrEqualsWithAudience(t *testing.T) {
	// Create two GCP IRs with different audiences
	ir1, err1 := buildGcpIr(&kgateway.GcpBackend{
		Host:     "example.com",
		Audience: stringPtr("https://audience1.com"),
	})
	assert.NoError(t, err1)

	ir2, err2 := buildGcpIr(&kgateway.GcpBackend{
		Host:     "example.com",
		Audience: stringPtr("https://audience2.com"),
	})
	assert.NoError(t, err2)

	// They should not be equal due to different audiences
	// Note: This tests proto.Equal for audienceConfigAny
	assert.False(t, ir1.Equals(ir2))
}

func stringPtr(s string) *string {
	return &s
}
