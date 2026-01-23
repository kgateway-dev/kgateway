package backend

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	gcp_auth "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/gcp_authn/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

// buildTestGcpIr is a test helper that builds a GcpIr and panics on error.
func buildTestGcpIr(backend *kgateway.GcpBackend) *GcpIr {
	ir, err := buildGcpIr(backend)
	if err != nil {
		panic(err)
	}
	return ir
}

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
			name:     "equal hostnames",
			ir1:      buildTestGcpIr(&kgateway.GcpBackend{Host: "example.com"}),
			ir2:      buildTestGcpIr(&kgateway.GcpBackend{Host: "example.com"}),
			expected: true,
		},
		{
			name:     "different hostnames",
			ir1:      buildTestGcpIr(&kgateway.GcpBackend{Host: "example.com"}),
			ir2:      buildTestGcpIr(&kgateway.GcpBackend{Host: "other.com"}),
			expected: false,
		},
		{
			name:     "different audiences",
			ir1:      buildTestGcpIr(&kgateway.GcpBackend{Host: "example.com", Audience: stringPtr("https://audience1.com")}),
			ir2:      buildTestGcpIr(&kgateway.GcpBackend{Host: "example.com", Audience: stringPtr("https://audience2.com")}),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ir1.Equals(tt.ir2)
			assert.Equal(t, tt.expected, result, "Equals() should return %v", tt.expected)
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
			assert.Equal(t, tt.expectedHost, ir.hostname, "hostname should match")

			// Verify transport socket
			assert.NotNil(t, ir.transportSocket, "transport socket should be set")
			assert.Equal(t, "envoy.transport_sockets.tls", ir.transportSocket.Name)
			assert.NotNil(t, ir.transportSocket.GetTypedConfig())
			var tlsContext envoytlsv3.UpstreamTlsContext
			err = anypb.UnmarshalTo(ir.transportSocket.GetTypedConfig(), &tlsContext, proto.UnmarshalOptions{})
			assert.NoError(t, err, "should be able to unmarshal TLS context")
			assert.Equal(t, tt.expectedHost, tlsContext.Sni, "SNI should match hostname")

			// Verify audience config
			assert.NotNil(t, ir.audienceConfigAny, "audience config should be set")
			var audienceConfig gcp_auth.Audience
			err = anypb.UnmarshalTo(ir.audienceConfigAny, &audienceConfig, proto.UnmarshalOptions{})
			assert.NoError(t, err, "should be able to unmarshal audience config")
			assert.Equal(t, tt.expectedAudience, audienceConfig.Url, "audience URL should match")
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
			name:      "valid GCP IR",
			ir:        buildTestGcpIr(&kgateway.GcpBackend{Host: "test.example.com"}),
			wantError: false,
		},
		{
			name:      "valid GCP IR with audience",
			ir:        buildTestGcpIr(&kgateway.GcpBackend{Host: "test.example.com", Audience: stringPtr("https://custom-audience.com")}),
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

			// Verify cluster discovery type
			assert.NotNil(t, cluster.ClusterDiscoveryType)
			clusterType, ok := cluster.ClusterDiscoveryType.(*envoyclusterv3.Cluster_Type)
			assert.True(t, ok, "cluster discovery type should be Cluster_Type")
			assert.Equal(t, envoyclusterv3.Cluster_STRICT_DNS, clusterType.Type)

			// Verify load assignment
			assert.NotNil(t, cluster.LoadAssignment)
			assert.Len(t, cluster.LoadAssignment.Endpoints, 1)
			assert.Len(t, cluster.LoadAssignment.Endpoints[0].LbEndpoints, 1)
			endpoint := cluster.LoadAssignment.Endpoints[0].LbEndpoints[0].GetEndpoint()
			assert.NotNil(t, endpoint)
			socketAddr := endpoint.Address.GetSocketAddress()
			assert.NotNil(t, socketAddr)
			assert.Equal(t, tt.ir.hostname, socketAddr.Address)
			assert.Equal(t, gcpBackendPort, socketAddr.GetPortValue())

			assert.NotNil(t, cluster.TransportSocket, "transport socket should be set")
			assert.Equal(t, "envoy.transport_sockets.tls", cluster.TransportSocket.Name)

			assert.NotNil(t, cluster.Metadata)
			assert.NotNil(t, cluster.Metadata.TypedFilterMetadata)
			audienceAny, ok := cluster.Metadata.TypedFilterMetadata[gcpAuthnFilterName]
			assert.True(t, ok, "audience config should be in metadata")
			assert.NotNil(t, audienceAny)
			var audienceConfig gcp_auth.Audience
			err = anypb.UnmarshalTo(audienceAny, &audienceConfig, proto.UnmarshalOptions{})
			assert.NoError(t, err)
			// Verify the audience URL matches what was in the IR
			var expectedAudience gcp_auth.Audience
			err = anypb.UnmarshalTo(tt.ir.audienceConfigAny, &expectedAudience, proto.UnmarshalOptions{})
			assert.NoError(t, err)
			assert.Equal(t, expectedAudience.Url, audienceConfig.Url)
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
