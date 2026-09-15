package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// routedClusterWrapperV is a coherent build whose routes reference exactly the
// clusters it emits: the shape a retarget produces.
func routedClusterWrapperV(version string, clusters ...string) XdsSnapWrapper {
	wrap := clusterWrapperV(version, clusters...)
	wrap.referencedClusters = make(map[string]struct{}, len(clusters))
	for _, name := range clusters {
		wrap.referencedClusters[name] = struct{}{}
	}
	return wrap
}

func newReferenceAheadTranslator(t *testing.T, ahead time.Duration) *ProxyTranslator {
	pt := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, time.Hour, true, scopedClustersWithReferenceAhead(ahead))
	return &pt
}

func publishedListenerVersionOf(t *testing.T, pt *ProxyTranslator) string {
	t.Helper()
	snap, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	require.NoError(t, err)
	return snap.GetVersion(resourcev3.ListenerType)
}

// TestReferenceAheadDeliversTheClusterBeforeTheRoute is the addition-side
// guarantee. Publishing a new cluster and the route that targets it in one
// snapshot does not make Envoy apply CDS first: after a CDS response is sent
// its watch is closed until Envoy ACKs, so a route landing in that window
// reaches the wire on the still-open RDS watch, before its destination. The new
// cluster must therefore go out while the routing types stay where they were.
func TestReferenceAheadDeliversTheClusterBeforeTheRoute(t *testing.T) {
	pt := newReferenceAheadTranslator(t, 80*time.Millisecond)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	// The route retargets onto a cluster this client has never been sent.
	pt.syncXds(ctx, routedClusterWrapperV("v2", "old", "new"))

	assert.ElementsMatch(t, []string{"old", "new"}, publishedClusterNames(t, pt),
		"the new destination must be delivered immediately")
	assert.Equal(t, "v1", publishedListenerVersionOf(t, pt),
		"the routing types must stay at the version the client already has until the cluster has landed")

	require.Eventually(t, func() bool {
		return publishedListenerVersionOf(t, pt) == "v2"
	}, 2*time.Second, 5*time.Millisecond,
		"the hold must release on its own window, not wait for the publish budget")
	assert.ElementsMatch(t, []string{"old", "new"}, publishedClusterNames(t, pt))
}

// TestReferenceAheadIsDisabledByZero: an operator who accepts the retarget blip
// gets cluster and route in one snapshot, exactly as before.
func TestReferenceAheadIsDisabledByZero(t *testing.T) {
	pt := newReferenceAheadTranslator(t, 0)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	pt.syncXds(ctx, routedClusterWrapperV("v2", "old", "new"))

	assert.Equal(t, "v2", publishedListenerVersionOf(t, pt), "no hold is applied")
	assert.ElementsMatch(t, []string{"old", "new"}, publishedClusterNames(t, pt))
}

// TestReferenceAheadDoesNotHoldARetargetOntoAnAlreadySentCluster is what keeps
// the hold from taxing ordinary route edits: the destination is already in the
// client's CDS, so there is nothing to deliver ahead of anything.
func TestReferenceAheadDoesNotHoldARetargetOntoAnAlreadySentCluster(t *testing.T) {
	pt := newReferenceAheadTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "a", "b"))
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	// Routes change, but every destination was already delivered.
	pt.syncXds(ctx, routedClusterWrapperV("v2", "a", "b"))

	assert.Equal(t, "v2", publishedListenerVersionOf(t, pt),
		"a retarget between clusters the client already has must not be held")
}

// TestNewlyEmittedReferencesIgnoresTheBlackhole: routes whose backends fail
// resolution target the blackhole cluster. Holding on it would delay exactly
// the configurations that are already broken.
func TestNewlyEmittedReferencesIgnoresTheBlackhole(t *testing.T) {
	wrap := routedClusterWrapperV("v1", wellknown.BlackholeClusterName)
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v0", nil)

	assert.Empty(t, newlyEmittedReferences(wrap, published))
}

// TestNewlyEmittedReferencesIgnoresClustersAbsentFromTheBuild: a referenced
// cluster missing from this build is the deferred path's case, and it is
// already held there. Reporting it here too would hold it twice, on two
// different windows.
func TestNewlyEmittedReferencesIgnoresClustersAbsentFromTheBuild(t *testing.T) {
	wrap := routedClusterWrapperV("v1", "present")
	wrap.referencedClusters["absent"] = struct{}{}
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v0", nil)

	assert.Equal(t, []string{"present"}, newlyEmittedReferences(wrap, published))
}

// TestReferenceAheadHoldIsBoundedByTheBudget: the window paces delivery, the
// budget bounds it. A window longer than the budget must not outlast the
// budget, or a misconfigured value would pin a client's routing updates.
func TestReferenceAheadHoldIsBoundedByTheBudget(t *testing.T) {
	translator := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, 60*time.Millisecond, true, scopedClustersWithReferenceAhead(time.Hour))
	pt := &translator
	ctx := context.Background()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	pt.syncXds(ctx, routedClusterWrapperV("v2", "old", "new"))
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	require.Eventually(t, func() bool {
		return publishedListenerVersionOf(t, pt) == "v2"
	}, 2*time.Second, 5*time.Millisecond,
		"a reference-ahead window longer than the budget must still release at the budget")
}
