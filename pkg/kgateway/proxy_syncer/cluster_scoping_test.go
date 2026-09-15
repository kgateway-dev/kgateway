package proxy_syncer

import (
	"testing"
	"time"

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
	assert.False(t, clusterScopingFrom(apisettings.Settings{}, nil).ScopesClusters(),
		"the zero Settings must not scope clusters")
	assert.False(t, clusterScopingFrom(apisettings.Settings{
		ClusterDiscoveryMode: apisettings.ClusterDiscoveryAll,
	}, nil).ScopesClusters())
	assert.True(t, clusterScopingFrom(apisettings.Settings{
		ClusterDiscoveryMode: apisettings.ClusterDiscoveryReferenced,
	}, nil).ScopesClusters())
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
	assert.Empty(t, disabled.RequestTimeSelectors)

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

// TestDisabledScopingHasNoTransitionWindows completes the off-switch: with CDS
// unscoped, neither window is in force, whatever the durations are set to.
// Nothing is ever de-referenced and no cluster is ever new to a client, so a
// gate built from this configuration keeps no per-client transition state and
// every publish takes the path it took before the feature existed.
func TestDisabledScopingHasNoTransitionWindows(t *testing.T) {
	configured := clusterScopingFrom(apisettings.Settings{
		ClusterDiscoveryMode:    apisettings.ClusterDiscoveryAll,
		ClusterDereferenceGrace: time.Hour,
		ClusterReferenceAhead:   time.Hour,
	}, nil)

	assert.Zero(t, configured.DereferenceGrace(),
		"a de-reference window is meaningless when no cluster ever leaves the emitted set")
	assert.Zero(t, configured.ReferenceAhead(),
		"a reference-ahead window is meaningless when every cluster was delivered long ago")

	gate := newPublishGate(time.Minute, false, configured)
	assert.False(t, gate.appliesTransitionGraces(),
		"the coherent publish path must be untouched when CDS is not scoped")
}

// TestScopedClustersUsesTheConfiguredWindows is the negative control: the
// durations must actually reach the gate once scoping is on, or the windows
// would silently never apply.
func TestScopedClustersUsesTheConfiguredWindows(t *testing.T) {
	configured := clusterScopingFrom(apisettings.Settings{
		ClusterDiscoveryMode:    apisettings.ClusterDiscoveryReferenced,
		ClusterDereferenceGrace: 7 * time.Second,
		ClusterReferenceAhead:   3 * time.Second,
	}, nil)

	assert.Equal(t, 7*time.Second, configured.DereferenceGrace())
	assert.Equal(t, 3*time.Second, configured.ReferenceAhead())

	gate := newPublishGate(time.Minute, false, configured)
	assert.True(t, gate.appliesTransitionGraces())
}
