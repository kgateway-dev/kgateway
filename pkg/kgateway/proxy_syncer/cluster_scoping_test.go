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

// TestClusterScopingIsOffByDefault: an upgrade must not change what any proxy
// is sent. The zero Settings, and the documented default, both leave CDS
// unscoped.
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

// TestDisabledScopingSkipsTheEmissionWalkEntirely pins that turning the feature
// off is not just "compute the set and ignore it". The walk visits every
// message of every generated listener and route, descending into typed_config,
// and a deployment that has not opted in should not pay for it.
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

// TestDisabledScopingFilterIsIdentity: with the feature off the filter returns
// its input unchanged, including the case where the emitted set would have
// dropped something. Nothing about a published snapshot depends on the emitted
// set unless an operator asked for it.
func TestDisabledScopingFilterIsIdentity(t *testing.T) {
	clusters, versions := clusterResourcesFor("routed", "unreferenced")

	got, gotVersions, filtered := filterClustersToEmitted(
		clusterScoping{}, emissionSet("routed"), clusters, versions)

	assert.False(t, filtered)
	assert.Equal(t, clusters.Items, got.Items)
	assert.Equal(t, versions, gotVersions)
}

// TestDisabledScopingLeavesTheGatewayProjectionEmpty: the emitted set rides on
// GatewayXdsResources, and with scoping off it must stay zero so it cannot
// contribute to that row's equality or be mistaken for "no clusters emitted".
func TestDisabledScopingLeavesTheGatewayProjectionEmpty(t *testing.T) {
	assert.True(t,
		emittedClustersFor(clusterScoping{}, envoycache.Resources{}, envoycache.Resources{}).
			Equals(emittedClusters{}),
		"a disabled projection must equal the zero value, so it never invalidates the row")
}
