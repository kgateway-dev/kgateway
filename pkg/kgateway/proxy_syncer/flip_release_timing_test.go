package proxy_syncer

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowInstallCache delays SetSnapshot while delay is set, standing in for an
// install blocked on a stream.
type slowInstallCache struct {
	envoycache.SnapshotCache
	delay atomic.Int64
}

func (c *slowInstallCache) SetSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot) error {
	time.Sleep(time.Duration(c.delay.Load()))
	return c.SnapshotCache.SetSnapshot(ctx, node, snapshot)
}

// A held flip's budget starts once its held snapshot is installed: an install
// slower than the budget must not release the flip as soon as it lands.
func TestFlipRelease_BudgetStartsAfterTheHeldSnapshotIsInstalled(t *testing.T) {
	budget := 200 * time.Millisecond
	cache := &slowInstallCache{SnapshotCache: newTestSnapshotCache(t)}
	translator := NewProxyTranslator(cache, stubPriorXDS{}, budget, true, clusterScoping{})
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	_, flipWrap := flipHoldFixture(t, pt)

	cache.delay.Store(int64(2 * budget))
	pt.syncXds(context.Background(), flipWrap)
	cache.delay.Store(0)

	require.Never(t, func() bool {
		return snapshotReferencesCluster(servedSnapshot(t, pt), "cluster-new")
	}, budget/2, 10*time.Millisecond, "the flip must not be released as soon as its held snapshot is installed")
	require.Eventually(t, func() bool {
		return snapshotReferencesCluster(servedSnapshot(t, pt), "cluster-new")
	}, 2*time.Second, 5*time.Millisecond, "the flip is released one budget after the install")
}

// With the budget off and no reference-ahead, nothing bounds a held flip, so
// the gate keeps no record of it.
func TestFlipHold_WithoutBudgetKeepsNoRecord(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 0)
	heldRouteVersion, flipWrap := flipHoldFixture(t, pt)

	pt.syncXds(context.Background(), flipWrap)
	require.Equal(t, heldRouteVersion, servedSnapshot(t, pt).GetVersion(resourcev3.RouteType), "the flip is held")
	pt.gate.mu.Lock()
	defer pt.gate.mu.Unlock()
	assert.Empty(t, pt.gate.pendingFlips)
}

// With the budget off, an unready flip joining a reference-ahead hold leaves it
// unbounded. A window release that already fired and waits on the gate lock
// while that build publishes must then do nothing.
func TestFlipHold_UnboundedHoldStopsAReleaseAlreadyFiring(t *testing.T) {
	translator := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, 0, true,
		scopedClustersWithReferenceAhead(20*time.Millisecond))
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	pt.syncXds(ctx, routedBuild(t, "v2", "new", "old", "new")) // reference-ahead hold
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt))

	unready := routedBuild(t, "v3", "unready", "old", "new", "unready")
	unready.missingEndpointsReferenced = []string{"unready"}
	unready.synthesizedEndpoints = []string{"unready"}
	pt.gate.mu.Lock()
	time.Sleep(100 * time.Millisecond) // the window's release fires and waits on the lock
	err := pt.gate.resolveDeferredLocked(ctx, pt.xdsCache, unready)
	pt.gate.mu.Unlock()
	require.NoError(t, err)

	require.Never(t, func() bool { return publishedListenerVersionOf(t, pt) != "v1" }, 300*time.Millisecond, 10*time.Millisecond,
		"with the budget off, the flip onto the unready cluster stays held")
}

// A de-reference grace starts once the route update that opens it is
// installed. An installation slower than the grace must not use it up: the
// next build still keeps the old cluster.
func TestDereferenceGraceStartsAfterASlowInstall(t *testing.T) {
	grace := 300 * time.Millisecond
	cache := &slowInstallCache{SnapshotCache: newTestSnapshotCache(t)}
	translator := NewProxyTranslator(cache, stubPriorXDS{has: true}, time.Hour, true, scopedClustersWithGrace(grace))
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	cache.delay.Store(int64(2 * grace))
	pt.syncXds(ctx, routedBuild(t, "v2", "new", "new")) // stops naming old, installed slowly
	cache.delay.Store(0)
	pt.syncXds(ctx, routedBuild(t, "v3", "new", "new"))
	assert.Contains(t, publishedClusterNames(t, pt), "old", "old keeps its grace after the slow installation")
	require.Eventually(t, func() bool { return !slices.Contains(publishedClusterNames(t, pt), "old") }, 2*time.Second, 5*time.Millisecond,
		"old still leaves once its grace closes")
}

// The same for a held flip's release: the grace opens when the released routes
// are installed, not when the release starts.
func TestDereferenceGraceStartsAfterASlowFlipReleaseInstall(t *testing.T) {
	grace := 300 * time.Millisecond
	cache := &slowInstallCache{SnapshotCache: newTestSnapshotCache(t)}
	translator := NewProxyTranslator(cache, stubPriorXDS{has: true}, 100*time.Millisecond, true, scopedClustersWithGrace(grace))
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	unready := routedBuild(t, "v2", "new", "new")
	unready.missingEndpointsReferenced = []string{"new"}
	unready.synthesizedEndpoints = []string{"new"}
	pt.syncXds(ctx, unready) // held; the budget releases it
	require.Equal(t, "v1", publishedListenerVersionOf(t, pt), "the flip onto the unready cluster is held")
	cache.delay.Store(int64(2 * grace))
	require.Eventually(t, func() bool { return publishedListenerVersionOf(t, pt) == "v2" }, 3*time.Second, 5*time.Millisecond,
		"the budget releases the flip")
	cache.delay.Store(0)
	pt.syncXds(ctx, routedBuild(t, "v3", "new", "new"))
	assert.Contains(t, publishedClusterNames(t, pt), "old", "old keeps its grace after the slow installation")
	require.Eventually(t, func() bool { return !slices.Contains(publishedClusterNames(t, pt), "old") }, 2*time.Second, 5*time.Millisecond,
		"old still leaves once its grace closes")
}
