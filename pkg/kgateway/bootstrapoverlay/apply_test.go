package bootstrapoverlay

import (
	"errors"
	"testing"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycaresv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/network/dns_resolver/cares/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

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
			name:    "a scalar zero value counts as unset, except in a oneof",
			overlay: `{admin: {address: {socket_address: {address: "", port_value: 0}}}}`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				sa := merged.GetAdmin().GetAddress().GetSocketAddress()
				assert.Equal(t, "127.0.0.1", sa.GetAddress(), "an empty address should keep the base value")
				assert.Zero(t, sa.GetPortValue(), "port_value is in the port_specifier oneof, so a zero value is set and replaces the base value")
			},
		},
		{
			name:    "a Struct merges by key",
			overlay: `{node: {metadata: {extra: {a: 1}}}}`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				fields := merged.GetNode().GetMetadata().GetFields()
				assert.Equal(t, float64(1), fields["extra"].GetStructValue().GetFields()["a"].GetNumberValue(), "the overlay key should be added")
				assert.NotEmpty(t, fields["role"].GetStringValue(), "the base key the overlay leaves out should be kept")
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
			name: "an Any with only a @type replaces the base Any with that type's defaults",
			overlay: `
typed_dns_resolver_config:
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.network.dns_resolver.cares.v3.CaresDnsResolverConfig`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				// The Any keeps its type with an empty payload.
				typed := merged.GetTypedDnsResolverConfig().GetTypedConfig()
				assert.Equal(t, "type.googleapis.com/envoy.extensions.network.dns_resolver.cares.v3.CaresDnsResolverConfig", typed.GetTypeUrl())
				assert.Empty(t, typed.GetValue(), "the base Any's payload should be gone")

				// The resolver gets c-ares's defaults, and the message around the Any still merges.
				cares := &envoycaresv3.CaresDnsResolverConfig{}
				require.NoError(t, typed.UnmarshalTo(cares))
				assert.Nil(t, cares.GetUdpMaxQueries(), "the base Any's udp_max_queries should be gone")
				assert.Equal(t, "envoy.network.dns_resolver.cares", merged.GetTypedDnsResolverConfig().GetName())
			},
		},
		{
			name: "an Any with a different @type and no fields keeps none of the base's bytes",
			overlay: `
typed_dns_resolver_config:
  name: envoy.network.dns_resolver.getaddrinfo
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.network.dns_resolver.getaddrinfo.v3.GetAddrInfoDnsResolverConfig`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				typed := merged.GetTypedDnsResolverConfig().GetTypedConfig()
				assert.Equal(t, "type.googleapis.com/envoy.extensions.network.dns_resolver.getaddrinfo.v3.GetAddrInfoDnsResolverConfig", typed.GetTypeUrl())
				assert.Empty(t, typed.GetValue(), "the c-ares payload should not stay under the new type")
			},
		},
		{
			name: "an Any inside a list entry leaves the base's other entries alone",
			overlay: `
static_resources:
  clusters:
  - name: user_cluster
    typed_extension_protocol_options:
      envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
        "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions`,
			check: func(t *testing.T, merged *envoybootstrapv3.Bootstrap) {
				// The overlay's cluster is added, and the base's Anys outside the list are kept.
				assert.Equal(t, []string{"gw.default", "xds_cluster", "admin_port_cluster", "user_cluster"}, names(merged.GetStaticResources().GetClusters()))
				cares := &envoycaresv3.CaresDnsResolverConfig{}
				require.NoError(t, merged.GetTypedDnsResolverConfig().GetTypedConfig().UnmarshalTo(cares))
				assert.Equal(t, uint32(100), cares.GetUdpMaxQueries().GetValue())
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
			res, err := Apply(base, []Overlay{overlay(t, gwParams, tt.overlay)}, apisettings.BootstrapOverlayManagedPathsWarn)
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
  - {name: user_shared, connect_timeout: 1s}
layered_runtime:
  layers:
  - {name: class_runtime, static_layer: {a: 1}}`),
		overlay(t, gwParams, `
stats_flush_interval: 20s
stats_config:
  stats_matcher: {inclusion_list: {patterns: [{prefix: gateway.}]}}
static_resources:
  clusters:
  - {name: user_shared, connect_timeout: 2s}
layered_runtime:
  layers:
  - {name: gateway_runtime, static_layer: {a: 2}}`),
	}, apisettings.BootstrapOverlayManagedPathsWarn)
	require.NoError(t, err)

	// The Gateway's value wins each conflict.
	merged := res.Bootstrap
	assert.Equal(t, int64(20), merged.GetStatsFlushInterval().GetSeconds())
	shared := merged.GetStaticResources().GetClusters()[3]
	assert.Equal(t, "user_shared", shared.GetName())
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
			overlays: []Overlay{{Source: gwParams, Raw: []byte(`{"node": {"id": "\u007b\u007b.PodName}}"}}`)}},
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
			wantErr:  `runtime layer "mine" sets admin_layer instead of static_layer; a bootstrap overlay can add only static runtime layers`,
		},
		{
			name:     "disk layer",
			overlays: []Overlay{overlay(t, gwParams, `{layered_runtime: {layers: [{name: from_disk, disk_layer: {symlink_root: /etc/runtime}}]}}`)},
			wantErr:  `runtime layer "from_disk" sets disk_layer instead of static_layer`,
		},
		{
			name: "RTDS layer",
			overlays: []Overlay{overlay(t, gwParams, `
static_resources:
  clusters: [{name: runtime_server, connect_timeout: 1s}]
layered_runtime:
  layers:
  - {name: static, static_layer: {a: 1}}
  - name: from_rtds
    rtds_layer:
      name: from_rtds
      rtds_config:
        api_config_source:
          api_type: GRPC
          grpc_services: [{envoy_grpc: {cluster_name: runtime_server}}]
`)},
			wantErr: `runtime layer "from_rtds" sets rtds_layer instead of static_layer`,
		},
		{
			name:     "layer without a type",
			overlays: []Overlay{overlay(t, gwParams, `{layered_runtime: {layers: [{name: empty}]}}`)},
			wantErr:  `runtime layer "empty" sets no layer type instead of static_layer`,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply under every mode: an invalid overlay is rejected regardless.
			for _, mode := range []apisettings.BootstrapOverlayManagedPathsMode{
				apisettings.BootstrapOverlayManagedPathsWarn,
				apisettings.BootstrapOverlayManagedPathsPrune,
				apisettings.BootstrapOverlayManagedPathsReject,
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

func TestApplyReportsEveryProblem(t *testing.T) {
	t.Run("every problem in an overlay", func(t *testing.T) {
		// The GatewayClass overlay is valid and defines the layer "shared". The Gateway
		// overlay has one problem of each kind the checks after parsing report.
		overlays := []Overlay{
			overlay(t, gcParams, `{layered_runtime: {layers: [{name: shared, static_layer: {}}]}}`),
			overlay(t, gwParams, `
static_resources:
  secrets: [{name: SYSTEM_CA_CERT}]
  clusters: [{name: kube_a}, {name: kube_a}, {name: kube_b}]
layered_runtime:
  layers:
  - {name: from_disk, disk_layer: {symlink_root: /etc/runtime}}
  - {name: admin_layer, static_layer: {}}
  - {name: user, static_layer: {}}
  - {name: user, static_layer: {}}
  - {name: shared, static_layer: {}}
`),
		}
		res, err := Apply(parseBase(t), overlays, apisettings.BootstrapOverlayManagedPathsWarn)

		// Check that each problem has its own error naming the Gateway overlay, and that
		// a name that breaks two rules or repeats is reported once for each rule.
		assert.Nil(t, res)
		want := []string{
			`static secret "SYSTEM_CA_CERT" is reserved: the proxy adds it for the OS CA bundle`,
			`runtime layer "from_disk" sets disk_layer instead of static_layer; a bootstrap overlay can add only static runtime layers`,
			`static_resources.clusters has more than one entry named "kube_a"`,
			`layered_runtime.layers has more than one entry named "user"`,
			`static cluster "kube_a" is not in the generated bootstrap and its name does not start with "user_"; give a new static cluster a name that starts with "user_"`,
			`static cluster "kube_b" is not in the generated bootstrap and its name does not start with "user_"; give a new static cluster a name that starts with "user_"`,
			`runtime layer "admin_layer" has the name of a layer in the generated bootstrap; give it another name`,
			`runtime layer "shared" is also defined in GatewayParameters kgateway-system/class-params; give one of them another name`,
		}
		for i := range want {
			want[i] = "bootstrap overlay in GatewayParameters default/gw-params: " + want[i]
		}
		assert.Equal(t, want, invalidOverlayMessages(t, err))
	})

	t.Run("every invalid overlay", func(t *testing.T) {
		// The GatewayClass overlay does not parse, and the Gateway overlay has a cluster
		// without the prefix.
		overlays := []Overlay{
			{Source: gcParams, Raw: []byte(`{"layerd_runtime": {}}`)},
			overlay(t, gwParams, `{static_resources: {clusters: [{name: kube_a}]}}`),
		}
		res, err := Apply(parseBase(t), overlays, apisettings.BootstrapOverlayManagedPathsWarn)

		// Check that both overlays are reported, in order.
		assert.Nil(t, res)
		msgs := invalidOverlayMessages(t, err)
		require.Len(t, msgs, 2, "one error for each invalid overlay")
		assert.Contains(t, msgs[0], "bootstrap overlay in GatewayParameters kgateway-system/class-params: does not parse as an Envoy Bootstrap")
		assert.Contains(t, msgs[1], `bootstrap overlay in GatewayParameters default/gw-params: static cluster "kube_a" is not in the generated bootstrap`)
	})
}

// invalidOverlayMessages returns the message of each *InvalidOverlayError that err,
// an error from Apply, joins.
func invalidOverlayMessages(t *testing.T, err error) []string {
	t.Helper()

	// Unwrap the joined errors.
	joined, ok := err.(interface{ Unwrap() []error })
	require.True(t, ok, "Apply joins its invalid-overlay errors: %v", err)

	// Check that each is an *InvalidOverlayError, and collect its message.
	var msgs []string
	for _, e := range joined.Unwrap() {
		invalid, ok := errors.AsType[*InvalidOverlayError](e)
		require.True(t, ok, "%v is not an *InvalidOverlayError", e)
		msgs = append(msgs, invalid.Error())
	}
	return msgs
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
		res, err := Apply(parseBase(t), overlays(t), apisettings.BootstrapOverlayManagedPathsWarn)
		require.NoError(t, err)

		// Every path is reported, and the overlays applied.
		assert.Equal(t, wantPaths, res.ManagedPaths)
		assert.Equal(t, "other", res.Bootstrap.GetClusterManager().GetLocalClusterName())
		assert.Equal(t, "z1", res.Bootstrap.GetNode().GetLocality().GetZone())
		assert.Equal(t, int64(9), res.Bootstrap.GetStaticResources().GetClusters()[1].GetConnectTimeout().GetSeconds())
	})

	t.Run("PRUNE drops and reports each path and applies the rest", func(t *testing.T) {
		// Apply under PRUNE.
		base := parseBase(t)
		res, err := Apply(base, overlays(t), apisettings.BootstrapOverlayManagedPathsPrune)
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
	})

	t.Run("REJECT applies nothing and reports every overlay's paths", func(t *testing.T) {
		// Apply under REJECT.
		res, err := Apply(parseBase(t), overlays(t), apisettings.BootstrapOverlayManagedPathsReject)

		// The error lists the paths of both overlays.
		var denied *ManagedPathsDeniedError
		require.ErrorAs(t, err, &denied)
		assert.Equal(t, wantPaths, denied.Paths)
		assert.ErrorContains(t, err, "static_resources.clusters[name=xds_cluster] (GatewayParameters default/gw-params)")
		assert.Nil(t, res)
	})

	t.Run("REJECT applies an overlay that sets no managed path", func(t *testing.T) {
		// Apply an unmanaged overlay under REJECT.
		res, err := Apply(parseBase(t), []Overlay{overlay(t, gwParams, `{stats_flush_interval: 10s}`)}, apisettings.BootstrapOverlayManagedPathsReject)

		// It applies, with nothing to report.
		require.NoError(t, err)
		assert.Empty(t, res.ManagedPaths)
		assert.Equal(t, int64(10), res.Bootstrap.GetStatsFlushInterval().GetSeconds())
	})
}

func TestApplyTypedStruct(t *testing.T) {
	for _, typedStruct := range []string{"udpa.type.v1.TypedStruct", "xds.type.v3.TypedStruct"} {
		t.Run(typedStruct, func(t *testing.T) {
			// An overlay adds a bootstrap extension whose config is wrapped in a TypedStruct.
			overlay := overlay(t, gwParams, `
bootstrap_extensions:
- name: wrapped
  typed_config:
    "@type": type.googleapis.com/`+typedStruct+`
    type_url: type.googleapis.com/envoy.extensions.network.dns_resolver.cares.v3.CaresDnsResolverConfig
    value: {use_resolvers_as_fallback: true}`)

			// It parses and merges, keeping the wrapper.
			res, err := Apply(parseBase(t), []Overlay{overlay}, apisettings.BootstrapOverlayManagedPathsWarn)
			require.NoError(t, err)
			exts := res.Bootstrap.GetBootstrapExtensions()
			require.Len(t, exts, 1)
			assert.Equal(t, "type.googleapis.com/"+typedStruct, exts[0].GetTypedConfig().GetTypeUrl())
		})
	}
}

func TestApplyNewGeneratedRuntimeLayer(t *testing.T) {
	// The generated bootstrap gets a runtime layer that a later kgateway release could
	// add, with the name of a layer a stored overlay already adds.
	newBase := func() *envoybootstrapv3.Bootstrap {
		base := parseBase(t)
		mergeOverlay(base, parseYAMLBootstrap(t, `{layered_runtime: {layers: [{name: user_runtime, static_layer: {}}]}}`))
		return base
	}
	overlay := overlay(t, gwParams, `{layered_runtime: {layers: [{name: user_runtime, static_layer: {a: 1}}]}}`)

	// The overlay is invalid under every mode, so the Gateway takes the invalid-merge
	// fallback rather than replacing or dropping either layer.
	for _, mode := range []apisettings.BootstrapOverlayManagedPathsMode{
		apisettings.BootstrapOverlayManagedPathsWarn,
		apisettings.BootstrapOverlayManagedPathsPrune,
		apisettings.BootstrapOverlayManagedPathsReject,
	} {
		res, err := Apply(newBase(), []Overlay{overlay}, mode)
		var invalid *InvalidOverlayError
		require.ErrorAs(t, err, &invalid, "mode %s", mode)
		assert.ErrorContains(t, err, `runtime layer "user_runtime" has the name of a layer in the generated bootstrap`, "mode %s", mode)
		assert.Nil(t, res)
	}
}

func TestApplyNamedEntries(t *testing.T) {
	// The generated bootstrap gets one entry of each named list that only this test's
	// base has, as a later kgateway release could add.
	withGeneratedEntries := func(t *testing.T) *envoybootstrapv3.Bootstrap {
		t.Helper()
		bs := parseBase(t)
		extra := parseYAMLBootstrap(t, `
static_resources:
  clusters: [{name: generated_cluster}]
  listeners:
  - name: generated_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 9000}}
  secrets: [{name: generated_secret}]
overload_manager:
  resource_monitors: [{name: envoy.resource_monitors.global_downstream_max_connections}]`)
		mergeOverlay(bs, extra)
		return bs
	}

	// The GatewayClass's overlay replaces each of them and xds_cluster, and adds a
	// cluster, a listener, a secret and a resource monitor of its own; the Gateway's
	// replaces one of them again.
	classOverlay := `
static_resources:
  clusters: [{name: generated_cluster}, {name: xds_cluster}, {name: user_cluster}]
  listeners:
  - name: generated_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 9001}}
  - name: user_listener
    address: {socket_address: {address: 0.0.0.0, port_value: 9002}}
  secrets: [{name: generated_secret}, {name: my_secret}]
overload_manager:
  resource_monitors:
  - {name: envoy.resource_monitors.global_downstream_max_connections}
  - {name: envoy.resource_monitors.fixed_heap}`
	gatewayOverlay := `{static_resources: {clusters: [{name: generated_cluster}]}}`
	overlays := func(t *testing.T) []Overlay {
		return []Overlay{overlay(t, gcParams, classOverlay), overlay(t, gwParams, gatewayOverlay)}
	}
	wantPaths := []ManagedPath{
		{Path: "static_resources.clusters[name=generated_cluster]", Source: gcParams},
		{Path: "static_resources.clusters[name=xds_cluster]", Source: gcParams},
		{Path: "static_resources.listeners[name=generated_listener]", Source: gcParams},
		{Path: "static_resources.secrets[name=generated_secret]", Source: gcParams},
		{Path: "overload_manager.resource_monitors[name=envoy.resource_monitors.global_downstream_max_connections]", Source: gcParams},
		{Path: "static_resources.clusters[name=generated_cluster]", Source: gwParams},
	}

	t.Run("WARN applies and reports each generated entry as a managed path", func(t *testing.T) {
		// Apply the overlays.
		res, err := Apply(withGeneratedEntries(t), overlays(t), apisettings.BootstrapOverlayManagedPathsWarn)
		require.NoError(t, err)

		// Each generated entry is reported once per overlay that replaces it, and the
		// overlay's own entries not at all.
		assert.Equal(t, wantPaths, res.ManagedPaths)

		// The replacement applied, and the new entries were added.
		listeners := res.Bootstrap.GetStaticResources().GetListeners()
		assert.Equal(t, []string{"readiness_listener", "prometheus_listener", "generated_listener", "user_listener"}, names(listeners))
		assert.Equal(t, uint32(9001), listeners[2].GetAddress().GetSocketAddress().GetPortValue(),
			"the overlay's listener should replace the generated one")
		assert.Equal(t, []string{"generated_secret", "my_secret"}, names(res.Bootstrap.GetStaticResources().GetSecrets()))
		assert.Len(t, res.Bootstrap.GetOverloadManager().GetResourceMonitors(), 2)
	})

	t.Run("PRUNE drops each generated entry and adds the new ones", func(t *testing.T) {
		// Apply the overlays.
		res, err := Apply(withGeneratedEntries(t), overlays(t), apisettings.BootstrapOverlayManagedPathsPrune)
		require.NoError(t, err)

		// The generated entries keep their generated values; the new entries were added.
		assert.Equal(t, wantPaths, res.ManagedPaths)
		listeners := res.Bootstrap.GetStaticResources().GetListeners()
		assert.Equal(t, uint32(9000), listeners[2].GetAddress().GetSocketAddress().GetPortValue())
		assert.Equal(t, "user_listener", listeners[3].GetName())
		assert.Equal(t, []string{"generated_secret", "my_secret"}, names(res.Bootstrap.GetStaticResources().GetSecrets()))
		assert.Contains(t, names(res.Bootstrap.GetStaticResources().GetClusters()), "user_cluster")
	})

	t.Run("REJECT refuses an overlay that replaces a generated entry", func(t *testing.T) {
		// Apply an overlay that replaces only the resource monitor.
		res, err := Apply(withGeneratedEntries(t), []Overlay{overlay(t, gwParams,
			`{overload_manager: {resource_monitors: [{name: envoy.resource_monitors.global_downstream_max_connections}]}}`)},
			apisettings.BootstrapOverlayManagedPathsReject)

		// It is refused as a managed path.
		var denied *ManagedPathsDeniedError
		require.ErrorAs(t, err, &denied)
		assert.Equal(t, []ManagedPath{{
			Path:   "overload_manager.resource_monitors[name=envoy.resource_monitors.global_downstream_max_connections]",
			Source: gwParams,
		}}, denied.Paths)
		assert.Nil(t, res)
	})
}

func TestApplyStaticNamePrefix(t *testing.T) {
	tests := []struct {
		name    string
		overlay string
		wantErr string
	}{
		{
			name:    "a translated backend cluster's name",
			overlay: `{static_resources: {clusters: [{name: kube_default_httpbin_8000, connect_timeout: 1s}]}}`,
			wantErr: `static cluster "kube_default_httpbin_8000" is not in the generated bootstrap and its name does not start with "user_"`,
		},
		{
			name:    "a generated cluster the base does not have",
			overlay: `{static_resources: {clusters: [{name: gateway_proxy_sds}]}}`,
			wantErr: `static cluster "gateway_proxy_sds" is not in the generated bootstrap`,
		},
		{
			name:    "a cluster without a name",
			overlay: `{static_resources: {clusters: [{connect_timeout: 1s}]}}`,
			wantErr: `static cluster "" is not in the generated bootstrap`,
		},
		{
			name: "a translated listener's name",
			overlay: `
static_resources:
  listeners:
  - name: listener~8080
    address: {socket_address: {address: 0.0.0.0, port_value: 9000}}`,
			wantErr: `static listener "listener~8080" is not in the generated bootstrap and its name does not start with "user_"`,
		},
		{
			name:    "a prefix in another case",
			overlay: `{static_resources: {clusters: [{name: User_cluster}]}}`,
			wantErr: `static cluster "User_cluster" is not in the generated bootstrap`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply under every mode: a new name without the prefix is rejected regardless.
			for _, mode := range []apisettings.BootstrapOverlayManagedPathsMode{
				apisettings.BootstrapOverlayManagedPathsWarn,
				apisettings.BootstrapOverlayManagedPathsPrune,
				apisettings.BootstrapOverlayManagedPathsReject,
			} {
				res, err := Apply(parseBase(t), []Overlay{overlay(t, gwParams, tt.overlay)}, mode)

				// Check the error type and message.
				var invalid *InvalidOverlayError
				require.ErrorAs(t, err, &invalid, "mode %s", mode)
				assert.ErrorContains(t, err, tt.wantErr, "mode %s", mode)
				assert.Nil(t, res)
			}
		})
	}

	t.Run("a release that removes a generated cluster an overlay replaces", func(t *testing.T) {
		// A stored overlay replaces admin_port_cluster, and the base no longer has it.
		base := parseBase(t)
		base.StaticResources.Clusters = base.StaticResources.Clusters[:2]
		_, err := Apply(base, []Overlay{overlay(t, gwParams, `{static_resources: {clusters: [{name: admin_port_cluster}]}}`)}, apisettings.BootstrapOverlayManagedPathsWarn)

		// The overlay is rejected, so the Gateway takes the invalid-merge fallback.
		var invalid *InvalidOverlayError
		require.ErrorAs(t, err, &invalid)
		assert.ErrorContains(t, err, `static cluster "admin_port_cluster" is not in the generated bootstrap`)
	})
}

func TestSourceKind(t *testing.T) {
	// A source without a kind is a GatewayParameters.
	assert.Equal(t, "GatewayParameters default/gw-params", gwParams.String())

	// Messages name a source of another kind by that kind.
	proxyParams := Source{Kind: "ProxyParameters", Namespace: "default", Name: "proxy-params"}
	_, err := Apply(parseBase(t), []Overlay{overlay(t, proxyParams, `{node: {id: x}, layered_runtime: {layers: [{name: admin_layer, static_layer: {}}]}}`)}, apisettings.BootstrapOverlayManagedPathsWarn)
	assert.ErrorContains(t, err, "bootstrap overlay in ProxyParameters default/proxy-params: ")
	res, err := Apply(parseBase(t), []Overlay{overlay(t, proxyParams, `{node: {id: x}}`)}, apisettings.BootstrapOverlayManagedPathsWarn)
	require.NoError(t, err)
	require.Len(t, res.ManagedPaths, 1)
	assert.Equal(t, "node (ProxyParameters default/proxy-params)", res.ManagedPaths[0].String())
}
