package proxy_syncer

import (
	"context"
	"testing"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// consistencyCheckingCache runs snapshotConsistencyError on every snapshot a
// test publishes, including intermediate and timer-fired ones: no CLA or
// RouteConfiguration that nothing references (the bootstrap local CLA
// excepted), and no RouteConfiguration a listener names but the snapshot
// lacks. It has no false positives: deliberately incomplete publishes
// (bounded first publish, flip release, left-out CLAs) leave gaps only in
// route->cluster and cluster->CLA edges, which it does not check. Tests that
// expect those closures use assertSnapshotCoherent.
//
// It uses t.Errorf because SetSnapshot runs on timer goroutines, where
// Fatalf must not be called.
type consistencyCheckingCache struct {
	envoycache.SnapshotCache
	t *testing.T
}

func newConsistencyCheckingCache(t *testing.T, inner envoycache.SnapshotCache) envoycache.SnapshotCache {
	return &consistencyCheckingCache{SnapshotCache: inner, t: t}
}

// newTestSnapshotCache is the package's standard test cache: an ADS snapshot
// cache wrapped with the consistency checks.
func newTestSnapshotCache(t *testing.T) envoycache.SnapshotCache {
	return newConsistencyCheckingCache(t, envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil))
}

func (c *consistencyCheckingCache) SetSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot) error {
	snap, ok := snapshot.(*envoycache.Snapshot)
	if !ok {
		c.t.Errorf("published snapshot for %q is %T, not *envoycache.Snapshot", node, snapshot)
	} else if err := snapshotConsistencyError(node, snap); err != nil {
		c.t.Errorf("published snapshot for %q is inconsistent: %v", node, err)
	}
	return c.SnapshotCache.SetSnapshot(ctx, node, snapshot)
}
