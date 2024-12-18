package proxy_syncer_test

import (
	"testing"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func mustAny(src proto.Message) *anypb.Any {
	a, e := anypb.New(src)
	if e != nil {
		panic(e)
	}
	return a
}

func TestRedacted(t *testing.T) {
	UseDetailedUnmarshalling = true
	g := gomega.NewWithT(t)
	c := resource.NewEnvoyResource(&envoy_config_cluster_v3.Cluster{
		TransportSocket: &corev3.TransportSocket{
			Name: "foo",
			ConfigType: &corev3.TransportSocket_TypedConfig{
				TypedConfig: mustAny(&envoyauth.UpstreamTlsContext{
					CommonTlsContext: &envoyauth.CommonTlsContext{
						TlsCertificates: []*envoyauth.TlsCertificate{
							{
								PrivateKey: &corev3.DataSource{
									Specifier: &corev3.DataSource_InlineString{
										InlineString: "foo",
									},
								},
							},
						},
					},
				}),
			},
		},
	})
	x := XdsSnapWrapper{}.WithSnapshot(&xds.EnvoySnapshot{
		Clusters: cache.Resources{
			Version: "foo",
			Items:   map[string]cache.Resource{"foo": c},
		},
	},
	)
	data, err := x.MarshalJSON()

	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	expectedJson := `{"Snap":{"Clusters":{"foo":{"transportSocket":{"name":"foo","typedConfig":{"@type":"type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext","commonTlsContext":{"tlsCertificates":[{"privateKey":{"inlineString":"[REDACTED]"}}]}}}}}},"ProxyKey":""}`
	g.Expect(s).To(gomega.MatchJSON(expectedJson))
}
