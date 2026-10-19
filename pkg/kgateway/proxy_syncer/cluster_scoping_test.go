package proxy_syncer

import (
	"testing"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

// TestClusterScopingIsOffByDefault: an upgrade must not change what proxies are sent.
func TestClusterScopingIsOffByDefault(t *testing.T) {
	assert.False(t, clusterScopingFrom(apisettings.Settings{}).ScopesClusters(),
		"the zero Settings must not scope clusters")
	assert.False(t, clusterScopingFrom(apisettings.Settings{
		ClusterDiscoveryMode: apisettings.ClusterDiscoveryAll,
	}).ScopesClusters())
	assert.True(t, clusterScopingFrom(apisettings.Settings{
		ClusterDiscoveryMode: apisettings.ClusterDiscoveryReferenced,
	}).ScopesClusters())
}

// TestDisabledScopingSkipsTheEmissionWalkEntirely: disabled scoping must skip the
// proto walk, not compute and discard its result.
func TestDisabledScopingSkipsTheEmissionWalkEntirely(t *testing.T) {
	routes := sliceToResources([]*envoyroutev3.RouteConfiguration{{
		Name: "route-config",
		VirtualHosts: []*envoyroutev3.VirtualHost{{
			Name:    "vhost",
			Domains: []string{"*"},
			Routes: []*envoyroutev3.Route{{
				Name: "routed",
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
					ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "routed"},
				}},
			}},
		}},
	}})
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})

	disabled := emittedClustersFor(clusterScoping{}, routes, listeners)
	assert.Empty(t, disabled.Names, "nothing is collected when CDS is not scoped")
	assert.Empty(t, disabled.Unresolvable)

	enabled := emittedClustersFor(scopedClusters(), routes, listeners)
	require.Contains(t, enabled.Names, "routed",
		"the same inputs must collect normally once scoping is on")
}

// TestDisabledScopingFilterIsIdentity: with scoping off the filter returns its
// input unchanged, even when the emitted set would drop a cluster.
func TestDisabledScopingFilterIsIdentity(t *testing.T) {
	clusters, versions := clusterResourcesFor("routed", "unreferenced")

	got, gotVersions, filtered := filterClustersToEmitted(
		clusterScoping{}, emissionSet("routed"), clusters, versions)

	assert.False(t, filtered)
	assert.Equal(t, clusters.Items, got.Items)
	assert.Equal(t, versions, gotVersions)
}

// TestDisabledScopingLeavesTheGatewayProjectionEmpty: with scoping off the set
// stays zero, so it never affects the row's equality.
func TestDisabledScopingLeavesTheGatewayProjectionEmpty(t *testing.T) {
	assert.True(t,
		emittedClustersFor(clusterScoping{}, envoycache.Resources{}, envoycache.Resources{}).
			Equals(emittedClusters{}),
		"a disabled projection must equal the zero value, so it never invalidates the row")
}
