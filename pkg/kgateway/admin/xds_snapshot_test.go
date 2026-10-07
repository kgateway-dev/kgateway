package admin

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
)

func TestRedactSecrets(t *testing.T) {
	testCases := []struct {
		name string
		in   *cache.Snapshot
		want *cache.Snapshot
	}{
		{
			name: "nil snapshot",
			in:   nil,
			want: nil,
		},
		{
			name: "secret data is redacted",
			in: &cache.Snapshot{
				// Index by the named response-type constants: their numeric
				// values shift whenever go-control-plane adds a type.
				Resources: [types.UnknownType]cache.Resources{
					types.Cluster: {
						Version: "cluster1",
						Items: map[string]types.ResourceWithTTL{
							"cluster1": {
								Resource: &envoyclusterv3.Cluster{
									Name: "cluster1",
								},
							},
						},
					},
					types.Endpoint: {
						Version: "endpoint1",
						Items: map[string]types.ResourceWithTTL{
							"endpoint1": {
								Resource: &envoyendpointv3.ClusterLoadAssignment{
									ClusterName: "cluster1",
								},
							},
						},
					},
					types.Listener: {
						Version: "listener",
					},
					types.Route: {
						Version: "route",
					},
					types.ScopedRoute: {
						Version: "scopedroute",
					},
					types.VirtualHost: {
						Version: "virtualhost",
					},
					types.Secret: {
						Version: "secret1",
						Items: map[string]types.ResourceWithTTL{
							"secret-foo": {
								Resource: &envoytlsv3.Secret{
									Name: "secret-foo",
									Type: &envoytlsv3.Secret_GenericSecret{
										GenericSecret: &envoytlsv3.GenericSecret{
											Secret: &envoycorev3.DataSource{
												Specifier: &envoycorev3.DataSource_InlineBytes{
													InlineBytes: []byte("secret-data"),
												},
											},
										},
									},
								},
							},
							"secret-bar": {
								Resource: &envoytlsv3.Secret{
									Name: "secret-bar",
									Type: &envoytlsv3.Secret_GenericSecret{
										GenericSecret: &envoytlsv3.GenericSecret{
											Secret: &envoycorev3.DataSource{
												Specifier: &envoycorev3.DataSource_InlineString{
													InlineString: "secret-data",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: &cache.Snapshot{
				Resources: [types.UnknownType]cache.Resources{
					types.Cluster: {
						Version: "cluster1",
						Items: map[string]types.ResourceWithTTL{
							"cluster1": {
								Resource: &envoyclusterv3.Cluster{
									Name: "cluster1",
								},
							},
						},
					},
					types.Endpoint: {
						Version: "endpoint1",
						Items: map[string]types.ResourceWithTTL{
							"endpoint1": {
								Resource: &envoyendpointv3.ClusterLoadAssignment{
									ClusterName: "cluster1",
								},
							},
						},
					},
					types.Listener: {
						Version: "listener",
					},
					types.Route: {
						Version: "route",
					},
					types.ScopedRoute: {
						Version: "scopedroute",
					},
					types.VirtualHost: {
						Version: "virtualhost",
					},
					types.Secret: {
						Version: "secret1",
						Items: map[string]types.ResourceWithTTL{
							"secret-foo": {
								Resource: &envoytlsv3.Secret{
									Name: "secret-foo",
								},
							},
							"secret-bar": {
								Resource: &envoytlsv3.Secret{
									Name: "secret-bar",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			got := redactSecrets(tc.in)
			diff := cmp.Diff(tc.want, got, protocmp.Transform())
			r.Empty(diff)
		})
	}
}

// tunnelListenerSnapshot builds a snapshot with a CONNECT authorization header.
func tunnelListenerSnapshot(t *testing.T, authorization string) *cache.Snapshot {
	t.Helper()
	tcpProxy, err := utils.MessageToAny(&envoytcp.TcpProxy{
		StatPrefix:       "connect_tunnel_test",
		ClusterSpecifier: &envoytcp.TcpProxy_Cluster{Cluster: "kube_default_egress-proxy_3128"},
		TunnelingConfig: &envoytcp.TcpProxy_TunnelingConfig{
			Hostname: "external.example.com:443",
			HeadersToAdd: []*envoycorev3.HeaderValueOption{{
				Header:       &envoycorev3.HeaderValue{Key: "Proxy-Authorization", Value: authorization},
				AppendAction: envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			}},
		},
	})
	require.NoError(t, err)
	listener := &envoylistenerv3.Listener{
		Name: "connect_tunnel_test",
		FilterChains: []*envoylistenerv3.FilterChain{{
			Filters: []*envoylistenerv3.Filter{{
				Name:       envoywellknown.TCPProxy,
				ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: tcpProxy},
			}},
		}},
	}
	return &cache.Snapshot{
		Resources: [types.UnknownType]cache.Resources{
			types.Listener: {
				Version: "listeners",
				Items:   map[string]types.ResourceWithTTL{listener.Name: {Resource: listener}},
			},
		},
	}
}

func TestRedactListenerCredentialsInSnapshot(t *testing.T) {
	r := require.New(t)
	in := tunnelListenerSnapshot(t, "Basic c2VjcmV0")

	got := redactListenerCredentials(in)

	r.Empty(cmp.Diff(tunnelListenerSnapshot(t, xds.RedactedValue), got, protocmp.Transform()), "CONNECT header values must be redacted")
	r.Empty(cmp.Diff(tunnelListenerSnapshot(t, "Basic c2VjcmV0"), in, protocmp.Transform()), "the cached snapshot must not be modified")
}
