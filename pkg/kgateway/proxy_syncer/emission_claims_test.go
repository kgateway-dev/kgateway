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

// TestClaimedSelectorKeepsTheGatewayScoped is the point of the whole mechanism.
// An unclaimed request-time selector costs the gateway the optimization
// everywhere; claiming it keeps scoping on for every other backend.
func TestClaimedSelectorKeepsTheGatewayScoped(t *testing.T) {
	emission := emittedClusters{
		Names:                map[string]struct{}{"routed": {}},
		RequestTimeSelectors: []requestTimeSelector{{selectorSpecifierPlugin, "picker"}},
	}

	assert.False(t, emission.Filterable(emissionClaims{}),
		"an unclaimed selector must cost the gateway its scoping")

	claimed := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{"picker"},
	}))
	assert.True(t, emission.Filterable(claimed),
		"the plugin that owns the selector accounted for it")
}

// TestClaimedSelectorsAreMatchedByName: a claim accounts for the selector it
// names and nothing else, so one plugin's claim cannot silently cover another
// plugin's unbounded selector.
func TestClaimedSelectorsAreMatchedByName(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{"picker"},
	}))

	assert.True(t, claims.accountsFor(requestTimeSelector{selectorSpecifierPlugin, "picker"}))
	assert.True(t, claims.accountsFor(requestTimeSelector{selectorInlinePlugin, "picker"}),
		"the inline form of the same extension is the same selector")
	assert.False(t, claims.accountsFor(requestTimeSelector{selectorSpecifierPlugin, "other"}))
}

// TestClusterHeaderCanNeverBeClaimed: the destination is whatever the client
// puts in the header, so no declaration can bound the set of clusters it might
// name. A gateway with one stays unscoped however much is claimed.
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

// TestClaimedClustersSurviveTheFilter covers what a claim buys once the gateway
// is scoped: the candidates the plugin may compute, and its own placeholder,
// stay in CDS even though nothing names them.
func TestClaimedClustersSurviveTheFilter(t *testing.T) {
	scoping := scopedClusters()
	scoping.claims = collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors:    []string{"picker"},
		Names:        []string{"picker-fallback"},
		NamePrefixes: []string{"tenant-"},
	}))

	clusters, versions := clusterResourcesFor(
		"routed", "picker-fallback", "tenant-a", "tenant-b", "unreferenced")

	got, _, filtered := filterClustersToEmitted(scoping, emissionSet("routed"), clusters, versions)

	require.True(t, filtered)
	assert.ElementsMatch(t,
		[]string{"routed", "picker-fallback", "tenant-a", "tenant-b"},
		resourceNames(got),
		"claimed names and prefixes are kept; only the genuinely unreferenced backend goes")
}

// TestEmptyPrefixClaimIsRejected: a prefix matching everything re-admits the
// whole inventory while still reporting the gateway as scoped, which is worse
// than reverting, because it is invisible.
func TestEmptyPrefixClaimIsRejected(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors:    []string{"picker"},
		NamePrefixes: []string{""},
	}))

	assert.Empty(t, claims.prefixes)
	assert.False(t, claims.claims("anything-at-all"))
	assert.True(t, claims.accountsFor(requestTimeSelector{selectorSpecifierPlugin, "picker"}),
		"the selector is still accounted for; only the unbounded prefix was dropped")
}

// TestNoClaimsIsTheOrdinaryCase: nearly every plugin routes only where the
// configuration says, registers nothing, and must cost nothing.
func TestNoClaimsIsTheOrdinaryCase(t *testing.T) {
	claims := collectEmissionClaims(sdk.ContributesPolicies{
		schema.GroupKind{Group: "test", Kind: "Ordinary"}: {Name: "ordinary"},
	})

	assert.True(t, claims.empty())
	assert.False(t, claims.claims("anything"))
	assert.True(t, emittedClusters{Names: map[string]struct{}{"a": {}}}.Filterable(claims),
		"a gateway with no request-time selectors is scoped regardless of claims")
}
