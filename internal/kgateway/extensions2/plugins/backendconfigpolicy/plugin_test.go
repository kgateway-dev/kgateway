package backendconfigpolicy

import (
	"context"
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TestBackendConfigPolicyFlow(t *testing.T) {
	tests := []struct {
		name    string
		policy  *v1alpha1.BackendConfigPolicy
		want    *clusterv3.Cluster
		wantErr bool
	}{
		{
			name: "full configuration",
			policy: &v1alpha1.BackendConfigPolicy{
				Spec: v1alpha1.BackendConfigPolicySpec{
					MaxRequestsPerConnection:      ptr.To(100),
					ConnectTimeout:                ptr.To(gwv1.Duration("5s")),
					PerConnectionBufferLimitBytes: ptr.To(1024),
					TCPKeepalive: &v1alpha1.TCPKeepalive{
						KeepAliveProbes:   ptr.To(3),
						KeepAliveTime:     ptr.To(gwv1.Duration("30s")),
						KeepAliveInterval: ptr.To(gwv1.Duration("5s")),
					},
					CommonHttpProtocolOptions: &v1alpha1.CommonHttpProtocolOptions{
						IdleTimeout:                  ptr.To(gwv1.Duration("60s")),
						MaxHeadersCount:              ptr.To(100),
						MaxStreamDuration:            ptr.To(gwv1.Duration("30s")),
						HeadersWithUnderscoresAction: ptr.To(v1alpha1.HeadersWithUnderscoresActionAllow),
					},
					Http1ProtocolOptions: &v1alpha1.Http1ProtocolOptions{
						EnableTrailers:                          ptr.To(true),
						OverrideStreamErrorOnInvalidHttpMessage: ptr.To(true),
					},
				},
			},
			want: &clusterv3.Cluster{
				MaxRequestsPerConnection:      &wrapperspb.UInt32Value{Value: 100},
				ConnectTimeout:                durationpb.New(5 * time.Second),
				PerConnectionBufferLimitBytes: &wrapperspb.UInt32Value{Value: 1024},
				UpstreamConnectionOptions: &clusterv3.UpstreamConnectionOptions{
					TcpKeepalive: &corev3.TcpKeepalive{
						KeepaliveProbes:   &wrapperspb.UInt32Value{Value: 3},
						KeepaliveTime:     &wrapperspb.UInt32Value{Value: 30},
						KeepaliveInterval: &wrapperspb.UInt32Value{Value: 5},
					},
				},
				CommonHttpProtocolOptions: &corev3.HttpProtocolOptions{
					IdleTimeout:                  durationpb.New(60 * time.Second),
					MaxHeadersCount:              &wrapperspb.UInt32Value{Value: 100},
					MaxStreamDuration:            durationpb.New(30 * time.Second),
					HeadersWithUnderscoresAction: corev3.HttpProtocolOptions_ALLOW,
				},
				HttpProtocolOptions: &corev3.Http1ProtocolOptions{
					EnableTrailers:                          true,
					OverrideStreamErrorOnInvalidHttpMessage: &wrapperspb.BoolValue{Value: true},
				},
			},
			wantErr: false,
		},
		{
			name: "minimal configuration",
			policy: &v1alpha1.BackendConfigPolicy{
				Spec: v1alpha1.BackendConfigPolicySpec{
					MaxRequestsPerConnection: ptr.To(50),
					ConnectTimeout:           ptr.To(gwv1.Duration("2s")),
				},
			},
			want: &clusterv3.Cluster{
				MaxRequestsPerConnection: &wrapperspb.UInt32Value{Value: 50},
				ConnectTimeout:           durationpb.New(2 * time.Second),
			},
			wantErr: false,
		},
		{
			name: "invalid duration",
			policy: &v1alpha1.BackendConfigPolicy{
				Spec: v1alpha1.BackendConfigPolicySpec{
					ConnectTimeout: ptr.To(gwv1.Duration("invalid")),
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty policy",
			policy: &v1alpha1.BackendConfigPolicy{
				Spec: v1alpha1.BackendConfigPolicySpec{},
			},
			want:    &clusterv3.Cluster{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First translate the policy
			policyIR, err := translate(tt.policy)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Then process the backend with the translated policy
			cluster := &clusterv3.Cluster{}
			processBackend(context.Background(), policyIR, ir.BackendObjectIR{}, cluster)

			// Compare the resulting cluster configuration
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.Equal(t, tt.want, cluster)
		})
	}
}

func TestHeaderFormat(t *testing.T) {
	policy := &v1alpha1.BackendConfigPolicy{
		Spec: v1alpha1.BackendConfigPolicySpec{
			Http1ProtocolOptions: &v1alpha1.Http1ProtocolOptions{
				HeaderFormat: ptr.To(v1alpha1.PreserveCaseHeaderKeyFormat),
			},
		},
	}

	policyIR, err := translate(policy)
	require.NoError(t, err)

	cluster := &clusterv3.Cluster{}
	processBackend(context.Background(), policyIR, ir.BackendObjectIR{}, cluster)

	assert.NotNil(t, cluster.HttpProtocolOptions.HeaderKeyFormat.GetStatefulFormatter())
	assert.Nil(t, cluster.HttpProtocolOptions.HeaderKeyFormat.GetProperCaseWords())

	policy2 := &v1alpha1.BackendConfigPolicy{
		Spec: v1alpha1.BackendConfigPolicySpec{
			Http1ProtocolOptions: &v1alpha1.Http1ProtocolOptions{
				HeaderFormat: ptr.To(v1alpha1.ProperCaseHeaderKeyFormat),
			},
		},
	}

	policyIR2, err := translate(policy2)
	require.NoError(t, err)

	cluster2 := &clusterv3.Cluster{}
	processBackend(context.Background(), policyIR2, ir.BackendObjectIR{}, cluster2)

	assert.Nil(t, cluster2.HttpProtocolOptions.HeaderKeyFormat.GetStatefulFormatter())
	assert.NotNil(t, cluster2.HttpProtocolOptions.HeaderKeyFormat.GetProperCaseWords())
}
