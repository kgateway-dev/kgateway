package proxy_syncer

import (
	"testing"
	"time"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoygrpcaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	sfsvalue "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/set_filter_state/v3"
	envoyjwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	envoyhttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	sfsnetwork "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/set_filter_state/v3"
	envoysniclusterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3"
	envoytcpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// TestEmissionSetIncludesAncillaryReferencesTheGateSkips mirrors
// TestCollectReferencedClusters_ExcludesAncillaryReferences: readiness skips
// these clusters so one plugin bug cannot starve a gateway; emission keeps them
// because filters call them.
func TestEmissionSetIncludesAncillaryReferencesTheGateSkips(t *testing.T) {
	listeners := listenersWithAncillaryClusters(t)

	emission := collectReferencedClustersForEmission(envoycache.Resources{}, listeners)
	gating := collectReferencedClusters(envoycache.Resources{}, listeners)

	for _, name := range []string{"access-log-cluster", "jwks-cluster"} {
		assert.Containsf(t, emission.Names, name,
			"%s is a real cluster reached only through typed_config; dropping it is a permanent, route-invisible outage", name)
		assert.NotContainsf(t, gating, name,
			"%s must stay out of the readiness gate", name)
	}
	assert.True(t, emission.Filterable(), "no request-time selector is present")
}

// TestEmissionSetAlwaysIncludesBlackhole: routes with unresolved backends target
// blackhole, which a healthy build may name nowhere.
func TestEmissionSetAlwaysIncludesBlackhole(t *testing.T) {
	emission := collectReferencedClustersForEmission(envoycache.Resources{}, envoycache.Resources{})

	assert.Contains(t, emission.Names, wellknown.BlackholeClusterName)
}

// TestEmissionSetCollectsDeclarativeRouteTargets covers each route-target shape
// the translator emits.
func TestEmissionSetCollectsDeclarativeRouteTargets(t *testing.T) {
	routes := sliceToResources([]*envoyroutev3.RouteConfiguration{{
		Name: "listener~80",
		VirtualHosts: []*envoyroutev3.VirtualHost{{
			Name:    "vhost",
			Domains: []string{"*"},
			Routes: []*envoyroutev3.Route{
				{
					Name:   "direct",
					Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "direct-cluster"}}},
				},
				{
					Name: "weighted",
					Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
						ClusterSpecifier: &envoyroutev3.RouteAction_WeightedClusters{
							WeightedClusters: &envoyroutev3.WeightedCluster{
								Clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
									{Name: "weighted-a"}, {Name: "weighted-b"},
								},
							},
						},
					}},
				},
				{
					Name: "mirrored",
					Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
						ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "primary"},
						RequestMirrorPolicies: []*envoyroutev3.RouteAction_RequestMirrorPolicy{
							{Cluster: "mirror-cluster"},
						},
					}},
				},
			},
		}},
	}})

	listeners := sliceToResources([]*envoylistenerv3.Listener{{
		Name: "tcp",
		FilterChains: []*envoylistenerv3.FilterChain{{
			Filters: []*envoylistenerv3.Filter{{
				Name: envoywellknown.TCPProxy,
				ConfigType: &envoylistenerv3.Filter_TypedConfig{
					TypedConfig: mustMessageToAny(t, &envoytcpv3.TcpProxy{
						StatPrefix:       "tcp",
						ClusterSpecifier: &envoytcpv3.TcpProxy_Cluster{Cluster: "tcp-cluster"},
					}),
				},
			}},
		}},
	}})

	emission := collectReferencedClustersForEmission(routes, listeners)

	for _, name := range []string{
		"direct-cluster", "weighted-a", "weighted-b", "primary", "mirror-cluster", "tcp-cluster",
	} {
		assert.Contains(t, emission.Names, name)
	}
	assert.True(t, emission.Filterable())
}

// TestEmissionSetReportsRequestTimeSelectors: the three request-time
// cluster_specifier arms and header-named weighted or mirror clusters make the
// gateway unfilterable. Their candidates are named nowhere, and pruning them
// silently routes to the plugin's fallback.
func TestEmissionSetReportsRequestTimeSelectors(t *testing.T) {
	tests := []struct {
		name     string
		action   *envoyroutev3.RouteAction
		expected string
	}{
		{
			name:     "cluster header",
			action:   &envoyroutev3.RouteAction{ClusterSpecifier: &envoyroutev3.RouteAction_ClusterHeader{ClusterHeader: "x-target"}},
			expected: `cluster_header "x-target"`,
		},
		{
			name:     "cluster specifier plugin",
			action:   &envoyroutev3.RouteAction{ClusterSpecifier: &envoyroutev3.RouteAction_ClusterSpecifierPlugin{ClusterSpecifierPlugin: "lua-picker"}},
			expected: `cluster_specifier_plugin "lua-picker"`,
		},
		{
			name: "inline cluster specifier plugin",
			action: &envoyroutev3.RouteAction{ClusterSpecifier: &envoyroutev3.RouteAction_InlineClusterSpecifierPlugin{
				InlineClusterSpecifierPlugin: &envoyroutev3.ClusterSpecifierPlugin{
					Extension: &envoycorev3.TypedExtensionConfig{Name: "inline-picker"},
				},
			}},
			expected: `inline_cluster_specifier_plugin "inline-picker"`,
		},
		{
			name: "weighted cluster entry naming its cluster by header",
			action: &envoyroutev3.RouteAction{ClusterSpecifier: &envoyroutev3.RouteAction_WeightedClusters{
				WeightedClusters: &envoyroutev3.WeightedCluster{Clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
					{Name: "backend", Weight: wrapperspb.UInt32(1)},
					{ClusterHeader: "x-weighted-target", Weight: wrapperspb.UInt32(1)},
				}},
			}},
			expected: `cluster_header "x-weighted-target"`,
		},
		{
			name: "mirror policy naming its cluster by header",
			action: &envoyroutev3.RouteAction{
				ClusterSpecifier:      &envoyroutev3.RouteAction_Cluster{Cluster: "backend"},
				RequestMirrorPolicies: []*envoyroutev3.RouteAction_RequestMirrorPolicy{{ClusterHeader: "x-mirror-target"}},
			},
			expected: `cluster_header "x-mirror-target"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			routes := sliceToResources([]*envoyroutev3.RouteConfiguration{{
				Name: "listener~80",
				VirtualHosts: []*envoyroutev3.VirtualHost{{
					Name:    "vhost",
					Domains: []string{"*"},
					Routes: []*envoyroutev3.Route{{
						Name:   "dynamic",
						Action: &envoyroutev3.Route_Route{Route: tc.action},
					}},
				}},
			}})

			emission := collectReferencedClustersForEmission(routes, envoycache.Resources{})

			assert.False(t, emission.Filterable(),
				"a request-time destination must make the gateway fall back to emitting every cluster")
			assert.Equal(t, []string{tc.expected}, emission.Unresolvable)
		})
	}
}

// TestEmissionSetIsFilterableForOrdinaryRoutes: an ordinary route must not trip
// the guard, or filtering never engages.
func TestEmissionSetIsFilterableForOrdinaryRoutes(t *testing.T) {
	routes := sliceToResources([]*envoyroutev3.RouteConfiguration{{
		Name: "listener~80",
		VirtualHosts: []*envoyroutev3.VirtualHost{{
			Name:    "vhost",
			Domains: []string{"*"},
			Routes: []*envoyroutev3.Route{{
				Name:   "plain",
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "backend"}}},
			}},
		}},
	}})

	emission := collectReferencedClustersForEmission(routes, envoycache.Resources{})

	assert.True(t, emission.Filterable())
	assert.Empty(t, emission.Unresolvable)
}

// TestEmittedClustersEquals: both halves must count, or a gateway that becomes
// unfilterable keeps being filtered.
func TestEmittedClustersEquals(t *testing.T) {
	base := emittedClusters{Names: map[string]struct{}{"a": {}}}

	assert.True(t, base.Equals(emittedClusters{Names: map[string]struct{}{"a": {}}}))
	assert.False(t, base.Equals(emittedClusters{Names: map[string]struct{}{"a": {}, "b": {}}}),
		"a changed cluster set must invalidate the projection")
	assert.False(t, base.Equals(emittedClusters{Names: map[string]struct{}{"a": {}}, Unresolvable: []string{"cluster_header \"x\""}}),
		"a gateway that just became unfilterable must invalidate the projection")

	gateway := GatewayXdsResources{EmittedClusters: base}
	assert.True(t, gateway.Equals(GatewayXdsResources{EmittedClusters: emittedClusters{Names: map[string]struct{}{"a": {}}}}))
	assert.False(t, gateway.Equals(GatewayXdsResources{EmittedClusters: emittedClusters{Names: map[string]struct{}{"a": {}, "b": {}}}}),
		"the gateway collection must detect changed emission candidates even with unchanged resource versions")
	assert.False(t, gateway.Equals(GatewayXdsResources{EmittedClusters: emittedClusters{Names: base.Names, Unresolvable: []string{"cluster_header \"x\""}}}),
		"the gateway collection must detect when filtering becomes unsafe")
}

func listenersWithAncillaryClusters(t *testing.T) envoycache.Resources {
	t.Helper()
	hcm := &envoyhttpv3.HttpConnectionManager{
		AccessLog: []*envoyaccesslogv3.AccessLog{{
			Name: "envoy.access_loggers.http_grpc",
			ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
				TypedConfig: mustMessageToAny(t, &envoygrpcaccesslogv3.HttpGrpcAccessLogConfig{
					CommonConfig: &envoygrpcaccesslogv3.CommonGrpcAccessLogConfig{
						TransportApiVersion: envoycorev3.ApiVersion_V3,
						LogName:             "grpc-log",
						GrpcService: &envoycorev3.GrpcService{
							TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
								EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{ClusterName: "access-log-cluster"},
							},
						},
					},
				}),
			},
		}},
		HttpFilters: []*envoyhttpv3.HttpFilter{{
			Name: "envoy.filters.http.jwt_authn",
			ConfigType: &envoyhttpv3.HttpFilter_TypedConfig{
				TypedConfig: mustMessageToAny(t, &envoyjwtauthnv3.JwtAuthentication{
					Providers: map[string]*envoyjwtauthnv3.JwtProvider{"provider": {
						JwksSourceSpecifier: &envoyjwtauthnv3.JwtProvider_RemoteJwks{
							RemoteJwks: &envoyjwtauthnv3.RemoteJwks{
								HttpUri: &envoycorev3.HttpUri{
									Uri:              "https://example.com/jwks",
									HttpUpstreamType: &envoycorev3.HttpUri_Cluster{Cluster: "jwks-cluster"},
									Timeout:          durationpb.New(time.Second),
								},
							},
						},
					}},
				}),
			},
		}},
	}

	listeners := sliceToResources([]*envoylistenerv3.Listener{{
		Name: "listener",
		FilterChains: []*envoylistenerv3.FilterChain{{
			Filters: []*envoylistenerv3.Filter{{
				Name: envoywellknown.HTTPConnectionManager,
				ConfigType: &envoylistenerv3.Filter_TypedConfig{
					TypedConfig: mustMessageToAny(t, hcm),
				},
			}},
		}},
	}})
	require.NotEmpty(t, listeners.Items)
	return listeners
}

// Filter-state overrides can select clusters beyond TcpProxy's declared target.
func TestEmissionSetReportsTcpProxyClusterOverrides(t *testing.T) {
	tcpProxy := &envoylistenerv3.Filter{
		Name: envoywellknown.TCPProxy,
		ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: mustMessageToAny(t, &envoytcpv3.TcpProxy{
			StatPrefix:       "tcp",
			ClusterSpecifier: &envoytcpv3.TcpProxy_Cluster{Cluster: "backend"},
		})},
	}
	listenerWith := func(override proto.Message) envoycache.Resources {
		return sliceToResources([]*envoylistenerv3.Listener{{
			Name: "tcp",
			FilterChains: []*envoylistenerv3.FilterChain{{Filters: []*envoylistenerv3.Filter{
				{Name: "override", ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: mustMessageToAny(t, override)}},
				tcpProxy,
			}}},
		}})
	}

	for name, tc := range map[string]struct {
		override proto.Message
		expected []string
	}{
		"sni_cluster": {&envoysniclusterv3.SniCluster{}, []string{"sni_cluster"}},
		"set_filter_state writing the TcpProxy cluster": {&sfsnetwork.Config{OnNewConnection: []*sfsvalue.FilterStateValue{{
			Key: &sfsvalue.FilterStateValue_ObjectKey{ObjectKey: "envoy.tcp_proxy.cluster"},
		}}}, []string{`filter_state "envoy.tcp_proxy.cluster"`}},
		"set_filter_state writing another key": {&sfsnetwork.Config{OnNewConnection: []*sfsvalue.FilterStateValue{{
			Key: &sfsvalue.FilterStateValue_ObjectKey{ObjectKey: "io.istio.peer_principal"},
		}}}, []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			emission := collectReferencedClustersForEmission(envoycache.Resources{}, listenerWith(tc.override))
			assert.Equal(t, tc.expected, emission.Unresolvable)
			assert.Contains(t, emission.Names, "backend")
		})
	}
}

// Every arm of RouteAction.cluster_specifier is either declarative (the walk
// sees its names) or a request-time selector collectUnresolvableSelectors
// reports. An arm added by a go-control-plane bump is reported as unknown, and
// this test fails so that someone decides which it is.
func TestRouteActionClusterSpecifierArmsAreClassified(t *testing.T) {
	oneof := (&envoyroutev3.RouteAction{}).ProtoReflect().Descriptor().Oneofs().ByName("cluster_specifier")
	require.NotNil(t, oneof)
	var arms []string
	for i := 0; i < oneof.Fields().Len(); i++ {
		arms = append(arms, string(oneof.Fields().Get(i).Name()))
	}
	assert.ElementsMatch(t, []string{
		// declarative
		"cluster", "weighted_clusters",
		// request-time selectors
		"cluster_header", "cluster_specifier_plugin", "inline_cluster_specifier_plugin",
	}, arms)
}

// An undecodable typed_config, such as an unlinked downstream extension, can
// name clusters the walk never sees, so it makes the gateway unfilterable.
func TestEmissionSetReportsUndecodableTypedConfig(t *testing.T) {
	hcm := &envoyhttpv3.HttpConnectionManager{
		StatPrefix: "http",
		HttpFilters: []*envoyhttpv3.HttpFilter{{
			Name: "example.opaque",
			ConfigType: &envoyhttpv3.HttpFilter_TypedConfig{TypedConfig: &anypb.Any{
				TypeUrl: "type.googleapis.com/example.v1.NotLinked",
				Value:   []byte{0x0a, 0x07, 'b', 'a', 'c', 'k', 'e', 'n', 'd'},
			}},
		}},
	}
	listeners := sliceToResources([]*envoylistenerv3.Listener{{
		Name: "http",
		FilterChains: []*envoylistenerv3.FilterChain{{Filters: []*envoylistenerv3.Filter{{
			Name:       envoywellknown.HTTPConnectionManager,
			ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: mustMessageToAny(t, hcm)},
		}}}},
	}})

	emission := collectReferencedClustersForEmission(envoycache.Resources{}, listeners)

	assert.False(t, emission.Filterable())
	assert.Equal(t, []string{`undecodable typed_config "type.googleapis.com/example.v1.NotLinked"`}, emission.Unresolvable)
}
