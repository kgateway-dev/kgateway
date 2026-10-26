package validator

import (
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
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

// fileTLSBootstrap returns a bootstrap whose cluster and static secret
// reference TLS files that exist only on the proxy.
func fileTLSBootstrap() *envoybootstrapv3.Bootstrap {
	return &envoybootstrapv3.Bootstrap{
		Node: &envoycorev3.Node{Id: "test-id", Cluster: "test-cluster"},
		StaticResources: &envoybootstrapv3.Bootstrap_StaticResources{
			Clusters: []*envoyclusterv3.Cluster{{
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
			}},
			Secrets: []*envoytlsv3.Secret{{
				Name: "file-ca",
				Type: &envoytlsv3.Secret_ValidationContext{
					ValidationContext: &envoytlsv3.CertificateValidationContext{
						TrustedCa: fileDataSource("/proxy/only/ca.crt"),
					},
				},
			}},
		},
	}
}

func TestReplaceTLSFiles(t *testing.T) {
	t.Run("replaces files in Any and static secrets", func(t *testing.T) {
		bootstrap := fileTLSBootstrap()
		require.True(t, replaceTLSFiles(bootstrap.ProtoReflect()))

		upstream := &envoytlsv3.UpstreamTlsContext{}
		require.NoError(t, bootstrap.GetStaticResources().GetClusters()[0].GetTransportSocket().GetTypedConfig().UnmarshalTo(upstream))
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
			bootstrap.GetStaticResources().GetSecrets()[0].GetValidationContext().GetTrustedCa(),
			protocmp.Transform(),
		))
	})

	t.Run("replaces an inline key paired with a file cert", func(t *testing.T) {
		cert := &envoytlsv3.TlsCertificate{
			CertificateChain: fileDataSource("/proxy/only/tls.crt"),
			PrivateKey:       inlineDataSource("inline key"),
		}
		require.True(t, replaceTLSFiles(cert.ProtoReflect()))
		assert.Equal(t, placeholderKey, cert.GetPrivateKey().GetInlineString(), "key must match the placeholder cert")
	})

	t.Run("leaves inline data alone", func(t *testing.T) {
		bootstrap := fileTLSBootstrap()
		require.True(t, replaceTLSFiles(bootstrap.ProtoReflect()))
		before := proto.Clone(bootstrap)
		assert.False(t, replaceTLSFiles(bootstrap.ProtoReflect()), "inline data sources should not change")
		assert.True(t, proto.Equal(before, bootstrap))
	})
}

func TestPrepareBootstrapConfigLeavesInputUnchanged(t *testing.T) {
	bootstrap := fileTLSBootstrap()
	before := proto.Clone(bootstrap)
	_, err := prepareBootstrapConfig(bootstrap)
	require.NoError(t, err)
	assert.True(t, proto.Equal(before, bootstrap), "file paths must still reach the proxy")
}
