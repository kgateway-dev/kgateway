package proxy_syncer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

func claimingPlugins(claim sdk.ClusterEmissionClaim) sdk.ContributesPolicies {
	return sdk.ContributesPolicies{
		{Group: "test", Kind: "Picker"}: {
			Name:                 "picker",
			ClaimEmittedClusters: func() sdk.ClusterEmissionClaim { return claim },
		},
	}
}

// TestClaimedSelectorKeepsTheGatewayScoped: an unclaimed request-time selector
// unscopes the whole gateway; claiming it restores scoping.
func TestClaimedSelectorKeepsTheGatewayScoped(t *testing.T) {
	emission := emittedClusters{
		Names:                map[string]struct{}{"routed": {}},
		RequestTimeSelectors: []requestTimeSelector{{selectorSpecifierPlugin, "picker"}},
	}

	assert.False(t, emission.Filterable(emissionClaims{}),
		"an unclaimed selector must cost the gateway its scoping")

	claimed := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors:    []string{"picker"},
		NamePrefixes: []string{"picker_"},
	}))
	assert.True(t, emission.Filterable(claimed),
		"the plugin that owns the selector accounted for it")
}

// TestClaimedSelectorsAreMatchedByName: a claim covers only the selectors it
// names, never another plugin's.
func TestClaimedSelectorsAreMatchedByName(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors:    []string{"picker"},
		NamePrefixes: []string{"picker_"},
	}))

	assert.True(t, claims.accountsFor(requestTimeSelector{selectorSpecifierPlugin, "picker"}))
	assert.True(t, claims.accountsFor(requestTimeSelector{selectorInlinePlugin, "picker"}),
		"the inline form of the same extension is the same selector")
	assert.False(t, claims.accountsFor(requestTimeSelector{selectorSpecifierPlugin, "other"}))
}

// TestTwoPluginsClaimingOneNameShareItsDestinations: claims match on the
// extension name alone, so two plugins claiming one name each have their
// destinations kept wherever either uses it. The collision is logged.
func TestTwoPluginsClaimingOneNameShareItsDestinations(t *testing.T) {
	claims := collectEmissionClaims(sdk.ContributesPolicies{
		{Group: "a.test", Kind: "Picker"}: {
			Name: "a",
			ClaimEmittedClusters: func() sdk.ClusterEmissionClaim {
				return sdk.ClusterEmissionClaim{Selectors: []string{"shared"}, NamePrefixes: []string{"a_"}}
			},
		},
		{Group: "b.test", Kind: "Picker"}: {
			Name: "b",
			ClaimEmittedClusters: func() sdk.ClusterEmissionClaim {
				return sdk.ClusterEmissionClaim{Selectors: []string{"shared"}, Names: []string{"b-fallback"}}
			},
		},
	})

	kept := claims.keptFor(emittedClusters{RequestTimeSelectors: []requestTimeSelector{{selectorSpecifierPlugin, "shared"}}})
	assert.True(t, kept.claims("a_backend"))
	assert.True(t, kept.claims("b-fallback"))
}

// TestClusterHeaderCanNeverBeClaimed: the client picks the header's
// destination, so no claim can bound it and the gateway stays unscoped.
func TestClusterHeaderCanNeverBeClaimed(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors:    []string{"picker", "x-target"},
		NamePrefixes: []string{"tenant-"},
	}))

	assert.False(t, claims.accountsFor(requestTimeSelector{selectorClusterHeader, "x-target"}))

	emission := emittedClusters{
		Names:                map[string]struct{}{"routed": {}},
		RequestTimeSelectors: []requestTimeSelector{{selectorClusterHeader, "x-target"}},
	}
	assert.False(t, emission.Filterable(claims))
	assert.Equal(t, []string{`cluster_header "x-target"`}, emission.unaccountedSelectors(claims))
}

// pickerGateway routes to routed and uses the picker cluster specifier plugin.
func pickerGateway() emittedClusters {
	set := emissionSet("routed")
	set.RequestTimeSelectors = []requestTimeSelector{{selectorSpecifierPlugin, "picker"}}
	return set
}

// TestClaimedClustersSurviveTheFilter: a scoped gateway keeps the plugin's
// claimed candidates and placeholder though nothing names them.
func TestClaimedClustersSurviveTheFilter(t *testing.T) {
	scoping := scopedClusters()
	scoping.claims = collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors:    []string{"picker"},
		Names:        []string{"picker-fallback"},
		NamePrefixes: []string{"tenant-"},
	}))

	clusters, versions := clusterResourcesFor(
		"routed", "picker-fallback", "tenant-a", "tenant-b", "unreferenced")

	got, _, filtered := filterClustersToEmitted(scoping, pickerGateway(), clusters, versions)

	require.True(t, filtered)
	assert.ElementsMatch(t,
		[]string{"routed", "picker-fallback", "tenant-a", "tenant-b"},
		resourceNames(got),
		"claimed names and prefixes are kept; only the genuinely unreferenced backend goes")
}

// TestClaimsKeepNothingInAGatewayWithoutTheirSelector: controller-wide claims
// must not grow gateways that never use the plugin.
func TestClaimsKeepNothingInAGatewayWithoutTheirSelector(t *testing.T) {
	scoping := scopedClusters()
	scoping.claims = collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors:    []string{"picker"},
		Names:        []string{"picker-fallback"},
		NamePrefixes: []string{"tenant-"},
	}))

	clusters, versions := clusterResourcesFor("routed", "picker-fallback", "tenant-a", "unreferenced")
	got, _, filtered := filterClustersToEmitted(scoping, emissionSet("routed"), clusters, versions)

	require.True(t, filtered)
	assert.ElementsMatch(t, []string{"routed"}, resourceNames(got),
		"a gateway without the picker selector keeps only what its configuration names")
}

// TestClaimWithoutSelectorsKeepsNothing: a selector-less claim is dropped, not
// read as keep-everywhere.
func TestClaimWithoutSelectorsKeepsNothing(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Names:        []string{"picker-fallback"},
		NamePrefixes: []string{"tenant-"},
	}))

	assert.True(t, claims.empty())
	assert.False(t, claims.keptFor(pickerGateway()).claims("tenant-a"))
}

// TestEmptyPrefixClaimIsRejected: an empty prefix would silently re-admit every
// cluster while the gateway reports scoped.
func TestEmptyPrefixClaimIsRejected(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors:    []string{"picker"},
		NamePrefixes: []string{""},
	}))

	assert.True(t, claims.empty())
	assert.False(t, claims.keptFor(pickerGateway()).claims("anything-at-all"))
	assert.False(t, claims.accountsFor(requestTimeSelector{selectorSpecifierPlugin, "picker"}),
		"with its prefix dropped the claim keeps none of the selector's candidates, so it must not account for the selector")
}

// TestClaimWithoutDestinationsDoesNotAccountForItsSelectors: otherwise the
// plugin's computed clusters are pruned and it silently falls back on every
// request.
func TestClaimWithoutDestinationsDoesNotAccountForItsSelectors(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{"picker"},
	}))

	assert.False(t, claims.accountsFor(requestTimeSelector{selectorSpecifierPlugin, "picker"}))
	assert.False(t, emittedClusters{
		Names:                map[string]struct{}{"a": {}},
		RequestTimeSelectors: []requestTimeSelector{{selectorSpecifierPlugin, "picker"}},
	}.Filterable(claims), "its gateway stays unscoped")
}

// TestOnlySpecifierPluginsCanBeClaimed: other selector kinds are never
// accounted for, even under a claimed name.
func TestOnlySpecifierPluginsCanBeClaimed(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors:    []string{"picker", "envoy.tcp_proxy.cluster", "type.googleapis.com/example.Opaque"},
		NamePrefixes: []string{"backend_"},
	}))

	assert.True(t, claims.accountsFor(requestTimeSelector{selectorSpecifierPlugin, "picker"}))
	assert.True(t, claims.accountsFor(requestTimeSelector{selectorInlinePlugin, "picker"}))
	for _, selector := range []requestTimeSelector{
		{selectorFilterState, "envoy.tcp_proxy.cluster"},
		{selectorOpaqueConfig, "type.googleapis.com/example.Opaque"},
		{Kind: selectorSniCluster},
		{selectorUnknownSpecifier, "picker"},
	} {
		assert.False(t, claims.accountsFor(selector), "%s must not be claimable", selector)
	}
}

// TestNoClaimsIsTheOrdinaryCase: a plugin that claims nothing costs nothing.
func TestNoClaimsIsTheOrdinaryCase(t *testing.T) {
	claims := collectEmissionClaims(sdk.ContributesPolicies{
		schema.GroupKind{Group: "test", Kind: "Ordinary"}: {Name: "ordinary"},
	})

	assert.True(t, claims.empty())
	assert.False(t, claims.keptFor(emittedClusters{}).claims("anything"))
	assert.True(t, emittedClusters{Names: map[string]struct{}{"a": {}}}.Filterable(claims),
		"a gateway with no request-time selectors is scoped regardless of claims")
}
