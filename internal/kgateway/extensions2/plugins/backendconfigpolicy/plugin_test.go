package backendconfigpolicy

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	preserve_case_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/header_formatters/preserve_case/v3"
	envoy_upstreams_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

func TestBackendConfigPolicyTranslation(t *testing.T) {
	tests := []struct {
		name    string
		policy  *v1alpha1.BackendConfigPolicy
		cluster *envoyclusterv3.Cluster
		backend *ir.BackendObjectIR
		want    *envoyclusterv3.Cluster
		wantErr bool
	}{
		{
			name: "full configuration",
			policy: &v1alpha1.BackendConfigPolicy{
				Spec: v1alpha1.BackendConfigPolicySpec{
					ConnectTimeout:                ptr.To(metav1.Duration{Duration: 5 * time.Second}),
					PerConnectionBufferLimitBytes: ptr.To(int32(1024)),
					TCPKeepalive: &v1alpha1.TCPKeepalive{
						KeepAliveProbes:   ptr.To(int32(3)),
						KeepAliveTime:     ptr.To(metav1.Duration{Duration: 30 * time.Second}),
						KeepAliveInterval: ptr.To(metav1.Duration{Duration: 5 * time.Second}),
					},
					CommonHttpProtocolOptions: &v1alpha1.CommonHttpProtocolOptions{
						IdleTimeout:              ptr.To(metav1.Duration{Duration: 60 * time.Second}),
						MaxHeadersCount:          ptr.To(int32(100)),
						MaxStreamDuration:        ptr.To(metav1.Duration{Duration: 30 * time.Second}),
						MaxRequestsPerConnection: ptr.To(int32(100)),
					},
					Http1ProtocolOptions: &v1alpha1.Http1ProtocolOptions{
						EnableTrailers:                          ptr.To(true),
						PreserveHttp1HeaderCase:                 ptr.To(true),
						OverrideStreamErrorOnInvalidHttpMessage: ptr.To(true),
					},
				},
			},
			want: &envoyclusterv3.Cluster{
				ConnectTimeout:                durationpb.New(5 * time.Second),
				PerConnectionBufferLimitBytes: &wrapperspb.UInt32Value{Value: 1024},
				UpstreamConnectionOptions: &envoyclusterv3.UpstreamConnectionOptions{
					TcpKeepalive: &envoycorev3.TcpKeepalive{
						KeepaliveProbes:   &wrapperspb.UInt32Value{Value: 3},
						KeepaliveTime:     &wrapperspb.UInt32Value{Value: 30},
						KeepaliveInterval: &wrapperspb.UInt32Value{Value: 5},
					},
				},
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": mustMessageToAny(t, &envoy_upstreams_http_v3.HttpProtocolOptions{
						CommonHttpProtocolOptions: &envoycorev3.HttpProtocolOptions{
							IdleTimeout:              durationpb.New(60 * time.Second),
							MaxHeadersCount:          &wrapperspb.UInt32Value{Value: 100},
							MaxStreamDuration:        durationpb.New(30 * time.Second),
							MaxRequestsPerConnection: &wrapperspb.UInt32Value{Value: 100},
						},
						UpstreamProtocolOptions: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
							ExplicitHttpConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
								ProtocolConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{
									HttpProtocolOptions: &envoycorev3.Http1ProtocolOptions{
										EnableTrailers: true,
										HeaderKeyFormat: &envoycorev3.Http1ProtocolOptions_HeaderKeyFormat{
											HeaderFormat: &envoycorev3.Http1ProtocolOptions_HeaderKeyFormat_StatefulFormatter{
												StatefulFormatter: &envoycorev3.TypedExtensionConfig{
													Name:        PreserveCasePlugin,
													TypedConfig: mustMessageToAny(t, &preserve_case_v3.PreserveCaseFormatterConfig{}),
												},
											},
										},
										OverrideStreamErrorOnInvalidHttpMessage: &wrapperspb.BoolValue{Value: true},
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
						MaxRequestsPerConnection: ptr.To(int32(50)),
					},
				},
			},
			want: &envoyclusterv3.Cluster{
				ConnectTimeout: durationpb.New(2 * time.Second),
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": mustMessageToAny(t, &envoy_upstreams_http_v3.HttpProtocolOptions{
						CommonHttpProtocolOptions: &envoycorev3.HttpProtocolOptions{
							MaxRequestsPerConnection: &wrapperspb.UInt32Value{Value: 50},
						},
						UpstreamProtocolOptions: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
							ExplicitHttpConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
								ProtocolConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
							},
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
			want:    &envoyclusterv3.Cluster{},
			wantErr: false,
		},
		{
			name: "attempt to apply http1 protocol options to http2 backend should not apply",
			policy: &v1alpha1.BackendConfigPolicy{
				Spec: v1alpha1.BackendConfigPolicySpec{
					Http1ProtocolOptions: &v1alpha1.Http1ProtocolOptions{
						EnableTrailers: ptr.To(true),
					},
				},
			},
			backend: &ir.BackendObjectIR{
				AppProtocol: ir.HTTP2AppProtocol,
			},
			cluster: &envoyclusterv3.Cluster{
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": mustMessageToAny(t, &envoy_upstreams_http_v3.HttpProtocolOptions{
						UpstreamProtocolOptions: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
							ExplicitHttpConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
								ProtocolConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
									Http2ProtocolOptions: &envoycorev3.Http2ProtocolOptions{},
								},
							},
						},
					}),
				},
			},
			want: &envoyclusterv3.Cluster{
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": mustMessageToAny(t, &envoy_upstreams_http_v3.HttpProtocolOptions{
						UpstreamProtocolOptions: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
							ExplicitHttpConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
								ProtocolConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
									Http2ProtocolOptions: &envoycorev3.Http2ProtocolOptions{},
								},
							},
						},
					}),
				},
			},
			wantErr: false,
		},
		{
			name: "http2 protocol options applied to http2 backend",
			policy: &v1alpha1.BackendConfigPolicy{
				Spec: v1alpha1.BackendConfigPolicySpec{
					Http2ProtocolOptions: &v1alpha1.Http2ProtocolOptions{
						InitialStreamWindowSize:                 ptr.To(resource.MustParse("64Ki")),
						InitialConnectionWindowSize:             ptr.To(resource.MustParse("64Ki")),
						MaxConcurrentStreams:                    ptr.To(int32(100)),
						OverrideStreamErrorOnInvalidHttpMessage: ptr.To(true),
					},
				},
			},
			backend: &ir.BackendObjectIR{
				AppProtocol: ir.HTTP2AppProtocol,
			},
			cluster: &envoyclusterv3.Cluster{
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": mustMessageToAny(t, &envoy_upstreams_http_v3.HttpProtocolOptions{
						UpstreamProtocolOptions: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
							ExplicitHttpConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
								ProtocolConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
									Http2ProtocolOptions: &envoycorev3.Http2ProtocolOptions{},
								},
							},
						},
					}),
				},
			},
			want: &envoyclusterv3.Cluster{
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": mustMessageToAny(t, &envoy_upstreams_http_v3.HttpProtocolOptions{
						UpstreamProtocolOptions: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_{
							ExplicitHttpConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig{
								ProtocolConfig: &envoy_upstreams_http_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
									Http2ProtocolOptions: &envoycorev3.Http2ProtocolOptions{
										InitialStreamWindowSize:                 &wrapperspb.UInt32Value{Value: 65536},
										InitialConnectionWindowSize:             &wrapperspb.UInt32Value{Value: 65536},
										MaxConcurrentStreams:                    &wrapperspb.UInt32Value{Value: 100},
										OverrideStreamErrorOnInvalidHttpMessage: &wrapperspb.BoolValue{Value: true},
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
			name: "http2 protocol options not applied to non-http2 backend",
			policy: &v1alpha1.BackendConfigPolicy{
				Spec: v1alpha1.BackendConfigPolicySpec{
					Http2ProtocolOptions: &v1alpha1.Http2ProtocolOptions{
						MaxConcurrentStreams: ptr.To(int32(100)),
					},
				},
			},
			backend: &ir.BackendObjectIR{},
			cluster: &envoyclusterv3.Cluster{},
			want:    &envoyclusterv3.Cluster{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First translate the policy
			policyIR, errs := translate(nil, nil, tt.policy)
			if tt.wantErr {
				assert.NotEmpty(t, errs)
				return
			}
			require.Empty(t, errs)

			// Then process the backend with the translated policy
			cluster := tt.cluster
			if cluster == nil {
				cluster = &envoyclusterv3.Cluster{}
			}
			backend := tt.backend
			if backend == nil {
				backend = &ir.BackendObjectIR{}
			}
			processBackend(context.Background(), policyIR, *backend, cluster)
			assert.Equal(t, tt.want, cluster)
		})
	}
}

// mustMessageToAny is a helper function to handle MessageToAny error in test cases
func mustMessageToAny(t *testing.T, msg proto.Message) *anypb.Any {
	a, err := utils.MessageToAny(msg)
	require.NoError(t, err, "failed to convert message to Any")
	return a
}
