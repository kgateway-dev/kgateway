package listenerpolicy

import (
	"testing"

	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestHttpListenerPolicyIrEqualsStripHostPort(t *testing.T) {
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
				stripHostPortMode: new(kgateway.StripAnyHostPortMode),
			},
			ir2:      &HttpListenerPolicyIr{},
			expected: false,
		},
		{
			name: "both MatchingPort",
			ir1: &HttpListenerPolicyIr{
				stripHostPortMode: new(kgateway.StripMatchingHostPortMode),
			},
			ir2: &HttpListenerPolicyIr{
				stripHostPortMode: new(kgateway.StripMatchingHostPortMode),
			},
			expected: true,
		},
		{
			name: "both AnyPort",
			ir1: &HttpListenerPolicyIr{
				stripHostPortMode: new(kgateway.StripAnyHostPortMode),
			},
			ir2: &HttpListenerPolicyIr{
				stripHostPortMode: new(kgateway.StripAnyHostPortMode),
			},
			expected: true,
		},
		{
			name: "MatchingPort vs AnyPort",
			ir1: &HttpListenerPolicyIr{
				stripHostPortMode: new(kgateway.StripMatchingHostPortMode),
			},
			ir2: &HttpListenerPolicyIr{
				stripHostPortMode: new(kgateway.StripAnyHostPortMode),
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

func TestApplyHCMStripHostPortMode(t *testing.T) {
	tests := []struct {
		name                   string
		mode                   *kgateway.StripHostPortMode
		expectMatchingHostPort bool
		expectAnyHostPort      bool
	}{
		{
			name:                   "nil - no stripping",
			mode:                   nil,
			expectMatchingHostPort: false,
			expectAnyHostPort:      false,
		},
		{
			name:                   "MatchingPort",
			mode:                   new(kgateway.StripMatchingHostPortMode),
			expectMatchingHostPort: true,
			expectAnyHostPort:      false,
		},
		{
			name:                   "AnyPort",
			mode:                   new(kgateway.StripAnyHostPortMode),
			expectMatchingHostPort: false,
			expectAnyHostPort:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pass := &listenerPolicyPluginGwPass{}
			pCtx := &ir.HcmContext{
				Policy: &ListenerPolicyIR{
					defaultPolicy: listenerPolicy{
						http: &HttpListenerPolicyIr{
							stripHostPortMode: tt.mode,
						},
					},
				},
			}
			out := &envoy_hcm.HttpConnectionManager{}

			err := pass.ApplyHCM(pCtx, out)
			require.NoError(t, err)

			require.Equal(t, tt.expectMatchingHostPort, out.StripMatchingHostPort)
			require.Equal(t, tt.expectAnyHostPort, out.GetStripAnyHostPort())
		})
	}
}
