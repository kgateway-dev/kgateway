package listenerpolicy

import (
	"testing"

	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestConvertPathWithEscapedSlashesAction(t *testing.T) {
	tests := []struct {
		name     string
		action   *kgateway.PathWithEscapedSlashesAction
		expected *envoy_hcm.HttpConnectionManager_PathWithEscapedSlashesAction
	}{
		{
			name:     "unset",
			action:   nil,
			expected: nil,
		},
		{
			name:     "KeepUnchanged",
			action:   new(kgateway.PathWithEscapedSlashesActionKeepUnchanged),
			expected: new(envoy_hcm.HttpConnectionManager_KEEP_UNCHANGED),
		},
		{
			name:     "RejectRequest",
			action:   new(kgateway.PathWithEscapedSlashesActionRejectRequest),
			expected: new(envoy_hcm.HttpConnectionManager_REJECT_REQUEST),
		},
		{
			name:     "UnescapeAndRedirect",
			action:   new(kgateway.PathWithEscapedSlashesActionUnescapeAndRedirect),
			expected: new(envoy_hcm.HttpConnectionManager_UNESCAPE_AND_REDIRECT),
		},
		{
			name:     "UnescapeAndForward",
			action:   new(kgateway.PathWithEscapedSlashesActionUnescapeAndForward),
			expected: new(envoy_hcm.HttpConnectionManager_UNESCAPE_AND_FORWARD),
		},
		{
			name:     "unknown value",
			action:   new(kgateway.PathWithEscapedSlashesAction("Bogus")),
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, convertPathWithEscapedSlashesAction(tt.action))
		})
	}
}

func TestApplyHCMPathWithEscapedSlashesAction(t *testing.T) {
	tests := []struct {
		name     string
		action   *envoy_hcm.HttpConnectionManager_PathWithEscapedSlashesAction
		expected envoy_hcm.HttpConnectionManager_PathWithEscapedSlashesAction
	}{
		{
			name:     "nil - envoy default",
			action:   nil,
			expected: envoy_hcm.HttpConnectionManager_IMPLEMENTATION_SPECIFIC_DEFAULT,
		},
		{
			name:     "KeepUnchanged",
			action:   new(envoy_hcm.HttpConnectionManager_KEEP_UNCHANGED),
			expected: envoy_hcm.HttpConnectionManager_KEEP_UNCHANGED,
		},
		{
			name:     "UnescapeAndRedirect",
			action:   new(envoy_hcm.HttpConnectionManager_UNESCAPE_AND_REDIRECT),
			expected: envoy_hcm.HttpConnectionManager_UNESCAPE_AND_REDIRECT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pass := &listenerPolicyPluginGwPass{}
			pCtx := &ir.HcmContext{
				Policy: &ListenerPolicyIR{
					defaultPolicy: listenerPolicy{
						http: &HttpListenerPolicyIr{
							pathWithEscapedSlashesAction: tt.action,
						},
					},
				},
			}
			out := &envoy_hcm.HttpConnectionManager{}

			err := pass.ApplyHCM(pCtx, out)
			require.NoError(t, err)

			require.Equal(t, tt.expected, out.GetPathWithEscapedSlashesAction())
		})
	}
}
