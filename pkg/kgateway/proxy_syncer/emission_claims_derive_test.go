package proxy_syncer

import (
	"strings"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/router/cluster_specifiers/lua/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

const (
	// destinationSelector is the extension name an example plugin owns.
	destinationSelector = "example.dev/destination"
	// emissionPrefixMarker starts a script line the example plugin writes, so
	// its claim reads the prefix back instead of parsing generated code.
	emissionPrefixMarker = "-- example-emission-prefix: "
)

// destinationLua is the config the example plugin generates for one namespace:
// a Lua cluster specifier composing backend_<namespace>_<name>.
func destinationLua(namespace string) *luav3.LuaConfig {
	prefix := "backend_" + namespace + "_"
	return &luav3.LuaConfig{
		DefaultCluster: wellknown.BlackholeClusterName,
		SourceCode: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_InlineString{
			InlineString: emissionPrefixMarker + prefix + "\n" +
				"function envoy_on_route(handle) return \"" + prefix + "\" .. handle:headers():get(\"x-backend\") end\n",
		}},
	}
}

// foreignLua is another plugin's config under the same extension name.
func foreignLua() *luav3.LuaConfig {
	return &luav3.LuaConfig{SourceCode: &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_InlineString{
		InlineString: "function envoy_on_route(handle) return handle:headers():get(\"x-anything\") end\n",
	}}}
}

// destinationsFromMarker is the example plugin's DestinationsFor. It confirms
// the use is its own, by name, type and marker line, then reads the prefix.
func destinationsFromMarker(use sdk.SelectorUse) sdk.ClaimedDestinations {
	lua, ok := use.Config.(*luav3.LuaConfig)
	if use.Name != destinationSelector || !ok {
		return sdk.ClaimedDestinations{}
	}
	for line := range strings.SplitSeq(lua.GetSourceCode().GetInlineString(), "\n") {
		if prefix, found := strings.CutPrefix(line, emissionPrefixMarker); found {
			return sdk.ClaimedDestinations{NamePrefixes: []string{prefix}}
		}
	}
	return sdk.ClaimedDestinations{}
}

func scopedWithClaims(claims emissionClaims) clusterScoping {
	scoping := scopedClusters()
	scoping.claims = claims
	return scoping
}

// inlineSpecifierRoutes is one RouteConfiguration with a route per config, each
// using the inline form of the destination selector.
func inlineSpecifierRoutes(t *testing.T, configs ...proto.Message) envoycache.Resources {
	t.Helper()
	var routes []*envoyroutev3.Route
	for _, config := range configs {
		routes = append(routes, specifierRoute(&envoyroutev3.RouteAction{
			ClusterSpecifier: &envoyroutev3.RouteAction_InlineClusterSpecifierPlugin{
				InlineClusterSpecifierPlugin: &envoyroutev3.ClusterSpecifierPlugin{Extension: &envoycorev3.TypedExtensionConfig{
					Name: destinationSelector, TypedConfig: mustMessageToAny(t, config),
				}},
			},
		}))
	}
	return sliceToResources([]*envoyroutev3.RouteConfiguration{{
		Name:         "http",
		VirtualHosts: []*envoyroutev3.VirtualHost{{Name: "vh", Domains: []string{"*"}, Routes: routes}},
	}})
}

func specifierRoute(action *envoyroutev3.RouteAction) *envoyroutev3.Route {
	return &envoyroutev3.Route{
		Match:  &envoyroutev3.RouteMatch{PathSpecifier: &envoyroutev3.RouteMatch_Prefix{Prefix: "/"}},
		Action: &envoyroutev3.Route_Route{Route: action},
	}
}

// TestDerivedClaimKeepsEachUsesOwnDestinations: uses of one extension name
// with different configs stay apart, and each keeps only its own namespace.
func TestDerivedClaimKeepsEachUsesOwnDestinations(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{destinationSelector}, DestinationsFor: destinationsFromMarker,
	}))

	emitted := emittedClustersFor(scopedWithClaims(claims), inlineSpecifierRoutes(t, destinationLua("ns1"), destinationLua("ns2")), envoycache.Resources{})

	require.Len(t, emitted.SpecifierUses, 2, "one use per distinct config")
	assert.True(t, emitted.Filterable(claims))
	kept := claims.keptFor(emitted)
	assert.True(t, kept.claims("backend_ns1_a"))
	assert.True(t, kept.claims("backend_ns2_b"))
	assert.False(t, kept.claims("backend_ns3_c"), "a namespace no selector uses is not kept")
}

// TestDerivedClaimDeclinesAConfigItDidNotGenerate: another plugin's specifier
// under the same name leaves the gateway unscoped instead of having its
// candidates pruned, with a reason that names the claim.
func TestDerivedClaimDeclinesAConfigItDidNotGenerate(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{destinationSelector}, DestinationsFor: destinationsFromMarker,
	}))

	emitted := emittedClustersFor(scopedWithClaims(claims), inlineSpecifierRoutes(t, destinationLua("ns1"), foreignLua()), envoycache.Resources{})

	assert.False(t, emitted.Filterable(claims))
	assert.Equal(t,
		[]string{`inline_cluster_specifier_plugin "example.dev/destination" (claim derived no destinations)`},
		emitted.unaccountedSelectors(claims))
}

// TestDerivedClaimStaticListsComeOnlyWithAClaimedUse: with DestinationsFor
// set, the claim's own Names do not account for a use the function declined.
func TestDerivedClaimStaticListsComeOnlyWithAClaimedUse(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors:       []string{destinationSelector},
		Names:           []string{"destination-placeholder"},
		DestinationsFor: destinationsFromMarker,
	}))

	own := emittedClustersFor(scopedWithClaims(claims), inlineSpecifierRoutes(t, destinationLua("ns1")), envoycache.Resources{})
	require.True(t, own.Filterable(claims))
	assert.True(t, claims.keptFor(own).claims("destination-placeholder"), "kept with a claimed use")

	foreign := emittedClustersFor(scopedWithClaims(claims), inlineSpecifierRoutes(t, foreignLua()), envoycache.Resources{})
	assert.False(t, foreign.Filterable(claims), "the static Names must not account for a declined use")
}

// TestDerivedClaimFailuresLeaveTheGatewayUnscoped: plugin code runs where
// snapshots are built; a panic or a bad result leaves the use unclaimed.
func TestDerivedClaimFailuresLeaveTheGatewayUnscoped(t *testing.T) {
	for name, tc := range map[string]struct {
		derive func(sdk.SelectorUse) sdk.ClaimedDestinations
		reason string
	}{
		"panic": {func(sdk.SelectorUse) sdk.ClaimedDestinations { panic("plugin bug") }, claimPanicked},
		"empty prefix": {func(sdk.SelectorUse) sdk.ClaimedDestinations {
			return sdk.ClaimedDestinations{NamePrefixes: []string{"backend_", ""}}
		}, claimDerivedEmptyPrefix},
		"only empty names": {func(sdk.SelectorUse) sdk.ClaimedDestinations {
			return sdk.ClaimedDestinations{Names: []string{""}}
		}, claimDerivedNothing},
	} {
		t.Run(name, func(t *testing.T) {
			claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
				Selectors: []string{destinationSelector}, DestinationsFor: tc.derive,
			}))

			var emitted emittedClusters
			require.NotPanics(t, func() {
				emitted = emittedClustersFor(scopedWithClaims(claims), inlineSpecifierRoutes(t, destinationLua("ns1")), envoycache.Resources{})
			})

			assert.False(t, emitted.Filterable(claims))
			assert.Equal(t,
				[]string{`inline_cluster_specifier_plugin "example.dev/destination" (` + tc.reason + `)`},
				emitted.unaccountedSelectors(claims))
		})
	}
}

// TestDerivedClaimResolvesReferencedPlugins: a route that names a plugin
// defined in its RouteConfiguration gets that definition's config; a reference
// with no definition stays unclaimed.
func TestDerivedClaimResolvesReferencedPlugins(t *testing.T) {
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{destinationSelector}, DestinationsFor: destinationsFromMarker,
	}))
	referenced := func(defined bool) envoycache.Resources {
		routeConfig := &envoyroutev3.RouteConfiguration{
			Name: "http",
			VirtualHosts: []*envoyroutev3.VirtualHost{{Name: "vh", Domains: []string{"*"}, Routes: []*envoyroutev3.Route{
				specifierRoute(&envoyroutev3.RouteAction{
					ClusterSpecifier: &envoyroutev3.RouteAction_ClusterSpecifierPlugin{ClusterSpecifierPlugin: destinationSelector},
				}),
			}}},
		}
		if defined {
			routeConfig.ClusterSpecifierPlugins = []*envoyroutev3.ClusterSpecifierPlugin{{Extension: &envoycorev3.TypedExtensionConfig{
				Name: destinationSelector, TypedConfig: mustMessageToAny(t, destinationLua("ns1")),
			}}}
		}
		return sliceToResources([]*envoyroutev3.RouteConfiguration{routeConfig})
	}

	emitted := emittedClustersFor(scopedWithClaims(claims), referenced(true), envoycache.Resources{})
	require.True(t, emitted.Filterable(claims))
	assert.True(t, claims.keptFor(emitted).claims("backend_ns1_a"))

	dangling := emittedClustersFor(scopedWithClaims(claims), referenced(false), envoycache.Resources{})
	assert.Equal(t,
		[]string{`cluster_specifier_plugin "example.dev/destination" (reference does not resolve)`},
		dangling.unaccountedSelectors(claims))
}

// TestDerivationRunsOncePerDistinctConfig: routes repeating a config share one
// decision, so the function runs per distinct config, not per route.
func TestDerivationRunsOncePerDistinctConfig(t *testing.T) {
	calls := 0
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{destinationSelector},
		DestinationsFor: func(use sdk.SelectorUse) sdk.ClaimedDestinations {
			calls++
			return destinationsFromMarker(use)
		},
	}))

	emittedClustersFor(scopedWithClaims(claims),
		inlineSpecifierRoutes(t, destinationLua("ns1"), destinationLua("ns1"), destinationLua("ns1"), destinationLua("ns2")),
		envoycache.Resources{})

	assert.Equal(t, 2, calls)
}

// TestDerivedClaimGetsItsOwnCopy: what one call does to its config does not
// reach the next.
func TestDerivedClaimGetsItsOwnCopy(t *testing.T) {
	derive := func(use sdk.SelectorUse) sdk.ClaimedDestinations {
		got := destinationsFromMarker(use)
		use.Config.(*luav3.LuaConfig).SourceCode = nil
		return got
	}
	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{destinationSelector}, DestinationsFor: derive,
	}))
	emitted := collectReferencedClustersForEmission(inlineSpecifierRoutes(t, destinationLua("ns1")), envoycache.Resources{})
	require.Len(t, emitted.SpecifierUses, 1)

	first := claims.deriveUse(emitted.SpecifierUses[0].specifierUseKey)
	second := claims.deriveUse(emitted.SpecifierUses[0].specifierUseKey)

	assert.True(t, first.claimed())
	assert.True(t, second.equal(first))
}

// TestDerivingClaimNeedsNoStaticDestinations: a claim with DestinationsFor is
// valid with no Names or NamePrefixes, but an empty static prefix still rejects
// it.
func TestDerivingClaimNeedsNoStaticDestinations(t *testing.T) {
	selector := requestTimeSelector{selectorInlinePlugin, destinationSelector}

	claims := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{destinationSelector}, DestinationsFor: destinationsFromMarker,
	}))
	assert.True(t, claims.derives(selector))
	assert.False(t, claims.accountsFor(selector), "only a static claim covers every use")

	rejected := collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{destinationSelector}, NamePrefixes: []string{""}, DestinationsFor: destinationsFromMarker,
	}))
	assert.True(t, rejected.empty())
}

// TestEmittedClustersEqualsComparesSpecifierUses: a changed config or decision
// must invalidate the projection.
func TestEmittedClustersEqualsComparesSpecifierUses(t *testing.T) {
	use := specifierUse{specifierUseKey: specifierUseKey{
		Selector: requestTimeSelector{selectorInlinePlugin, destinationSelector}, ConfigType: "t", Config: "a",
	}}
	base := emittedClusters{SpecifierUses: []specifierUse{use}}

	assert.True(t, base.Equals(emittedClusters{SpecifierUses: []specifierUse{use}}))

	otherConfig := use
	otherConfig.Config = "b"
	assert.False(t, base.Equals(emittedClusters{SpecifierUses: []specifierUse{otherConfig}}))

	decided := use
	decided.Derived = claimedUse{Prefixes: []string{"backend_ns1_"}}
	assert.False(t, base.Equals(emittedClusters{SpecifierUses: []specifierUse{decided}}))
}
