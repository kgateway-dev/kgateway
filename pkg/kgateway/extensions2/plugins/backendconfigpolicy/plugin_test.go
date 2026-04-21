package backendconfigpolicy

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
	preserve_case_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/header_formatters/preserve_case/v3"
	envoyproxyprotocolv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/proxy_protocol/v3"
	envoyrawbufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/raw_buffer/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_upstreams_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestBackendConfigPolicyTranslation(t *testing.T) {
	tests := []struct {
		name    string
		policy  *kgateway.BackendConfigPolicy
		cluster *envoyclusterv3.Cluster
		backend *ir.BackendObjectIR
		want    *envoyclusterv3.Cluster
		wantErr bool
	}{
		{
			name: "full configuration",
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					ConnectTimeout:                new(metav1.Duration{Duration: 5 * time.Second}),
					PerConnectionBufferLimitBytes: new(int32(1024)),
					TCPKeepalive: &kgateway.TCPKeepalive{
						KeepAliveProbes:   new(int32(3)),
						KeepAliveTime:     new(metav1.Duration{Duration: 30 * time.Second}),
						KeepAliveInterval: new(metav1.Duration{Duration: 5 * time.Second}),
					},
					CommonHttpProtocolOptions: &kgateway.CommonHttpProtocolOptions{
						IdleTimeout:              new(metav1.Duration{Duration: 60 * time.Second}),
						MaxHeadersCount:          new(int32(100)),
						MaxStreamDuration:        new(metav1.Duration{Duration: 30 * time.Second}),
						MaxRequestsPerConnection: new(int32(100)),
					},
					Http1ProtocolOptions: &kgateway.Http1ProtocolOptions{
						EnableTrailers:                          new(true),
						PreserveHttp1HeaderCase:                 new(true),
						OverrideStreamErrorOnInvalidHttpMessage: new(true),
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
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					ConnectTimeout: new(metav1.Duration{Duration: 2 * time.Second}),
					CommonHttpProtocolOptions: &kgateway.CommonHttpProtocolOptions{
						MaxRequestsPerConnection: new(int32(50)),
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
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{},
			},
			want:    &envoyclusterv3.Cluster{},
			wantErr: false,
		},
		{
			name: "attempt to apply http1 protocol options to http2 backend should not apply",
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					Http1ProtocolOptions: &kgateway.Http1ProtocolOptions{
						EnableTrailers: new(true),
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
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					Http2ProtocolOptions: &kgateway.Http2ProtocolOptions{
						InitialStreamWindowSize:                 new(resource.MustParse("64Ki")),
						InitialConnectionWindowSize:             new(resource.MustParse("64Ki")),
						MaxConcurrentStreams:                    new(int32(100)),
						OverrideStreamErrorOnInvalidHttpMessage: new(true),
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
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					Http2ProtocolOptions: &kgateway.Http2ProtocolOptions{
						MaxConcurrentStreams: new(int32(100)),
					},
				},
			},
			backend: &ir.BackendObjectIR{},
			cluster: &envoyclusterv3.Cluster{},
			want:    &envoyclusterv3.Cluster{},
			wantErr: false,
		},
		{
			name: "circuit breakers minimal configuration",
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					CircuitBreakers: &kgateway.CircuitBreakers{
						MaxConnections: new(int32(100)),
					},
				},
			},
			want: &envoyclusterv3.Cluster{
				CircuitBreakers: &envoyclusterv3.CircuitBreakers{
					Thresholds: []*envoyclusterv3.CircuitBreakers_Thresholds{
						{
							MaxConnections: &wrapperspb.UInt32Value{Value: 100},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "circuit breakers full configuration",
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					CircuitBreakers: &kgateway.CircuitBreakers{
						MaxConnections:     new(int32(1000)),
						MaxPendingRequests: new(int32(500)),
						MaxRequests:        new(int32(2000)),
						MaxRetries:         new(int32(10)),
					},
				},
			},
			want: &envoyclusterv3.Cluster{
				CircuitBreakers: &envoyclusterv3.CircuitBreakers{
					Thresholds: []*envoyclusterv3.CircuitBreakers_Thresholds{
						{
							MaxConnections:     &wrapperspb.UInt32Value{Value: 1000},
							MaxPendingRequests: &wrapperspb.UInt32Value{Value: 500},
							MaxRequests:        &wrapperspb.UInt32Value{Value: 2000},
							MaxRetries:         &wrapperspb.UInt32Value{Value: 10},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "circuit breakers with track remaining",
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					CircuitBreakers: &kgateway.CircuitBreakers{
						MaxConnections: new(int32(100)),
						TrackRemaining: func() *bool { v := true; return &v }(),
					},
				},
			},
			want: &envoyclusterv3.Cluster{
				CircuitBreakers: &envoyclusterv3.CircuitBreakers{
					Thresholds: []*envoyclusterv3.CircuitBreakers_Thresholds{
						{
							MaxConnections: &wrapperspb.UInt32Value{Value: 100},
							TrackRemaining: true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "upstream proxy protocol V1 without TLS",
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					UpstreamProxyProtocol: &kgateway.UpstreamProxyProtocol{
						Version: new(kgateway.ProxyProtocolVersionV1),
					},
				},
			},
			want: &envoyclusterv3.Cluster{
				TransportSocket: &envoycorev3.TransportSocket{
					Name: TransportSocketUpstreamProxyProtocol,
					ConfigType: &envoycorev3.TransportSocket_TypedConfig{
						TypedConfig: mustMessageToAny(t, &envoyproxyprotocolv3.ProxyProtocolUpstreamTransport{
							Config: &envoycorev3.ProxyProtocolConfig{
								Version: envoycorev3.ProxyProtocolConfig_V1,
							},
							TransportSocket: &envoycorev3.TransportSocket{
								Name: envoywellknown.TransportSocketRawBuffer,
								ConfigType: &envoycorev3.TransportSocket_TypedConfig{
									TypedConfig: mustMessageToAny(t, &envoyrawbufferv3.RawBuffer{}),
								},
							},
						}),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "upstream proxy protocol V2 without TLS",
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					UpstreamProxyProtocol: &kgateway.UpstreamProxyProtocol{
						Version: new(kgateway.ProxyProtocolVersionV2),
					},
				},
			},
			want: &envoyclusterv3.Cluster{
				TransportSocket: &envoycorev3.TransportSocket{
					Name: TransportSocketUpstreamProxyProtocol,
					ConfigType: &envoycorev3.TransportSocket_TypedConfig{
						TypedConfig: mustMessageToAny(t, &envoyproxyprotocolv3.ProxyProtocolUpstreamTransport{
							Config: &envoycorev3.ProxyProtocolConfig{
								Version: envoycorev3.ProxyProtocolConfig_V2,
							},
							TransportSocket: &envoycorev3.TransportSocket{
								Name: envoywellknown.TransportSocketRawBuffer,
								ConfigType: &envoycorev3.TransportSocket_TypedConfig{
									TypedConfig: mustMessageToAny(t, &envoyrawbufferv3.RawBuffer{}),
								},
							},
						}),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "upstream proxy protocol V1 with TLS",
			policy: &kgateway.BackendConfigPolicy{
				Spec: kgateway.BackendConfigPolicySpec{
					UpstreamProxyProtocol: &kgateway.UpstreamProxyProtocol{
						Version: new(kgateway.ProxyProtocolVersionV1),
					},
				},
			},
			cluster: &envoyclusterv3.Cluster{
				TransportSocket: &envoycorev3.TransportSocket{
					Name: envoywellknown.TransportSocketTls,
					ConfigType: &envoycorev3.TransportSocket_TypedConfig{
						TypedConfig: mustMessageToAny(t, &envoytlsv3.UpstreamTlsContext{
							Sni: "example.com",
						}),
					},
				},
			},
			want: &envoyclusterv3.Cluster{
				TransportSocket: &envoycorev3.TransportSocket{
					Name: TransportSocketUpstreamProxyProtocol,
					ConfigType: &envoycorev3.TransportSocket_TypedConfig{
						TypedConfig: mustMessageToAny(t, &envoyproxyprotocolv3.ProxyProtocolUpstreamTransport{
							Config: &envoycorev3.ProxyProtocolConfig{
								Version: envoycorev3.ProxyProtocolConfig_V1,
							},
							TransportSocket: &envoycorev3.TransportSocket{
								Name: envoywellknown.TransportSocketTls,
								ConfigType: &envoycorev3.TransportSocket_TypedConfig{
									TypedConfig: mustMessageToAny(t, &envoytlsv3.UpstreamTlsContext{
										Sni: "example.com",
									}),
								},
							},
						}),
					},
				},
			},
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

func TestBackendConfigPolicyDnsClusterConfig(t *testing.T) {
	t.Run("applies dns settings to hostname-based static backends", func(t *testing.T) {
		policyIR, errs := translate(nil, nil, &kgateway.BackendConfigPolicy{
			Spec: kgateway.BackendConfigPolicySpec{
				DNS: &kgateway.DNS{
					RefreshRate: &metav1.Duration{Duration: 60 * time.Second},
					Jitter:      &metav1.Duration{Duration: 15 * time.Second},
					RespectTTL:  new(true),
				},
			},
		})
		require.Empty(t, errs)

		cluster := &envoyclusterv3.Cluster{
			ClusterDiscoveryType: &envoyclusterv3.Cluster_ClusterType{
				ClusterType: &envoyclusterv3.Cluster_CustomClusterType{
					Name:        dnsClusterExtensionName,
					TypedConfig: mustMessageToAny(t, &envoydnsv3.DnsCluster{}),
				},
			},
		}
		backend := ir.BackendObjectIR{
			Obj: &kgateway.Backend{
				Spec: kgateway.BackendSpec{
					Static: &kgateway.StaticBackend{
						Hosts: []kgateway.Host{{
							Host: "example.com",
							Port: 8080,
						}},
					},
				},
			},
		}

		processBackend(context.Background(), policyIR, backend, cluster)

		var dnsCluster envoydnsv3.DnsCluster
		err := cluster.GetClusterType().GetTypedConfig().UnmarshalTo(&dnsCluster)
		require.NoError(t, err)
		assert.Equal(t, durationpb.New(60*time.Second), dnsCluster.GetDnsRefreshRate())
		assert.Equal(t, durationpb.New(15*time.Second), dnsCluster.GetDnsJitter())
		assert.True(t, dnsCluster.GetRespectDnsTtl())
	})

	t.Run("applies dns settings when cluster is dns", func(t *testing.T) {
		policyIR, errs := translate(nil, nil, &kgateway.BackendConfigPolicy{
			Spec: kgateway.BackendConfigPolicySpec{
				DNS: &kgateway.DNS{
					RefreshRate: &metav1.Duration{Duration: 60 * time.Second},
					Jitter:      &metav1.Duration{Duration: 15 * time.Second},
					RespectTTL:  new(true),
				},
			},
		})
		require.Empty(t, errs)

		cluster := &envoyclusterv3.Cluster{
			ClusterDiscoveryType: &envoyclusterv3.Cluster_ClusterType{
				ClusterType: &envoyclusterv3.Cluster_CustomClusterType{
					Name:        dnsClusterExtensionName,
					TypedConfig: mustMessageToAny(t, &envoydnsv3.DnsCluster{}),
				},
			},
		}
		backend := ir.BackendObjectIR{
			Obj: &kgateway.Backend{
				Spec: kgateway.BackendSpec{
					Static: &kgateway.StaticBackend{
						Hosts: []kgateway.Host{{
							Host: "10.0.0.1",
							Port: 8080,
						}},
					},
				},
			},
		}

		processBackend(context.Background(), policyIR, backend, cluster)

		var dnsCluster envoydnsv3.DnsCluster
		err := cluster.GetClusterType().GetTypedConfig().UnmarshalTo(&dnsCluster)
		require.NoError(t, err)
		assert.Equal(t, durationpb.New(60*time.Second), dnsCluster.GetDnsRefreshRate())
		assert.Equal(t, durationpb.New(15*time.Second), dnsCluster.GetDnsJitter())
		assert.True(t, dnsCluster.GetRespectDnsTtl())
	})

	t.Run("ignores dns settings for non-dns clusters", func(t *testing.T) {
		policyIR, errs := translate(nil, nil, &kgateway.BackendConfigPolicy{
			Spec: kgateway.BackendConfigPolicySpec{
				DNS: &kgateway.DNS{
					RefreshRate: &metav1.Duration{Duration: 60 * time.Second},
					Jitter:      &metav1.Duration{Duration: 15 * time.Second},
					RespectTTL:  new(true),
				},
			},
		})
		require.Empty(t, errs)

		cluster := &envoyclusterv3.Cluster{
			ClusterDiscoveryType: &envoyclusterv3.Cluster_ClusterType{
				ClusterType: &envoyclusterv3.Cluster_CustomClusterType{
					Name:        "envoy.clusters.aggregate",
					TypedConfig: mustMessageToAny(t, &wrapperspb.StringValue{Value: "unchanged"}),
				},
			},
		}

		processBackend(context.Background(), policyIR, ir.BackendObjectIR{}, cluster)

		assert.Equal(t, "envoy.clusters.aggregate", cluster.GetClusterType().GetName())
		assert.True(t, proto.Equal(mustMessageToAny(t, &wrapperspb.StringValue{Value: "unchanged"}), cluster.GetClusterType().GetTypedConfig()))
	})
}

// mustMessageToAny is a helper function to handle MessageToAny error in test cases
func mustMessageToAny(t *testing.T, msg proto.Message) *anypb.Any {
	a, err := utils.MessageToAny(msg)
	require.NoError(t, err, "failed to convert message to Any")
	return a
}
