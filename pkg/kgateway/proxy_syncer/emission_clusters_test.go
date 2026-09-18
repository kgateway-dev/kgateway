package proxy_syncer

import (
	"testing"
	"time"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoygrpcaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	envoyjwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	envoyhttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytcpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// TestEmissionSetIncludesAncillaryReferencesTheGateSkips is the counterpart to
// TestCollectReferencedClusters_ExcludesAncillaryReferences. The two collectors
// look at the same protos and must disagree about exactly these clusters: the
// gate excludes them so one plugin bug cannot starve a gateway, emission
// includes them because Envoy genuinely needs them.
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
	assert.True(t, emission.Filterable(emissionClaims{}), "no request-time selector is present")
}

// TestEmissionSetAlwaysIncludesBlackhole: routes whose backends fail resolution
// target the blackhole cluster, and a healthy build may name it from no proto at
// all. Filtering it out would remove the cluster those routes land on.
func TestEmissionSetAlwaysIncludesBlackhole(t *testing.T) {
	emission := collectReferencedClustersForEmission(envoycache.Resources{}, envoycache.Resources{})

	assert.Contains(t, emission.Names, wellknown.BlackholeClusterName)
}

// TestEmissionSetCollectsDeclarativeRouteTargets covers the destinations both
// collectors agree on, through each shape the translator emits.
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
	assert.True(t, emission.Filterable(emissionClaims{}))
}

// TestEmissionSetReportsRequestTimeSelectors pins the guard on all three arms of
// the cluster_specifier oneof that choose a destination per request. Each makes
// the whole gateway unfilterable, because the candidates it may select are named
// nowhere: pruning them does not 503, it silently sends every affected request to
// the plugin's fallback.
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

			assert.False(t, emission.Filterable(emissionClaims{}),
				"a request-time destination must make the gateway fall back to emitting every cluster")
			assert.Equal(t, []string{tc.expected}, emission.unaccountedSelectors(emissionClaims{}))
		})
	}
}

// TestEmissionSetIsFilterableForOrdinaryRoutes is the negative control for the
// guard: an ordinary declarative route must not trip it, or the filter would
// never engage anywhere.
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

	assert.True(t, emission.Filterable(emissionClaims{}))
	assert.Empty(t, emission.RequestTimeSelectors)
}

// TestEmittedClustersEquals: the set rides on GatewayXdsResources, whose Equals
// decides whether a gateway's xDS projection is recomputed. A difference in
// either half has to register, or a gateway that becomes unfilterable would keep
// being filtered.
func TestEmittedClustersEquals(t *testing.T) {
	base := emittedClusters{Names: map[string]struct{}{"a": {}}}

	assert.True(t, base.Equals(emittedClusters{Names: map[string]struct{}{"a": {}}}))
	assert.False(t, base.Equals(emittedClusters{Names: map[string]struct{}{"a": {}, "b": {}}}),
		"a changed cluster set must invalidate the projection")
	assert.False(t, base.Equals(emittedClusters{Names: map[string]struct{}{"a": {}}, RequestTimeSelectors: []requestTimeSelector{{selectorClusterHeader, "x"}}}),
		"a gateway that just became unfilterable must invalidate the projection")
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
