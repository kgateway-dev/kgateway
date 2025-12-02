package proxy_syncer_test

import (
	"strings"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/onsi/gomega"
	"google.golang.org/protobuf/encoding/protojson"
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
		Items:   map[string]envoycachetypes.ResourceWithTTL{"foo": {Resource: c}},
	}

	x := XdsSnapWrapper{}.WithSnapshot(snap)
	data, err := x.MarshalJSON()

	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	expectedJson := `{"Snap":{"Clusters":{"foo":{"transport_socket":{"name":"foo","typed_config":{"@type":"type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext","common_tls_context":{"tls_certificates":[{"private_key":{"inline_string":"[REDACTED]"}}]}}}}}},"ProxyKey":""}`
	g.Expect(s).To(gomega.MatchJSON(expectedJson))
}

func TestMapOfAny(t *testing.T) {
	UseDetailedUnmarshalling = true
	g := gomega.NewWithT(t)
	testCase := `{
      "name": "kube_kgateway-system_rate-limiter-kgateway_8083",
      "type": "EDS",
      "eds_cluster_config": {
       "eds_config": {
        "ads": {},
        "resource_api_version": "V3"
       }
      },
      "connect_timeout": "5s",
      "metadata": {},
      "ignore_health_on_host_removal": true,
      "typed_extension_protocol_options": {
       "envoy.extensions.upstreams.http.v3.HttpProtocolOptions": {
        "@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
        "explicit_http_config": {
         "http2_protocol_options": {}
        }
       }
      }
     }`
	s := redactCluster(t, testCase)

	expectedJson := `{"Snap":{"Clusters":{"foo":` + testCase + `}},"ProxyKey":""}`
	g.Expect(s).To(gomega.MatchJSON(expectedJson))
}

func TestRedactMapOfAny(t *testing.T) {
	UseDetailedUnmarshalling = true
	g := gomega.NewWithT(t)
	// this is not valid envoy config - just for testing
	testCase := `{
      "name": "kube_kgateway-system_rate-limiter-kgateway_8083",
      "type": "EDS",
      "eds_cluster_config": {
       "eds_config": {
        "ads": {},
        "resource_api_version": "V3"
       }
      },
      "typed_extension_protocol_options": {
       "secret": {
        "@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext",
		"common_tls_context":{"tls_certificates":[{"private_key":{"inline_string":"secret!"}}]}
       }
      }
     }`
	s := redactCluster(t, testCase)

	expectedJson := `{"Snap":{"Clusters":{"foo":` + strings.Replace(testCase, "secret!", "[REDACTED]", -1) + `}},"ProxyKey":""}`
	g.Expect(s).To(gomega.MatchJSON(expectedJson))
}

func redactCluster(t *testing.T, testCase string) string {
	var c envoyclusterv3.Cluster
	var j protojson.UnmarshalOptions
	err := j.Unmarshal([]byte(testCase), &c)

	if err != nil {
		t.Fatal(err)
	}
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Cluster] = envoycache.Resources{
		Version: "foo",
		Items:   map[string]envoycachetypes.ResourceWithTTL{"foo": {Resource: &c}},
	}

	x := XdsSnapWrapper{}.WithSnapshot(snap)
	data, err := x.MarshalJSON()

	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
