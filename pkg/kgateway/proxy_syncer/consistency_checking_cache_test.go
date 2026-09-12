package proxy_syncer

import (
	"context"
	"testing"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// consistencyCheckingCache decorates a SnapshotCache so that EVERY snapshot
// published by any test — including intermediate publishes no assertion looks
// at, and publishes fired from the gate's budget timers — is checked against
// go-control-plane's Snapshot.Consistent() invariant (every EDS resource
// matched to a CDS reference, every RDS resource to an LDS reference).
//
// Accounting for the known bootstrap local cluster, consistency is a universal
// invariant: it must hold on every publish, including the deliberately
// incomplete ones (bounded first publish, flip release), because their
// incompleteness lives entirely in the route->cluster edge that Consistent()
// does not model. That makes this a zero-false-positive oracle. The
// route->cluster closure is deliberately NOT asserted here — bounded publishes
// legitimately violate it — and remains an opt-in assertion
// (assertSnapshotCoherent) for tests that expect full coherence.
//
// t.Errorf (not Fatalf) is used because SetSnapshot runs on gate timer
// goroutines; Errorf is safe from non-test goroutines.
type consistencyCheckingCache struct {
	envoycache.SnapshotCache
	t *testing.T
}

func newConsistencyCheckingCache(t *testing.T, inner envoycache.SnapshotCache) envoycache.SnapshotCache {
	return &consistencyCheckingCache{SnapshotCache: inner, t: t}
}

// newTestSnapshotCache is the standard cache constructor for tests in this
// package: a plain ADS snapshot cache wrapped with the consistency oracle.
func newTestSnapshotCache(t *testing.T) envoycache.SnapshotCache {
	return newConsistencyCheckingCache(t, envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil))
}

func (c *consistencyCheckingCache) SetSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot) error {
	snap, ok := snapshot.(*envoycache.Snapshot)
	if !ok {
		c.t.Errorf("published snapshot for %q is %T, not *envoycache.Snapshot", node, snapshot)
	} else if err := snapshotConsistencyError(node, snap); err != nil {
		c.t.Errorf("published snapshot for %q violates Snapshot.Consistent(): %v", node, err)
	}
	return c.SnapshotCache.SetSnapshot(ctx, node, snapshot)
}
