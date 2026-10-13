package backendtlspolicy

import (
	"context"
	"errors"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/runtime/schema"

	translatorutils "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func backendWithBTP(t *testing.T, appProtocol ir.AppProtocol) ir.BackendObjectIR {
	t.Helper()
	return ir.BackendObjectIR{
		AppProtocol: appProtocol,
		AttachedPolicies: ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
			backendTlsPolicyGroupKind.GroupKind(): {{
				GroupKind: backendTlsPolicyGroupKind.GroupKind(),
				PolicyIr:  &backendTlsPolicy{transportSocket: tlsSocket(t, "btp.example.com")},
			}},
		}},
	}
}

func clusterWithSocket(ts *envoycorev3.TransportSocket) *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{Name: "test", TransportSocket: ts}
}

func setUpstream(t *testing.T, c *envoyclusterv3.Cluster, upo *envoy_upstreams_v3.HttpProtocolOptions) {
	t.Helper()
	require.NoError(t, translatorutils.MutateHttpOptions(c, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
		opts.CommonHttpProtocolOptions = upo.CommonHttpProtocolOptions
		opts.UpstreamProtocolOptions = upo.UpstreamProtocolOptions
	}))
}

func explicitHttp1(opts *envoycorev3.Http1ProtocolOptions) *envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_ {
	return &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
		ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
			ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{
				HttpProtocolOptions: opts,
			},
		},
	}
}

func readOptions(t *testing.T, c *envoyclusterv3.Cluster) *envoy_upstreams_v3.HttpProtocolOptions {
	t.Helper()
	opts, err := httpProtocolOptions(c)
	require.NoError(t, err)
	return opts
}

func TestProcessBaseCluster(t *testing.T) {
	tests := []struct {
		name     string
		backend  func(t *testing.T) ir.BackendObjectIR
		cluster  func(t *testing.T) *envoyclusterv3.Cluster
		wantAuto bool
	}{
		{
			name:     "TLS from BackendTLSPolicy with no app protocol negotiates via ALPN",
			backend:  func(t *testing.T) ir.BackendObjectIR { return backendWithBTP(t, ir.DefaultAppProtocol) },
			cluster:  func(t *testing.T) *envoyclusterv3.Cluster { return clusterWithSocket(tlsSocket(t, "btp.example.com")) },
			wantAuto: true,
		},
		{
			name:    "TLS wrapped in upstream proxy protocol negotiates via ALPN",
			backend: func(t *testing.T) ir.BackendObjectIR { return backendWithBTP(t, ir.DefaultAppProtocol) },
			cluster: func(t *testing.T) *envoyclusterv3.Cluster {
				c := clusterWithSocket(proxyProtocolWrappedRawBuffer(t))
				processBackend(context.Background(), &backendTlsPolicy{transportSocket: tlsSocket(t, "btp.example.com")}, ir.BackendObjectIR{}, c)
				return c
			},
			wantAuto: true,
		},
		{
			name:    "BackendConfigPolicy common options placeholder is replaced",
			backend: func(t *testing.T) ir.BackendObjectIR { return backendWithBTP(t, ir.DefaultAppProtocol) },
			cluster: func(t *testing.T) *envoyclusterv3.Cluster {
				c := clusterWithSocket(tlsSocket(t, "btp.example.com"))
				setUpstream(t, c, &envoy_upstreams_v3.HttpProtocolOptions{
					CommonHttpProtocolOptions: &envoycorev3.HttpProtocolOptions{IdleTimeout: durationpb.New(10 * time.Second)},
					UpstreamProtocolOptions:   explicitHttp1(nil),
				})
				return c
			},
			wantAuto: true,
		},
		{
			name:    "no BackendTLSPolicy attached",
			backend: func(t *testing.T) ir.BackendObjectIR { return ir.BackendObjectIR{} },
			cluster: func(t *testing.T) *envoyclusterv3.Cluster {
				return clusterWithSocket(tlsSocket(t, "other.example.com"))
			},
			wantAuto: false,
		},
		{
			name: "BackendTLSPolicy with errors",
			backend: func(t *testing.T) ir.BackendObjectIR {
				b := backendWithBTP(t, ir.DefaultAppProtocol)
				b.AttachedPolicies.Policies[backendTlsPolicyGroupKind.GroupKind()][0].Errors = []error{errors.New("bad")}
				return b
			},
			cluster:  func(t *testing.T) *envoyclusterv3.Cluster { return clusterWithSocket(tlsSocket(t, "btp.example.com")) },
			wantAuto: false,
		},
		{
			name:     "HTTP/2 app protocol stays explicit",
			backend:  func(t *testing.T) ir.BackendObjectIR { return backendWithBTP(t, ir.HTTP2AppProtocol) },
			cluster:  func(t *testing.T) *envoyclusterv3.Cluster { return clusterWithSocket(tlsSocket(t, "btp.example.com")) },
			wantAuto: false,
		},
		{
			name:     "WebSocket app protocol stays HTTP/1.1",
			backend:  func(t *testing.T) ir.BackendObjectIR { return backendWithBTP(t, ir.WebSocketAppProtocol) },
			cluster:  func(t *testing.T) *envoyclusterv3.Cluster { return clusterWithSocket(tlsSocket(t, "btp.example.com")) },
			wantAuto: false,
		},
		{
			name:    "transport socket matches (Istio auto mTLS) are left alone",
			backend: func(t *testing.T) ir.BackendObjectIR { return backendWithBTP(t, ir.DefaultAppProtocol) },
			cluster: func(t *testing.T) *envoyclusterv3.Cluster {
				c := clusterWithSocket(tlsSocket(t, "btp.example.com"))
				c.TransportSocketMatches = []*envoyclusterv3.Cluster_TransportSocketMatch{{Name: "plaintext"}}
				return c
			},
			wantAuto: false,
		},
		{
			name:     "non-TLS transport socket",
			backend:  func(t *testing.T) ir.BackendObjectIR { return backendWithBTP(t, ir.DefaultAppProtocol) },
			cluster:  func(t *testing.T) *envoyclusterv3.Cluster { return clusterWithSocket(proxyProtocolWrappedRawBuffer(t)) },
			wantAuto: false,
		},
		{
			name:    "BackendConfigPolicy http1ProtocolOptions pins HTTP/1.1",
			backend: func(t *testing.T) ir.BackendObjectIR { return backendWithBTP(t, ir.DefaultAppProtocol) },
			cluster: func(t *testing.T) *envoyclusterv3.Cluster {
				c := clusterWithSocket(tlsSocket(t, "btp.example.com"))
				setUpstream(t, c, &envoy_upstreams_v3.HttpProtocolOptions{UpstreamProtocolOptions: explicitHttp1(&envoycorev3.Http1ProtocolOptions{
					HeaderKeyFormat: &envoycorev3.Http1ProtocolOptions_HeaderKeyFormat{
						HeaderFormat: &envoycorev3.Http1ProtocolOptions_HeaderKeyFormat_ProperCaseWords_{},
					},
				})})
				return c
			},
			wantAuto: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.cluster(t)
			before := readOptions(t, c)

			processBaseCluster(nil, context.Background(), tt.backend(t), c)

			after := readOptions(t, c)
			if !tt.wantAuto {
				assert.Equal(t, before.GetUpstreamProtocolOptions(), after.GetUpstreamProtocolOptions(), "upstream protocol should be unchanged")
				return
			}
			auto := after.GetAutoConfig()
			require.NotNil(t, auto, "cluster should negotiate the upstream protocol via ALPN")
			assert.NotNil(t, auto.GetHttpProtocolOptions(), "auto config should allow HTTP/1.1")
			assert.NotNil(t, auto.GetHttp2ProtocolOptions(), "auto config should allow HTTP/2")
			assert.Equal(t, before.GetCommonHttpProtocolOptions().GetIdleTimeout(), after.GetCommonHttpProtocolOptions().GetIdleTimeout(),
				"common protocol options should be preserved")
		})
	}
}
