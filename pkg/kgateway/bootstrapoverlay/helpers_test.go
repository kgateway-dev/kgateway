package bootstrapoverlay

import (
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// generatedBase is a trimmed generated bootstrap with each managed path in it.
const generatedBase = `
admin:
  address: {socket_address: {address: 127.0.0.1, port_value: 19000}}
layered_runtime:
  layers:
  - {name: admin_layer, admin_layer: {}}
node:
  cluster: gw.default
  metadata: {role: kgateway-kube-gateway-api~default~gw}
cluster_manager: {local_cluster_name: gw.default}
stats_config:
  stats_matcher:
    inclusion_list:
      patterns: [{prefix: cluster.}]
typed_dns_resolver_config:
  name: envoy.network.dns_resolver.cares
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.network.dns_resolver.cares.v3.CaresDnsResolverConfig
    udp_max_queries: 100
static_resources:
  listeners:
  - name: readiness_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 8082}}
  - name: prometheus_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 9091}}
  clusters:
  - {name: gw.default, type: EDS}
  - {name: xds_cluster, connect_timeout: 5s}
  - {name: admin_port_cluster, connect_timeout: 5s}
dynamic_resources:
  cds_config: {ads: {}}
`

var (
	gcParams = Source{Namespace: "kgateway-system", Name: "class-params"}
	gwParams = Source{Namespace: "default", Name: "gw-params"}
)

// overlay converts a YAML overlay to the JSON GatewayParameters stores.
func overlay(t *testing.T, source Source, y string) Overlay {
	t.Helper()
	raw, err := yaml.YAMLToJSON([]byte(y))
	require.NoError(t, err)
	return Overlay{Source: source, Raw: raw}
}

func parseBase(t *testing.T) *envoybootstrapv3.Bootstrap {
	t.Helper()
	return parseYAMLBootstrap(t, generatedBase)
}

func parseYAMLBootstrap(t *testing.T, y string) *envoybootstrapv3.Bootstrap {
	t.Helper()
	bs, err := ParseBootstrap([]byte(y))
	require.NoError(t, err)
	return bs
}

func names[T namedEntry](entries []T) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.GetName())
	}
	return out
}

func prefixes(patterns []*envoymatcherv3.StringMatcher) []string {
	var out []string
	for _, p := range patterns {
		out = append(out, p.GetPrefix())
	}
	return out
}
