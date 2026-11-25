package proxy_syncer

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"

	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

// TestDecideXdsScopingRoundTripsADerivedPrefix is how a plugin tests its
// claim: generate configuration for a namespace, decide, and expect exactly
// that namespace's clusters kept. The prefix is exact because neither
// namespaces nor backend names contain "_".
func TestDecideXdsScopingRoundTripsADerivedPrefix(t *testing.T) {
	plugins := claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{destinationSelector}, DestinationsFor: destinationsFromMarker,
	})

	decision := DecideXdsScoping(plugins,
		[]*envoyroutev3.RouteConfiguration{inlineSpecifierRouteConfig(t, destinationLua("gwtest"))}, nil)

	assert.True(t, decision.Scoped, "unaccounted: %v", decision.Unaccounted)
	assert.True(t, decision.Emits("backend_gwtest_echo"))
	assert.False(t, decision.Emits("backend_gwtest2_echo"), "another namespace sharing the prefix's start")
	assert.False(t, decision.Emits("backend_other_echo"))
}

// TestDecideXdsScopingReportsWhyAGatewayIsUnscoped: the decision names the
// unaccounted selector and reason the controller would log, and an unscoped
// gateway emits everything.
func TestDecideXdsScopingReportsWhyAGatewayIsUnscoped(t *testing.T) {
	plugins := claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{destinationSelector}, DestinationsFor: destinationsFromMarker,
	})

	decision := DecideXdsScoping(plugins,
		[]*envoyroutev3.RouteConfiguration{inlineSpecifierRouteConfig(t, foreignLua())}, nil)

	assert.False(t, decision.Scoped)
	assert.Equal(t,
		[]string{`inline_cluster_specifier_plugin "example.dev/destination" (claim derived no destinations)`},
		decision.Unaccounted)
	assert.True(t, decision.Emits("anything"))
}
