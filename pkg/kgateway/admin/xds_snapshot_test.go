package admin

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
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
			name: "secret data isredacted",
			in: &cache.Snapshot{
				Resources: [types.UnknownType]cache.Resources{
					{
						Version: "cluster1",
						Items: map[string]types.ResourceWithTTL{
							"cluster1": {
								Resource: &envoyclusterv3.Cluster{
									Name: "cluster1",
								},
							},
						},
					},
					{
						Version: "endpoint1",
						Items: map[string]types.ResourceWithTTL{
							"endpoint1": {
								Resource: &envoyendpointv3.ClusterLoadAssignment{
									ClusterName: "cluster1",
								},
							},
						},
					},
					{
						Version: "listener",
					},
					{
						Version: "route",
					},
					{
						Version: "scopedroute",
					},
					{
						Version: "virtualhost",
					},
					{
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
					{
						Version: "cluster1",
						Items: map[string]types.ResourceWithTTL{
							"cluster1": {
								Resource: &envoyclusterv3.Cluster{
									Name: "cluster1",
								},
							},
						},
					},
					{
						Version: "endpoint1",
						Items: map[string]types.ResourceWithTTL{
							"endpoint1": {
								Resource: &envoyendpointv3.ClusterLoadAssignment{
									ClusterName: "cluster1",
								},
							},
						},
					},
					{
						Version: "listener",
					},
					{
						Version: "route",
					},
					{
						Version: "scopedroute",
					},
					{
						Version: "virtualhost",
					},
					{
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
