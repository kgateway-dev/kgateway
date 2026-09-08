package listenerpolicy

import (
	"testing"

	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestHttpListenerPolicyIrEqualsStripTrailingHostDot(t *testing.T) {
	tests := []struct {
		name     string
		ir1      *HttpListenerPolicyIr
		ir2      *HttpListenerPolicyIr
		expected bool
	}{
		{
			name:     "both unset",
			ir1:      &HttpListenerPolicyIr{},
			ir2:      &HttpListenerPolicyIr{},
			expected: true,
		},
		{
			name: "one set one unset",
			ir1: &HttpListenerPolicyIr{
				stripTrailingHostDot: new(true),
			},
			ir2:      &HttpListenerPolicyIr{},
			expected: false,
		},
		{
			name: "both true",
			ir1: &HttpListenerPolicyIr{
				stripTrailingHostDot: new(true),
			},
			ir2: &HttpListenerPolicyIr{
				stripTrailingHostDot: new(true),
			},
			expected: true,
		},
		{
			name: "true vs false",
			ir1: &HttpListenerPolicyIr{
				stripTrailingHostDot: new(true),
			},
			ir2: &HttpListenerPolicyIr{
				stripTrailingHostDot: new(false),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ir1.Equals(tt.ir2)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyHCMStripTrailingHostDot(t *testing.T) {
	tests := []struct {
		name     string
		value    *bool
		expected bool
	}{
		{
			name:     "nil - trailing dot kept",
			value:    nil,
			expected: false,
		},
		{
			name:     "explicit false",
			value:    new(false),
			expected: false,
		},
		{
			name:     "true - trailing dot stripped",
			value:    new(true),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pass := &listenerPolicyPluginGwPass{}
			pCtx := &ir.HcmContext{
				Policy: &ListenerPolicyIR{
					defaultPolicy: listenerPolicy{
						http: &HttpListenerPolicyIr{
							stripTrailingHostDot: tt.value,
						},
					},
				},
			}
			out := &envoy_hcm.HttpConnectionManager{}

			err := pass.ApplyHCM(pCtx, out)
			require.NoError(t, err)

			require.Equal(t, tt.expected, out.StripTrailingHostDot)
		})
	}
}
