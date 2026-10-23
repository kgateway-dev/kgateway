package proxy_syncer

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowInstallCache delays SetSnapshot while delay is set, standing in for an
// install blocked on a stream. It counts installs and keeps the slow ones.
type slowInstallCache struct {
	envoycache.SnapshotCache
	delay    atomic.Int64
	installs atomic.Int64

	mu   sync.Mutex
	slow []envoycache.ResourceSnapshot
}

func (c *slowInstallCache) SetSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot) error {
	c.installs.Add(1)
	if delay := time.Duration(c.delay.Load()); delay > 0 {
		c.mu.Lock()
		c.slow = append(c.slow, snapshot)
		c.mu.Unlock()
		time.Sleep(delay)
	}
	return c.SnapshotCache.SetSnapshot(ctx, node, snapshot)
}

// firstSlowInstallClusters returns the clusters of the first snapshot installed
// while delay was set.
func (c *slowInstallCache) firstSlowInstallClusters(t *testing.T) map[string]envoycachetypes.ResourceWithTTL {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotEmpty(t, c.slow, "nothing was installed slowly")
	return c.slow[0].GetResourcesAndTTL(resourcev3.ClusterType)
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

// An installation that crosses an open window's deadline leaves that
// deadline's prune queued on the gate lock. The prune must re-publish the build
// just installed, not the older one it was armed with.
func TestDereferencePruneQueuedDuringASlowInstallKeepsTheNewerBuild(t *testing.T) {
	grace := 200 * time.Millisecond
	cache := &slowInstallCache{SnapshotCache: newTestSnapshotCache(t)}
	translator := NewProxyTranslator(cache, stubPriorXDS{has: true}, time.Hour, true, scopedClustersWithGrace(grace))
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	pt.syncXds(ctx, routedBuild(t, "v2", "new", "new")) // old's window opens; its prune is armed with v2
	time.Sleep(grace / 2)
	cache.delay.Store(int64(grace))
	pt.syncXds(ctx, routedBuild(t, "v3", "new", "new")) // still keeps old, and installs past its deadline
	cache.delay.Store(0)
	require.Contains(t, cache.firstSlowInstallClusters(t), "old", "v3 must begin inside old's window, or its prune runs before v3")

	require.Eventually(t, func() bool { return !slices.Contains(publishedClusterNames(t, pt), "old") }, 2*time.Second, 5*time.Millisecond,
		"old's prune runs")
	assert.Equal(t, "v3", publishedListenerVersionOf(t, pt), "the prune re-publishes the newer build, not the one it was armed with")
}

// A window that closes during an installation must not wait for a later
// window: re-arming the prune for b would cancel a's, keeping a published
// until b's window closes.
func TestDereferenceWindowClosingDuringASlowInstallIsNotExtendedByALaterOne(t *testing.T) {
	grace := 300 * time.Millisecond
	cache := &slowInstallCache{SnapshotCache: newTestSnapshotCache(t)}
	translator := NewProxyTranslator(cache, stubPriorXDS{has: true}, time.Hour, true, scopedClustersWithGrace(grace))
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "c", "a", "b", "c"))
	pt.syncXds(ctx, routedBuild(t, "v2", "c", "b", "c")) // a's window opens
	time.Sleep(grace / 2)
	cache.delay.Store(int64(grace))
	pt.syncXds(ctx, routedBuild(t, "v3", "c", "c")) // b's window opens; a's closes during the installation
	cache.delay.Store(0)
	require.Contains(t, cache.firstSlowInstallClusters(t), "a", "v3 must begin inside a's window, or a's prune runs before v3")

	require.Eventually(t, func() bool { return !slices.Contains(publishedClusterNames(t, pt), "a") }, grace/2, 5*time.Millisecond,
		"a leaves once the installation lands, not when b's window closes")
	assert.Contains(t, publishedClusterNames(t, pt), "b", "b keeps its window")
	assert.Equal(t, "v3", publishedListenerVersionOf(t, pt))
	require.Eventually(t, func() bool { return !slices.Contains(publishedClusterNames(t, pt), "b") }, 2*time.Second, 5*time.Millisecond,
		"b still leaves once its window closes")
}

// The same during a reference-ahead hold, which keeps the expired cluster
// until it releases: the prune for a window that closed during the
// installation runs once, not on every publish it makes.
func TestDereferenceWindowClosingDuringASlowHeldInstallIsPrunedOnce(t *testing.T) {
	grace := 300 * time.Millisecond
	scoping := scopedClustersWithReferenceAhead(time.Hour)
	scoping.dereferenceGrace = grace
	cache := &slowInstallCache{SnapshotCache: newTestSnapshotCache(t)}
	translator := NewProxyTranslator(cache, stubPriorXDS{has: true}, time.Hour, true, scoping)
	pt := &translator
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()

	pt.syncXds(ctx, routedBuild(t, "v1", "c", "a", "c"))
	pt.syncXds(ctx, routedBuild(t, "v2", "c", "c")) // a's window opens
	time.Sleep(grace / 2)
	cache.delay.Store(int64(grace))
	pt.syncXds(ctx, routedBuild(t, "v3", "n", "c", "n")) // held for n; a's window closes during the installation
	cache.delay.Store(0)
	require.Contains(t, cache.firstSlowInstallClusters(t), "a", "v3 must begin inside a's window, or a's prune runs before v3")
	installed := cache.installs.Load()

	time.Sleep(grace)
	assert.LessOrEqual(t, cache.installs.Load()-installed, int64(1), "the prune runs once")
	assert.Equal(t, "v2", publishedListenerVersionOf(t, pt), "the retarget is still held")
	assert.Contains(t, publishedClusterNames(t, pt), "a", "the hold keeps a until it releases")
}
