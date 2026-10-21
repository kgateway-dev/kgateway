package proxy_syncer

import (
	"context"
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
