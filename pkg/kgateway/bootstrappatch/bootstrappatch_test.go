package bootstrappatch

import (
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycaresv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/network/dns_resolver/cares/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
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
	gcParams = types.NamespacedName{Namespace: "kgateway-system", Name: "class-params"}
	gwParams = types.NamespacedName{Namespace: "default", Name: "gw-params"}
)

// overlay converts a YAML overlay to the JSON GatewayParameters stores.
func overlay(t *testing.T, source types.NamespacedName, y string) Overlay {
	t.Helper()
	raw, err := yaml.YAMLToJSON([]byte(y))
	require.NoError(t, err)
	return Overlay{Source: source, Raw: raw}
}

func parseBase(t *testing.T) *envoybootstrapv3.Bootstrap {
	t.Helper()
	bs, err := ParseBootstrap([]byte(generatedBase))
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

func TestApplyMergeSemantics(t *testing.T) {
	tests := []struct {
		name    string
		overlay string
		check   func(t *testing.T, merged *envoybootstrapv3.Bootstrap)
	}{
		{
			name:    "a scalar replaces the base value and unset fields keep theirs",
			overlay: `{admin: {address: {socket_address: {port_value: 19001}}}}`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				sa := merged.GetAdmin().GetAddress().GetSocketAddress()
				assert.Equal(t, uint32(19001), sa.GetPortValue(), "overlay scalar should replace the base value")
				assert.Equal(t, "127.0.0.1", sa.GetAddress(), "field the overlay leaves unset should keep the base value")
			},
		},
		{
			name:    "a message merges field by field",
			overlay: `{stats_flush_interval: 10s, stats_config: {use_all_default_tags: true}}`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				assert.Equal(t, int64(10), merged.GetStatsFlushInterval().GetSeconds())
				assert.True(t, merged.GetStatsConfig().GetUseAllDefaultTags().GetValue())
				assert.NotNil(t, merged.GetStatsConfig().GetStatsMatcher().GetInclusionList(), "base stats matcher should survive")
			},
		},
		{
			name: "an Any replaces the base value whole",
			overlay: `
typed_dns_resolver_config:
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.network.dns_resolver.cares.v3.CaresDnsResolverConfig
    use_resolvers_as_fallback: true`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				cares := &envoycaresv3.CaresDnsResolverConfig{}
				require.NoError(t, merged.GetTypedDnsResolverConfig().GetTypedConfig().UnmarshalTo(cares))
				assert.True(t, cares.GetUseResolversAsFallback())
				assert.Nil(t, cares.GetUdpMaxQueries(), "the base Any's udp_max_queries should be gone")
				assert.Equal(t, "envoy.network.dns_resolver.cares", merged.GetTypedDnsResolverConfig().GetName())
			},
		},
		{
			name:    "setting a different oneof option clears the base option",
			overlay: `{stats_config: {stats_matcher: {reject_all: true}}}`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				assert.True(t, merged.GetStatsConfig().GetStatsMatcher().GetRejectAll())
				assert.Nil(t, merged.GetStatsConfig().GetStatsMatcher().GetInclusionList())
			},
		},
		{
			name: "a named cluster replaces the base entry whole, in place",
			overlay: `
static_resources:
  clusters:
  - {name: admin_port_cluster, connect_timeout: 1s}
  - {name: user_cluster}`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				clusters := merged.GetStaticResources().GetClusters()
				assert.Equal(t, []string{"gw.default", "xds_cluster", "admin_port_cluster", "user_cluster"}, names(clusters))
				assert.Equal(t, int64(1), clusters[2].GetConnectTimeout().GetSeconds())
			},
		},
		{
			name: "named listeners, secrets and resource monitors merge by name",
			overlay: `
static_resources:
  listeners:
  - name: readiness_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 8083}}
  secrets:
  - {name: user_secret}
overload_manager:
  resource_monitors:
  - name: envoy.resource_monitors.global_downstream_max_connections
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.resource_monitors.downstream_connections.v3.DownstreamConnectionsConfig
      max_active_downstream_connections: 5`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				listeners := merged.GetStaticResources().GetListeners()
				assert.Equal(t, []string{"readiness_listener", "prometheus_listener"}, names(listeners))
				assert.Equal(t, uint32(8083), listeners[0].GetAddress().GetSocketAddress().GetPortValue())
				assert.Equal(t, []string{"user_secret"}, names(merged.GetStaticResources().GetSecrets()))
				assert.Len(t, merged.GetOverloadManager().GetResourceMonitors(), 1)
			},
		},
		{
			name: "runtime layers go before the admin layer",
			overlay: `
layered_runtime:
  layers:
  - {name: user_a, static_layer: {a: 1}}
  - {name: user_b, static_layer: {b: 2}}`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				assert.Equal(t, []string{"user_a", "user_b", "admin_layer"}, names(merged.GetLayeredRuntime().GetLayers()))
			},
		},
		{
			name:    "other lists put the overlay's entries first",
			overlay: `{stats_config: {stats_matcher: {inclusion_list: {patterns: [{prefix: http.}, {prefix: listener.}]}}}}`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				patterns := merged.GetStatsConfig().GetStatsMatcher().GetInclusionList().GetPatterns()
				assert.Equal(t, []string{"http.", "listener.", "cluster."}, prefixes(patterns))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply the overlay to the generated bootstrap.
			base := parseBase(t)
			baseBefore := base.String()
			res, err := Apply(base, []Overlay{overlay(t, gwParams, tt.overlay)}, apisettings.BootstrapPatchManagedPathsWarn)
			require.NoError(t, err)

			// Check the merge, and that the base was copied rather than changed.
			tt.check(t, res.Bootstrap)
			assert.Equal(t, baseBefore, base.String(), "Apply should not modify the base")
		})
	}
}

func TestApplyGatewayClassThenGateway(t *testing.T) {
	// Apply a GatewayClass overlay, then a Gateway overlay that conflicts with it.
	res, err := Apply(parseBase(t), []Overlay{
		overlay(t, gcParams, `
stats_flush_interval: 10s
stats_config:
  stats_matcher: {inclusion_list: {patterns: [{prefix: class.}]}}
static_resources:
  clusters:
  - {name: shared, connect_timeout: 1s}
layered_runtime:
  layers:
  - {name: class_runtime, static_layer: {a: 1}}`),
		overlay(t, gwParams, `
stats_flush_interval: 20s
stats_config:
  stats_matcher: {inclusion_list: {patterns: [{prefix: gateway.}]}}
static_resources:
  clusters:
  - {name: shared, connect_timeout: 2s}
layered_runtime:
  layers:
  - {name: gateway_runtime, static_layer: {a: 2}}`),
	}, apisettings.BootstrapPatchManagedPathsWarn)
	require.NoError(t, err)

	// The Gateway's value wins each conflict.
	merged := res.Bootstrap
	assert.Equal(t, int64(20), merged.GetStatsFlushInterval().GetSeconds())
	shared := merged.GetStaticResources().GetClusters()[3]
	assert.Equal(t, "shared", shared.GetName())
	assert.Equal(t, int64(2), shared.GetConnectTimeout().GetSeconds())
	assert.Len(t, merged.GetStaticResources().GetClusters(), 4, "the Gateway's cluster should replace the GatewayClass's")

	// A list holds the Gateway's entries, then the GatewayClass's, then the base's.
	patterns := merged.GetStatsConfig().GetStatsMatcher().GetInclusionList().GetPatterns()
	assert.Equal(t, []string{"gateway.", "class.", "cluster."}, prefixes(patterns))

	// The Gateway layer comes after the GatewayClass layer, so it wins per key.
	assert.Equal(t, []string{"class_runtime", "gateway_runtime", "admin_layer"}, names(merged.GetLayeredRuntime().GetLayers()))
}

func TestApplyInvalidOverlay(t *testing.T) {
	tests := []struct {
		name     string
		overlays []Overlay
		wantErr  string
	}{
		{
			name:     "unknown field",
			overlays: []Overlay{{Source: gwParams, Raw: []byte(`{"layerd_runtime": {}}`)}},
			wantErr:  `bootstrap overlay in GatewayParameters default/gw-params: does not parse as an Envoy Bootstrap: proto:`,
		},
		{
			name: "type the controller does not link",
			overlays: []Overlay{{Source: gwParams, Raw: []byte(
				`{"bootstrap_extensions": [{"name": "x", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.unknown.v3.Unknown"}}]}`)}},
			wantErr: "envoy.extensions.unknown.v3.Unknown",
		},
		{
			name:     "template delimiter",
			overlays: []Overlay{{Source: gwParams, Raw: []byte(`{"node": {"id": "{{.PodName}}"}}`)}},
			wantErr:  `contains "{{"`,
		},
		{
			name:     "template delimiter hidden by a JSON escape",
			overlays: []Overlay{{Source: gwParams, Raw: []byte(`{"node": {"id": "{{.PodName}}"}}`)}},
			wantErr:  `contains "{{"`,
		},
		{
			name:     "system CA secret",
			overlays: []Overlay{overlay(t, gwParams, `{static_resources: {secrets: [{name: SYSTEM_CA_CERT}]}}`)},
			wantErr:  `static secret "SYSTEM_CA_CERT" is reserved`,
		},
		{
			name:     "admin layer",
			overlays: []Overlay{overlay(t, gwParams, `{layered_runtime: {layers: [{name: mine, admin_layer: {}}]}}`)},
			wantErr:  `runtime layer "mine" is an admin layer`,
		},
		{
			name:     "layer named like a generated layer",
			overlays: []Overlay{overlay(t, gwParams, `{layered_runtime: {layers: [{name: admin_layer, static_layer: {}}]}}`)},
			wantErr:  `runtime layer "admin_layer" has the name of a layer in the generated bootstrap`,
		},
		{
			name:     "layer name repeated in one overlay",
			overlays: []Overlay{overlay(t, gwParams, `{layered_runtime: {layers: [{name: a, static_layer: {}}, {name: a, static_layer: {}}]}}`)},
			wantErr:  `layered_runtime.layers has more than one entry named "a"`,
		},
		{
			name: "layer name used by the GatewayClass and the Gateway",
			overlays: []Overlay{
				overlay(t, gcParams, `{layered_runtime: {layers: [{name: user, static_layer: {}}]}}`),
				overlay(t, gwParams, `{layered_runtime: {layers: [{name: user, static_layer: {}}]}}`),
			},
			wantErr: `bootstrap overlay in GatewayParameters default/gw-params: runtime layer "user" is also defined in GatewayParameters kgateway-system/class-params`,
		},
		{
			name:     "cluster name repeated in one overlay",
			overlays: []Overlay{overlay(t, gwParams, `{static_resources: {clusters: [{name: c}, {name: c}]}}`)},
			wantErr:  `static_resources.clusters has more than one entry named "c"`,
		},
		{
			name:     "RTDS layer through ADS",
			overlays: []Overlay{overlay(t, gwParams, `{layered_runtime: {layers: [{name: rt, rtds_layer: {name: rt, rtds_config: {ads: {}}}}]}}`)},
			wantErr:  `RTDS runtime layer "rt" requests its values from the kgateway control plane through ADS`,
		},
		{
			name: "RTDS layer through the xds_cluster over gRPC",
			overlays: []Overlay{overlay(t, gwParams, `
layered_runtime:
  layers:
  - name: rt
    rtds_layer:
      name: rt
      rtds_config:
        api_config_source:
          api_type: GRPC
          grpc_services: [{envoy_grpc: {cluster_name: xds_cluster}}]
`)},
			wantErr: `RTDS runtime layer "rt" requests its values from the kgateway control plane through the xds_cluster cluster`,
		},
		{
			name: "RTDS layer through the xds_cluster over REST",
			overlays: []Overlay{overlay(t, gwParams, `
layered_runtime:
  layers:
  - name: rt
    rtds_layer:
      name: rt
      rtds_config:
        api_config_source: {api_type: REST, cluster_names: [xds_cluster]}
`)},
			wantErr: `RTDS runtime layer "rt" requests its values from the kgateway control plane through the xds_cluster cluster`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply under every mode: an invalid overlay is rejected regardless.
			for _, mode := range []apisettings.BootstrapPatchManagedPathsMode{
				apisettings.BootstrapPatchManagedPathsWarn,
				apisettings.BootstrapPatchManagedPathsPrune,
				apisettings.BootstrapPatchManagedPathsReject,
			} {
				res, err := Apply(parseBase(t), tt.overlays, mode)

				// Check the error type and message.
				var invalid *InvalidOverlayError
				require.ErrorAs(t, err, &invalid, "mode %s", mode)
				assert.ErrorContains(t, err, tt.wantErr, "mode %s", mode)
				assert.Nil(t, res)
			}
		})
	}
}

func TestApplyManagedPaths(t *testing.T) {
	// One overlay sets every managed path, spelled in lowerCamelCase where it differs,
	// plus a setting that is not managed.
	classOverlay := `
node: {locality: {zone: z1}}
dynamicResources: {lds_config: {ads: {}}}
admin: {address: {socket_address: {port_value: 19001}}}
clusterManager: {localClusterName: other}
statsFlushInterval: 10s
staticResources:
  clusters:
  - {name: xds_cluster}
  - {name: admin_port_cluster}
  - {name: gw.default}
  - {name: gateway_proxy_sds}
  listeners:
  - {name: readiness_listener}
  - {name: prometheus_listener}`
	gatewayOverlay := `{static_resources: {clusters: [{name: xds_cluster, connect_timeout: 9s}]}}`
	wantPaths := []ManagedPath{
		{Path: "node", Source: gcParams},
		{Path: "dynamic_resources", Source: gcParams},
		{Path: "admin.address", Source: gcParams},
		{Path: "cluster_manager.local_cluster_name", Source: gcParams},
		{Path: "static_resources.clusters[name=xds_cluster]", Source: gcParams},
		{Path: "static_resources.clusters[name=admin_port_cluster]", Source: gcParams},
		{Path: "static_resources.clusters[name=gw.default]", Source: gcParams},
		{Path: "static_resources.listeners[name=readiness_listener]", Source: gcParams},
		{Path: "static_resources.listeners[name=prometheus_listener]", Source: gcParams},
		{Path: "static_resources.clusters[name=xds_cluster]", Source: gwParams},
	}
	overlays := func(t *testing.T) []Overlay {
		return []Overlay{overlay(t, gcParams, classOverlay), overlay(t, gwParams, gatewayOverlay)}
	}

	t.Run("WARN applies and reports each path", func(t *testing.T) {
		// Apply under WARN.
		res, err := Apply(parseBase(t), overlays(t), apisettings.BootstrapPatchManagedPathsWarn)
		require.NoError(t, err)

		// Every path is reported, gateway_proxy_sds is not since the base lacks it,
		// and the overlays applied.
		assert.Equal(t, wantPaths, res.ManagedPaths)
		assert.Equal(t, "other", res.Bootstrap.GetClusterManager().GetLocalClusterName())
		assert.Equal(t, "z1", res.Bootstrap.GetNode().GetLocality().GetZone())
		assert.Equal(t, int64(9), res.Bootstrap.GetStaticResources().GetClusters()[1].GetConnectTimeout().GetSeconds())
		assert.Contains(t, names(res.Bootstrap.GetStaticResources().GetClusters()), "gateway_proxy_sds")
	})

	t.Run("PRUNE drops and reports each path and applies the rest", func(t *testing.T) {
		// Apply under PRUNE.
		base := parseBase(t)
		res, err := Apply(base, overlays(t), apisettings.BootstrapPatchManagedPathsPrune)
		require.NoError(t, err)

		// Every managed path keeps its generated value; the rest applied.
		assert.Equal(t, wantPaths, res.ManagedPaths)
		merged := res.Bootstrap
		assert.Equal(t, base.GetNode().String(), merged.GetNode().String())
		assert.Equal(t, base.GetDynamicResources().String(), merged.GetDynamicResources().String())
		assert.Equal(t, base.GetAdmin().String(), merged.GetAdmin().String())
		assert.Equal(t, "gw.default", merged.GetClusterManager().GetLocalClusterName())
		assert.Equal(t, int64(5), merged.GetStaticResources().GetClusters()[1].GetConnectTimeout().GetSeconds())
		assert.Equal(t, base.GetStaticResources().GetListeners()[0].String(), merged.GetStaticResources().GetListeners()[0].String())
		assert.Equal(t, int64(10), merged.GetStatsFlushInterval().GetSeconds())
		assert.Contains(t, names(merged.GetStaticResources().GetClusters()), "gateway_proxy_sds")
	})

	t.Run("REJECT applies nothing and reports every overlay's paths", func(t *testing.T) {
		// Apply under REJECT.
		res, err := Apply(parseBase(t), overlays(t), apisettings.BootstrapPatchManagedPathsReject)

		// The error lists the paths of both overlays.
		var denied *ManagedPathsDeniedError
		require.ErrorAs(t, err, &denied)
		assert.Equal(t, wantPaths, denied.Paths)
		assert.ErrorContains(t, err, "static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params)")
		assert.Nil(t, res)
	})

	t.Run("REJECT applies an overlay that sets no managed path", func(t *testing.T) {
		// Apply an unmanaged overlay under REJECT.
		res, err := Apply(parseBase(t), []Overlay{overlay(t, gwParams, `{stats_flush_interval: 10s}`)}, apisettings.BootstrapPatchManagedPathsReject)

		// It applies, with nothing to report.
		require.NoError(t, err)
		assert.Empty(t, res.ManagedPaths)
		assert.Equal(t, int64(10), res.Bootstrap.GetStatsFlushInterval().GetSeconds())
	})
}

func TestApplyCuratedOverrides(t *testing.T) {
	// The class overlay switches the stats matcher's oneof; the Gateway overlay replaces
	// the DNS resolver and adds the downstream connections monitor.
	classOverlay := `{stats_config: {stats_matcher: {reject_all: true}}}`
	gatewayOverlay := `
typed_dns_resolver_config:
  name: envoy.network.dns_resolver.cares
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.network.dns_resolver.cares.v3.CaresDnsResolverConfig
    use_resolvers_as_fallback: true
overload_manager:
  resource_monitors:
  - name: envoy.resource_monitors.global_downstream_max_connections
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.resource_monitors.downstream_connections.v3.DownstreamConnectionsConfig
      max_active_downstream_connections: 10`
	overlays := func(t *testing.T) []Overlay {
		return []Overlay{overlay(t, gcParams, classOverlay), overlay(t, gwParams, gatewayOverlay)}
	}

	t.Run("reports paths the base generated", func(t *testing.T) {
		// Apply onto a base without the monitor, under PRUNE: curated paths are not managed.
		res, err := Apply(parseBase(t), overlays(t), apisettings.BootstrapPatchManagedPathsPrune)
		require.NoError(t, err)

		// Each replacement is attributed to its overlay; the monitor is not reported
		// since the base did not generate it.
		assert.Equal(t, []CuratedOverride{
			{Field: "stats.matcher", Path: "stats_config.stats_matcher", Source: gcParams},
			{Field: "envoyContainer.bootstrap.dnsResolver", Path: "typed_dns_resolver_config", Source: gwParams},
		}, res.CuratedOverrides)
		assert.Empty(t, res.ManagedPaths)

		// The overlays replace the generated values, dropping udp_max_queries.
		assert.True(t, res.Bootstrap.GetStatsConfig().GetStatsMatcher().GetRejectAll())
		cares := &envoycaresv3.CaresDnsResolverConfig{}
		require.NoError(t, res.Bootstrap.GetTypedDnsResolverConfig().GetTypedConfig().UnmarshalTo(cares))
		assert.True(t, cares.GetUseResolversAsFallback())
		assert.Nil(t, cares.GetUdpMaxQueries())
	})

	t.Run("reports the monitor when the base has it", func(t *testing.T) {
		// Apply onto a base that has the monitor maxDownstreamConnections generates.
		base, err := ParseBootstrap([]byte(generatedBase + `
overload_manager:
  resource_monitors:
  - name: envoy.resource_monitors.global_downstream_max_connections
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.resource_monitors.downstream_connections.v3.DownstreamConnectionsConfig
      max_active_downstream_connections: 100`))
		require.NoError(t, err)
		res, err := Apply(base, overlays(t), apisettings.BootstrapPatchManagedPathsWarn)
		require.NoError(t, err)

		// The monitor is reported along with the other two.
		assert.Contains(t, res.CuratedOverrides, CuratedOverride{
			Field:  "envoyContainer.bootstrap.maxDownstreamConnections",
			Path:   "overload_manager.resource_monitors[name=envoy.resource_monitors.global_downstream_max_connections]",
			Source: gwParams,
		})
		assert.Len(t, res.CuratedOverrides, 3)
	})
}

func TestApplyUnvalidatedRuntimeLayers(t *testing.T) {
	// The GatewayClass adds a disk layer, and the Gateway a static layer and an RTDS
	// layer that reads from its own server.
	overlays := []Overlay{
		overlay(t, gcParams, `{layered_runtime: {layers: [{name: from_disk, disk_layer: {symlink_root: /etc/runtime}}]}}`),
		overlay(t, gwParams, `
static_resources:
  clusters: [{name: runtime_server, connect_timeout: 1s}]
layered_runtime:
  layers:
  - {name: static, static_layer: {a: 1}}
  - name: from_rtds
    rtds_layer:
      name: from_rtds
      rtds_config:
        resource_api_version: V3
        api_config_source:
          api_type: GRPC
          transport_api_version: V3
          grpc_services: [{envoy_grpc: {cluster_name: runtime_server}}]
`),
	}

	// Both layers apply under every mode and are reported in order, each with its
	// GatewayParameters. The static layer is not reported: its values are validated.
	for _, mode := range []apisettings.BootstrapPatchManagedPathsMode{
		apisettings.BootstrapPatchManagedPathsWarn,
		apisettings.BootstrapPatchManagedPathsPrune,
		apisettings.BootstrapPatchManagedPathsReject,
	} {
		res, err := Apply(parseBase(t), overlays, mode)
		require.NoError(t, err, "mode %s", mode)
		assert.Equal(t, []RuntimeLayer{
			{Name: "from_disk", Kind: RuntimeLayerDisk, Source: gcParams},
			{Name: "from_rtds", Kind: RuntimeLayerRTDS, Source: gwParams},
		}, res.UnvalidatedLayers, "mode %s", mode)
		assert.Equal(t, []string{"from_disk", "static", "from_rtds", "admin_layer"},
			names(res.Bootstrap.GetLayeredRuntime().GetLayers()), "mode %s", mode)
	}

	// Each layer is named with its kind and GatewayParameters.
	assert.Equal(t, `RTDS layer "from_rtds" (GatewayParameters default/gw-params)`,
		RuntimeLayer{Name: "from_rtds", Kind: RuntimeLayerRTDS, Source: gwParams}.String())
}

func TestListenerPorts(t *testing.T) {
	// Parse a bootstrap with the admin address, static listeners, an additional
	// address, a pipe listener and an internal listener.
	bs, err := ParseBootstrap([]byte(`
admin: {address: {socket_address: {address: 127.0.0.1, port_value: 19000}}}
static_resources:
  listeners:
  - name: a
    address: {socket_address: {address: 0.0.0.0, port_value: 8082}}
    additional_addresses:
    - address: {socket_address: {address: "::", port_value: 8443}}
  - name: b
    address: {socket_address: {address: 0.0.0.0, port_value: 8082}}
  - name: pipe
    address: {pipe: {path: /tmp/sock}}
  - name: internal
    address: {envoy_internal_address: {server_listener_name: x}}
`))
	require.NoError(t, err)

	// Each port appears once, sorted, and the pipe and internal listeners add none.
	assert.Equal(t, []uint32{8082, 8443, 19000}, ListenerPorts(bs))
}

func TestMarshal(t *testing.T) {
	// Marshal a merged bootstrap.
	res, err := Apply(parseBase(t), []Overlay{overlay(t, gwParams, `{statsFlushInterval: 10s}`)}, apisettings.BootstrapPatchManagedPathsWarn)
	require.NoError(t, err)
	out, err := Marshal(res.Bootstrap)
	require.NoError(t, err)

	// Field names are snake_case and keys are sorted.
	assert.Contains(t, string(out), "stats_flush_interval: 10s")
	assert.NotContains(t, string(out), "statsFlushInterval")
	assert.Regexp(t, `(?s)^admin:.*\ncluster_manager:.*\ndynamic_resources:.*\nlayered_runtime:`, string(out))

	// The output parses back to the same bootstrap and marshals to the same bytes.
	reparsed, err := ParseBootstrap(out)
	require.NoError(t, err)
	again, err := Marshal(reparsed)
	require.NoError(t, err)
	assert.Equal(t, string(out), string(again))
}
