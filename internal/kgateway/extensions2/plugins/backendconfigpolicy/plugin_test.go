package backendconfigpolicy

import (
	"context"
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	preserve_case_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/header_formatters/preserve_case/v3"
	envoy_upstreams_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
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
					ConnectTimeout:                ptr.To(metav1.Duration{Duration: 5 * time.Second}),
					PerConnectionBufferLimitBytes: ptr.To(1024),
					TCPKeepalive: &v1alpha1.TCPKeepalive{
						KeepAliveProbes:   ptr.To(3),
						KeepAliveTime:     ptr.To(metav1.Duration{Duration: 30 * time.Second}),
						KeepAliveInterval: ptr.To(metav1.Duration{Duration: 5 * time.Second}),
					},
					CommonHttpProtocolOptions: &v1alpha1.CommonHttpProtocolOptions{
						IdleTimeout:                  ptr.To(metav1.Duration{Duration: 60 * time.Second}),
						MaxHeadersCount:              ptr.To(100),
						MaxStreamDuration:            ptr.To(metav1.Duration{Duration: 30 * time.Second}),
						HeadersWithUnderscoresAction: ptr.To(v1alpha1.HeadersWithUnderscoresActionAllow),
						MaxRequestsPerConnection:     ptr.To(100),
					},
					Http1ProtocolOptions: &v1alpha1.Http1ProtocolOptions{
						EnableTrailers:                          ptr.To(true),
						HeaderFormat:                            ptr.To(v1alpha1.PreserveCaseHeaderKeyFormat),
						OverrideStreamErrorOnInvalidHttpMessage: ptr.To(true),
					},
				},
			},
			want: &clusterv3.Cluster{
				ConnectTimeout:                durationpb.New(5 * time.Second),
				PerConnectionBufferLimitBytes: &wrapperspb.UInt32Value{Value: 1024},
				UpstreamConnectionOptions: &clusterv3.UpstreamConnectionOptions{
					TcpKeepalive: &corev3.TcpKeepalive{
						KeepaliveProbes:   &wrapperspb.UInt32Value{Value: 3},
						KeepaliveTime:     &wrapperspb.UInt32Value{Value: 30},
						KeepaliveInterval: &wrapperspb.UInt32Value{Value: 5},
					},
				},
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": mustMessageToAny(t, &envoy_upstreams_http_v3.HttpProtocolOptions{
						CommonHttpProtocolOptions: &corev3.HttpProtocolOptions{
							IdleTimeout:                  durationpb.New(60 * time.Second),
							MaxHeadersCount:              &wrapperspb.UInt32Value{Value: 100},
							MaxStreamDuration:            durationpb.New(30 * time.Second),
							HeadersWithUnderscoresAction: corev3.HttpProtocolOptions_ALLOW,
							MaxRequestsPerConnection:     &wrapperspb.UInt32Value{Value: 100},
						},
						UpstreamProtocolOptions: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
							ExplicitHttpConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
								ProtocolConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{
									HttpProtocolOptions: &corev3.Http1ProtocolOptions{
										EnableTrailers:                          true,
										OverrideStreamErrorOnInvalidHttpMessage: &wrapperspb.BoolValue{Value: true},
										HeaderKeyFormat: &corev3.Http1ProtocolOptions_HeaderKeyFormat{
											HeaderFormat: &corev3.Http1ProtocolOptions_HeaderKeyFormat_StatefulFormatter{
												StatefulFormatter: &corev3.TypedExtensionConfig{
													Name:        PreserveCasePlugin,
													TypedConfig: mustMessageToAny(t, &preserve_case_v3.PreserveCaseFormatterConfig{}),
												},
											},
										},
									},
								},
							},
						},
					}),
				},
			},
			wantErr: false,
		},
		{
			name: "minimal configuration",
			policy: &v1alpha1.BackendConfigPolicy{
				Spec: v1alpha1.BackendConfigPolicySpec{
					ConnectTimeout: ptr.To(metav1.Duration{Duration: 2 * time.Second}),
					CommonHttpProtocolOptions: &v1alpha1.CommonHttpProtocolOptions{
						MaxRequestsPerConnection: ptr.To(50),
					},
				},
			},
			want: &clusterv3.Cluster{
				ConnectTimeout: durationpb.New(2 * time.Second),
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": mustMessageToAny(t, &envoy_upstreams_http_v3.HttpProtocolOptions{
						CommonHttpProtocolOptions: &corev3.HttpProtocolOptions{
							MaxRequestsPerConnection: &wrapperspb.UInt32Value{Value: 50},
						},
					}),
				},
			},
			wantErr: false,
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

// Helper function to handle MessageToAny error in test cases
func mustMessageToAny(t *testing.T, msg proto.Message) *anypb.Any {
	a, err := utils.MessageToAny(msg)
	require.NoError(t, err, "failed to convert message to Any")
	return a
}
