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
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

// TestClusterScopingIsOffByDefault: an upgrade must not change what proxies are sent.
func TestClusterScopingIsOffByDefault(t *testing.T) {
	assert.False(t, clusterScopingFrom(apisettings.Settings{}, nil).ScopesClusters(),
		"the zero Settings must not scope clusters")
	assert.False(t, clusterScopingFrom(apisettings.Settings{
		XdsScopingMode: apisettings.XdsScopingAll,
	}, nil).ScopesClusters())
	assert.True(t, clusterScopingFrom(apisettings.Settings{
		XdsScopingMode: apisettings.XdsScopingReferenced,
	}, nil).ScopesClusters())
}

// TestUnscopedModeRunsNoPluginClaims: without REFERENCED, no plugin's claim
// code runs and the transition windows are off, whatever they are set to.
func TestUnscopedModeRunsNoPluginClaims(t *testing.T) {
	plugins := sdk.ContributesPolicies{
		{Group: "test", Kind: "Picker"}: {
			Name: "picker",
			ClaimEmittedClusters: func() sdk.ClusterEmissionClaim {
				t.Error("a claim was collected while cluster scoping is off")
				return sdk.ClusterEmissionClaim{}
			},
		},
	}
	for name, mode := range map[string]apisettings.XdsScopingMode{"zero": "", "ALL": apisettings.XdsScopingAll} {
		t.Run(name, func(t *testing.T) {
			scoping := clusterScopingFrom(apisettings.Settings{
				XdsScopingMode:          mode,
				ClusterDereferenceGrace: time.Minute,
				ClusterReferenceAhead:   time.Minute,
			}, plugins)
			assert.False(t, scoping.ScopesClusters())
			assert.Zero(t, scoping.DereferenceGrace())
			assert.Zero(t, scoping.ReferenceAhead())
			assert.Empty(t, scoping.Claims().bySelector)
		})
	}
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
	assert.Empty(t, disabled.Names, "nothing is collected when xDS scoping is off")
	assert.Empty(t, disabled.RequestTimeSelectors)

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

// TestDisabledScopingHasNoTransitionWindows: unscoped xDS ignores both
// configured windows, so coherent builds take the plain publish path.
func TestDisabledScopingHasNoTransitionWindows(t *testing.T) {
	configured := clusterScopingFrom(apisettings.Settings{
		XdsScopingMode:          apisettings.XdsScopingAll,
		ClusterDereferenceGrace: time.Hour,
		ClusterReferenceAhead:   time.Hour,
	}, nil)

	assert.Zero(t, configured.DereferenceGrace(),
		"a de-reference window is meaningless when no cluster ever leaves the emitted set")
	assert.Zero(t, configured.ReferenceAhead(),
		"a reference-ahead window is meaningless when every cluster was delivered long ago")

	gate := newPublishGate(time.Minute, false, nil, configured)
	assert.False(t, gate.appliesTransitionGraces(),
		"the coherent publish path must be untouched when xDS scoping is off")
}

// TestScopedClustersUsesTheConfiguredWindows: with scoping on, the configured
// durations must reach the gate.
func TestScopedClustersUsesTheConfiguredWindows(t *testing.T) {
	configured := clusterScopingFrom(apisettings.Settings{
		XdsScopingMode:          apisettings.XdsScopingReferenced,
		ClusterDereferenceGrace: 7 * time.Second,
		ClusterReferenceAhead:   3 * time.Second,
	}, nil)

	assert.Equal(t, 7*time.Second, configured.DereferenceGrace())
	assert.Equal(t, 3*time.Second, configured.ReferenceAhead())

	gate := newPublishGate(time.Minute, false, nil, configured)
	assert.True(t, gate.appliesTransitionGraces())
}
