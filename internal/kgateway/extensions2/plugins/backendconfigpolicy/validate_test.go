package backendconfigpolicy

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

// mockValidator implements validator.Validator for testing
type mockValidator struct {
	validateFunc func(ctx context.Context, config string) error
}

var _ validator.Validator = &mockValidator{}

func (m *mockValidator) Validate(ctx context.Context, config string) error {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, config)
	}
	return nil
}

func TestBackendConfigPolicyXDSValidation(t *testing.T) {
	tests := []struct {
		name      string
		policyIR  *BackendConfigPolicyIR
		validator *mockValidator
		mode      apisettings.ValidationMode
		wantErr   bool
	}{
		{
			name: "valid policy with successful validation in strict mode",
			policyIR: &BackendConfigPolicyIR{
				ct:             time.Now(),
				connectTimeout: durationpb.New(5 * time.Second),
			},
			validator: &mockValidator{
				validateFunc: func(ctx context.Context, config string) error {
					return nil // Successful validation
				},
			},
			mode:    apisettings.ValidationStrict,
			wantErr: false,
		},
		{
			name: "invalid TLS policy with failed cipher suite validation in strict mode",
			policyIR: &BackendConfigPolicyIR{
				ct: time.Now(),
				tlsConfig: &envoytlsv3.UpstreamTlsContext{
					CommonTlsContext: &envoytlsv3.CommonTlsContext{
						TlsParams: &envoytlsv3.TlsParameters{
							CipherSuites: []string{"BOGUS_CIPHER_SUITE_1", "INVALID_AES_256_GCM_SHA384"},
						},
					},
				},
			},
			validator: &mockValidator{
				validateFunc: func(ctx context.Context, config string) error {
					return errors.New("Failed to initialize cipher suites BOGUS_CIPHER_SUITE_1:INVALID_AES_256_GCM_SHA384")
				},
			},
			mode:    apisettings.ValidationStrict,
			wantErr: true,
		},
		{
			name: "invalid TLS policy with failed ECDH curve validation in strict mode",
			policyIR: &BackendConfigPolicyIR{
				ct: time.Now(),
				tlsConfig: &envoytlsv3.UpstreamTlsContext{
					CommonTlsContext: &envoytlsv3.CommonTlsContext{
						TlsParams: &envoytlsv3.TlsParameters{
							EcdhCurves: []string{"invalid-curve-p256", "bogus-secp384r1"},
						},
					},
				},
			},
			validator: &mockValidator{
				validateFunc: func(ctx context.Context, config string) error {
					return errors.New("Failed to initialize ECDH curves")
				},
			},
			mode:    apisettings.ValidationStrict,
			wantErr: true,
		},
		{
			name: "invalid policy but validation skipped in standard mode",
			policyIR: &BackendConfigPolicyIR{
				ct: time.Now(),
				tlsConfig: &envoytlsv3.UpstreamTlsContext{
					CommonTlsContext: &envoytlsv3.CommonTlsContext{
						TlsParams: &envoytlsv3.TlsParameters{
							CipherSuites: []string{"BOGUS_CIPHER_SUITE_1"},
						},
					},
				},
			},
			validator: &mockValidator{
				validateFunc: func(ctx context.Context, config string) error {
					return errors.New("should not be called in standard mode")
				},
			},
			mode:    apisettings.ValidationStandard,
			wantErr: false, // No error because validation is skipped
		},
		{
			name: "policy with useHostnameForHashing uses STRICT_DNS cluster for validation",
			policyIR: &BackendConfigPolicyIR{
				ct: time.Now(),
				loadBalancerConfig: &LoadBalancerConfigIR{
					useHostnameForHashing: true,
				},
			},
			validator: &mockValidator{
				validateFunc: func(ctx context.Context, config string) error {
					// Verify that the cluster uses STRICT_DNS when useHostnameForHashing is enabled
					var bootstrap envoybootstrapv3.Bootstrap
					if err := protojson.Unmarshal([]byte(config), &bootstrap); err != nil {
						return err
					}
					if len(bootstrap.StaticResources.Clusters) != 1 {
						return errors.New("expected exactly one cluster in bootstrap")
					}
					cluster := bootstrap.StaticResources.Clusters[0]
					if cluster.GetType() != envoyclusterv3.Cluster_STRICT_DNS {
						return fmt.Errorf("expected STRICT_DNS cluster type, got %v", cluster.GetType())
					}
					return nil // Validation passes
				},
			},
			mode:    apisettings.ValidationStrict,
			wantErr: false,
		},
		{
			name: "policy without useHostnameForHashing uses STATIC cluster for validation",
			policyIR: &BackendConfigPolicyIR{
				ct:             time.Now(),
				connectTimeout: durationpb.New(10 * time.Second),
			},
			validator: &mockValidator{
				validateFunc: func(ctx context.Context, config string) error {
					// Verify that the cluster uses STATIC when useHostnameForHashing is not enabled
					var bootstrap envoybootstrapv3.Bootstrap
					if err := protojson.Unmarshal([]byte(config), &bootstrap); err != nil {
						return err
					}
					if len(bootstrap.StaticResources.Clusters) != 1 {
						return errors.New("expected exactly one cluster in bootstrap")
					}
					cluster := bootstrap.StaticResources.Clusters[0]
					if cluster.GetType() != envoyclusterv3.Cluster_STATIC {
						return fmt.Errorf("expected STATIC cluster type, got %v", cluster.GetType())
					}
					return nil // Validation passes
				},
			},
			mode:    apisettings.ValidationStrict,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateXDS(t.Context(), tt.policyIR, tt.validator, tt.mode)
			if tt.wantErr {
				assert.Error(t, err, "expected validation to fail but it succeeded")
			} else {
				assert.NoError(t, err, "expected validation to succeed but it failed: %v", err)
			}
		})
	}
}
