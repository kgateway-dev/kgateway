package bootstrap

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

func fileDataSource(path string) *envoycorev3.DataSource {
	return &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_Filename{Filename: path}}
}

// fileTLSCluster returns a cluster that references TLS files that exist only
// on the proxy.
func fileTLSCluster() *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{
		Name:                 "file-tls",
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC},
		TransportSocket: &envoycorev3.TransportSocket{
			Name: "envoy.transport_sockets.tls",
			ConfigType: &envoycorev3.TransportSocket_TypedConfig{
				TypedConfig: utils.MustMessageToAny(&envoytlsv3.UpstreamTlsContext{
					CommonTlsContext: &envoytlsv3.CommonTlsContext{
						TlsCertificates: []*envoytlsv3.TlsCertificate{{
							CertificateChain: fileDataSource("/proxy/only/tls.crt"),
							PrivateKey:       fileDataSource("/proxy/only/tls.key"),
						}},
						ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{
							ValidationContext: &envoytlsv3.CertificateValidationContext{
								TrustedCa: fileDataSource("/proxy/only/ca.crt"),
							},
						},
					},
				}),
			},
		},
	}
}

func TestBuildReplacesTLSFiles(t *testing.T) {
	cluster := fileTLSCluster()
	secret := &envoytlsv3.Secret{
		Name: "file-ca",
		Type: &envoytlsv3.Secret_ValidationContext{
			ValidationContext: &envoytlsv3.CertificateValidationContext{
				TrustedCa: fileDataSource("/proxy/only/ca.crt"),
			},
		},
	}
	clusterBefore := proto.Clone(cluster)
	secretBefore := proto.Clone(secret)

	builder := New()
	builder.AddCluster(cluster)
	builder.AddSecret(secret)
	bs, err := builder.Build()
	require.NoError(t, err)

	upstream := &envoytlsv3.UpstreamTlsContext{}
	require.NoError(t, ValidatedClusters(bs)[0].GetTransportSocket().GetTypedConfig().UnmarshalTo(upstream))
	expected := &envoytlsv3.UpstreamTlsContext{
		CommonTlsContext: &envoytlsv3.CommonTlsContext{
			TlsCertificates: []*envoytlsv3.TlsCertificate{{
				CertificateChain: inlineDataSource(placeholderCert),
				PrivateKey:       inlineDataSource(placeholderKey),
			}},
			ValidationContextType: &envoytlsv3.CommonTlsContext_ValidationContext{
				ValidationContext: &envoytlsv3.CertificateValidationContext{
					TrustedCa: inlineDataSource(placeholderCert),
				},
			},
		},
	}
	assert.Empty(t, cmp.Diff(expected, upstream, protocmp.Transform()))
	assert.Empty(t, cmp.Diff(
		inlineDataSource(placeholderCert),
		bs.GetStaticResources().GetSecrets()[0].GetValidationContext().GetTrustedCa(),
		protocmp.Transform(),
	))

	assert.True(t, proto.Equal(clusterBefore, cluster), "file paths must still reach the proxy")
	assert.True(t, proto.Equal(secretBefore, secret), "file paths must still reach the proxy")
}

func TestReplaceTLSFiles(t *testing.T) {
	t.Run("replaces an inline key paired with a file cert", func(t *testing.T) {
		cert := &envoytlsv3.TlsCertificate{
			CertificateChain: fileDataSource("/proxy/only/tls.crt"),
			PrivateKey:       inlineDataSource("inline key"),
		}
		require.True(t, replaceTLSFiles(cert.ProtoReflect()))
		assert.Equal(t, placeholderKey, cert.GetPrivateKey().GetInlineString(), "key must match the placeholder cert")
	})

	t.Run("leaves inline data alone", func(t *testing.T) {
		cluster := fileTLSCluster()
		require.True(t, replaceTLSFiles(cluster.ProtoReflect()))
		before := proto.Clone(cluster)
		assert.False(t, replaceTLSFiles(cluster.ProtoReflect()), "inline data sources should not change")
		assert.True(t, proto.Equal(before, cluster))
	})
}
