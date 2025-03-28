package routepolicy

import (
	"testing"

	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TestBuildEnvoyExtProc(t *testing.T) {
	tests := []struct {
		name           string
		gatewayExt     *ir.GatewayExtension
		extprocConfig  *v1alpha1.ExtProcPolicy
		expectedError  string
		validateResult func(*testing.T, *envoy_ext_proc_v3.ExternalProcessor)
	}{
		{
			name: "basic configuration",
			gatewayExt: &ir.GatewayExtension{
				ExtProc: &v1alpha1.ExtProcProvider{
					GrpcService: &v1alpha1.ExtGrpcService{},
				},
			},
			extprocConfig: &v1alpha1.ExtProcPolicy{},
			validateResult: func(t *testing.T, result *envoy_ext_proc_v3.ExternalProcessor) {
				assert.NotNil(t, result.GrpcService)
				assert.NotNil(t, result.GrpcService.GetEnvoyGrpc())
				assert.Equal(t, "test-backend", result.GrpcService.GetEnvoyGrpc().ClusterName)
			},
		},
		{
			name: "with authority",
			gatewayExt: &ir.GatewayExtension{
				ExtProc: &v1alpha1.ExtProcProvider{
					GrpcService: &v1alpha1.ExtGrpcService{
						Authority: stringPtr("test-authority"),
					},
				},
			},
			extprocConfig: &v1alpha1.ExtProcPolicy{},
			validateResult: func(t *testing.T, result *envoy_ext_proc_v3.ExternalProcessor) {
				assert.Equal(t, "test-authority", result.GrpcService.GetEnvoyGrpc().Authority)
			},
		},
		{
			name: "with failure mode allow",
			gatewayExt: &ir.GatewayExtension{
				ExtProc: &v1alpha1.ExtProcProvider{
					GrpcService: &v1alpha1.ExtGrpcService{},
				},
			},
			extprocConfig: &v1alpha1.ExtProcPolicy{
				FailureModeAllow: boolPtr(true),
			},
			validateResult: func(t *testing.T, result *envoy_ext_proc_v3.ExternalProcessor) {
				assert.True(t, result.FailureModeAllow)
			},
		},
		{
			name: "with all processing modes",
			gatewayExt: &ir.GatewayExtension{
				ExtProc: &v1alpha1.ExtProcProvider{
					GrpcService: &v1alpha1.ExtGrpcService{},
				},
			},
			extprocConfig: &v1alpha1.ExtProcPolicy{
				ProcessingMode: &v1alpha1.ProcessingMode{
					RequestHeaderMode:   stringPtr("SEND"),
					ResponseHeaderMode:  stringPtr("SKIP"),
					RequestBodyMode:     stringPtr("STREAMED"),
					ResponseBodyMode:    stringPtr("BUFFERED"),
					RequestTrailerMode:  stringPtr("SEND"),
					ResponseTrailerMode: stringPtr("SKIP"),
				},
			},
			validateResult: func(t *testing.T, result *envoy_ext_proc_v3.ExternalProcessor) {
				assert.NotNil(t, result.ProcessingMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_SEND, result.ProcessingMode.RequestHeaderMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_SKIP, result.ProcessingMode.ResponseHeaderMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_STREAMED, result.ProcessingMode.RequestBodyMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_BUFFERED, result.ProcessingMode.ResponseBodyMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_SEND, result.ProcessingMode.RequestTrailerMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_SKIP, result.ProcessingMode.ResponseTrailerMode)
			},
		},
		{
			name: "with default processing modes",
			gatewayExt: &ir.GatewayExtension{
				ExtProc: &v1alpha1.ExtProcProvider{
					GrpcService: &v1alpha1.ExtGrpcService{},
				},
			},
			extprocConfig: &v1alpha1.ExtProcPolicy{
				ProcessingMode: &v1alpha1.ProcessingMode{},
			},
			validateResult: func(t *testing.T, result *envoy_ext_proc_v3.ExternalProcessor) {
				assert.NotNil(t, result.ProcessingMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_DEFAULT, result.ProcessingMode.RequestHeaderMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_DEFAULT, result.ProcessingMode.ResponseHeaderMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_NONE, result.ProcessingMode.RequestBodyMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_NONE, result.ProcessingMode.ResponseBodyMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_DEFAULT, result.ProcessingMode.RequestTrailerMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_DEFAULT, result.ProcessingMode.ResponseTrailerMode)
			},
		},
		{
			name: "with invalid processing modes",
			gatewayExt: &ir.GatewayExtension{
				ExtProc: &v1alpha1.ExtProcProvider{
					GrpcService: &v1alpha1.ExtGrpcService{},
				},
			},
			extprocConfig: &v1alpha1.ExtProcPolicy{
				ProcessingMode: &v1alpha1.ProcessingMode{
					RequestHeaderMode:   stringPtr("INVALID"),
					ResponseHeaderMode:  stringPtr("INVALID"),
					RequestBodyMode:     stringPtr("INVALID"),
					ResponseBodyMode:    stringPtr("INVALID"),
					RequestTrailerMode:  stringPtr("INVALID"),
					ResponseTrailerMode: stringPtr("INVALID"),
				},
			},
			validateResult: func(t *testing.T, result *envoy_ext_proc_v3.ExternalProcessor) {
				assert.NotNil(t, result.ProcessingMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_DEFAULT, result.ProcessingMode.RequestHeaderMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_DEFAULT, result.ProcessingMode.ResponseHeaderMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_NONE, result.ProcessingMode.RequestBodyMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_NONE, result.ProcessingMode.ResponseBodyMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_DEFAULT, result.ProcessingMode.RequestTrailerMode)
				assert.Equal(t, envoy_ext_proc_v3.ProcessingMode_DEFAULT, result.ProcessingMode.ResponseTrailerMode)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildEnvoyExtProc("test-backend", tt.gatewayExt, tt.extprocConfig)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			tt.validateResult(t, result)
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
