package proxy_syncer_test

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	. "github.com/kgateway-dev/kgateway/v2/internal/kgateway/proxy_syncer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

func mustAny(src proto.Message) *anypb.Any {
	a, e := utils.MessageToAny(src)
	if e != nil {
		panic(e)
	}
	return a
}

func TestRedacted(t *testing.T) {
	UseDetailedUnmarshalling = true
	g := gomega.NewWithT(t)
	c := &envoyclusterv3.Cluster{
		TransportSocket: &envoycorev3.TransportSocket{
			Name: "foo",
			ConfigType: &envoycorev3.TransportSocket_TypedConfig{
				TypedConfig: mustAny(&envoytlsv3.UpstreamTlsContext{
					CommonTlsContext: &envoytlsv3.CommonTlsContext{
						TlsCertificates: []*envoytlsv3.TlsCertificate{
							{
								PrivateKey: &envoycorev3.DataSource{
									Specifier: &envoycorev3.DataSource_InlineString{
										InlineString: "foo",
									},
								},
							},
						},
					},
				}),
			},
		},
	}
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Cluster] = envoycache.Resources{
		Version: "foo",
		Items:   map[string]envoycachetypes.ResourceWithTTL{"foo": envoycachetypes.ResourceWithTTL{Resource: c}},
	}

	x := XdsSnapWrapper{}.WithSnapshot(snap)
	data, err := x.MarshalJSON()

	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	expectedJson := `{"Snap":{"Clusters":{"foo":{"transportSocket":{"name":"foo","typedConfig":{"@type":"type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext","commonTlsContext":{"tlsCertificates":[{"privateKey":{"inlineString":"[REDACTED]"}}]}}}}}},"ProxyKey":""}`
	g.Expect(s).To(gomega.MatchJSON(expectedJson))
}
