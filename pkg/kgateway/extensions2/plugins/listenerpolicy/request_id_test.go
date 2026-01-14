package listenerpolicy

import (
	"testing"

	envoyuuidv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/request_id/uuid/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

// TestUuidRequestIdConfigConversion tests the conversion logic from kgateway API
// UuidRequestIdConfig to Envoy's UuidRequestIdConfig proto. This ensures that
// the RequestID configuration feature correctly translates user settings into
// Envoy-compatible configuration.
func TestUuidRequestIdConfigConversion(t *testing.T) {
	tests := []struct {
		name     string
		config   *kgateway.UuidRequestIdConfig
		expected *envoyuuidv3.UuidRequestIdConfig
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: nil,
		},
		{
			name:   "empty config uses defaults",
			config: &kgateway.UuidRequestIdConfig{},
			expected: &envoyuuidv3.UuidRequestIdConfig{
				PackTraceReason:              wrapperspb.Bool(true),
				UseRequestIdForTraceSampling: wrapperspb.Bool(true),
			},
		},
		{
			name: "explicit false values",
			config: &kgateway.UuidRequestIdConfig{
				PackTraceReason:              ptr.To(false),
				UseRequestIDForTraceSampling: ptr.To(false),
			},
			expected: &envoyuuidv3.UuidRequestIdConfig{
				PackTraceReason:              wrapperspb.Bool(false),
				UseRequestIdForTraceSampling: wrapperspb.Bool(false),
			},
		},
		{
			name: "mixed values",
			config: &kgateway.UuidRequestIdConfig{
				PackTraceReason:              ptr.To(true),
				UseRequestIDForTraceSampling: ptr.To(false),
			},
			expected: &envoyuuidv3.UuidRequestIdConfig{
				PackTraceReason:              wrapperspb.Bool(true),
				UseRequestIdForTraceSampling: wrapperspb.Bool(false),
			},
		},
		{
			name: "partial config with defaults",
			config: &kgateway.UuidRequestIdConfig{
				PackTraceReason: ptr.To(false),
				// UseRequestIDForTraceSampling should default to true
			},
			expected: &envoyuuidv3.UuidRequestIdConfig{
				PackTraceReason:              wrapperspb.Bool(false),
				UseRequestIdForTraceSampling: wrapperspb.Bool(true),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result *envoyuuidv3.UuidRequestIdConfig
			if tt.config != nil {
				result = &envoyuuidv3.UuidRequestIdConfig{
					PackTraceReason:              wrapperspb.Bool(ptr.Deref(tt.config.PackTraceReason, true)),
					UseRequestIdForTraceSampling: wrapperspb.Bool(ptr.Deref(tt.config.UseRequestIDForTraceSampling, true)),
				}
			}

			if tt.expected == nil {
				require.Nil(t, result, "expected nil uuidRequestIdConfig")
			} else {
				require.NotNil(t, result, "expected non-nil uuidRequestIdConfig")
				require.Equal(t, tt.expected.PackTraceReason, result.PackTraceReason, "PackTraceReason should match")
				require.Equal(t, tt.expected.UseRequestIdForTraceSampling, result.UseRequestIdForTraceSampling, "UseRequestIdForTraceSampling should match")
			}
		})
	}
}

// TestHttpListenerPolicyIrEqualsRequestID tests the Equals() method of HttpListenerPolicyIr
// specifically for RequestID configuration comparisons. This is critical for KRT (Kubernetes
// Resource Translator) to properly detect configuration changes and trigger appropriate
// updates to the Envoy configuration.
func TestHttpListenerPolicyIrEqualsRequestID(t *testing.T) {
	tests := []struct {
		name     string
		ir1      *HttpListenerPolicyIr
		ir2      *HttpListenerPolicyIr
		expected bool
	}{
		{
			name:     "both nil",
			ir1:      &HttpListenerPolicyIr{},
			ir2:      &HttpListenerPolicyIr{},
			expected: true,
		},
		{
			name: "one nil",
			ir1: &HttpListenerPolicyIr{
				uuidRequestIdConfig: &envoyuuidv3.UuidRequestIdConfig{},
			},
			ir2:      &HttpListenerPolicyIr{},
			expected: false,
		},
		{
			name: "both same",
			ir1: &HttpListenerPolicyIr{
				uuidRequestIdConfig: &envoyuuidv3.UuidRequestIdConfig{
					PackTraceReason:              wrapperspb.Bool(true),
					UseRequestIdForTraceSampling: wrapperspb.Bool(true),
				},
			},
			ir2: &HttpListenerPolicyIr{
				uuidRequestIdConfig: &envoyuuidv3.UuidRequestIdConfig{
					PackTraceReason:              wrapperspb.Bool(true),
					UseRequestIdForTraceSampling: wrapperspb.Bool(true),
				},
			},
			expected: true,
		},
		{
			name: "different packTraceReason",
			ir1: &HttpListenerPolicyIr{
				uuidRequestIdConfig: &envoyuuidv3.UuidRequestIdConfig{
					PackTraceReason:              wrapperspb.Bool(true),
					UseRequestIdForTraceSampling: wrapperspb.Bool(true),
				},
			},
			ir2: &HttpListenerPolicyIr{
				uuidRequestIdConfig: &envoyuuidv3.UuidRequestIdConfig{
					PackTraceReason:              wrapperspb.Bool(false),
					UseRequestIdForTraceSampling: wrapperspb.Bool(true),
				},
			},
			expected: false,
		},
		{
			name: "different useRequestIdForTraceSampling",
			ir1: &HttpListenerPolicyIr{
				uuidRequestIdConfig: &envoyuuidv3.UuidRequestIdConfig{
					PackTraceReason:              wrapperspb.Bool(true),
					UseRequestIdForTraceSampling: wrapperspb.Bool(true),
				},
			},
			ir2: &HttpListenerPolicyIr{
				uuidRequestIdConfig: &envoyuuidv3.UuidRequestIdConfig{
					PackTraceReason:              wrapperspb.Bool(true),
					UseRequestIdForTraceSampling: wrapperspb.Bool(false),
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ir1.Equals(tt.ir2)
			require.Equal(t, tt.expected, result, "Equals() result should match expected")
		})
	}
}
