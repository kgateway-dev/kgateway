package proxy_syncer

import (
	"context"
	"testing"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// consistencyCheckingCache decorates a SnapshotCache so that EVERY snapshot
// published by any test — including intermediate publishes no assertion looks
// at, and publishes fired from the gate's budget timers — is checked for
// resources no other resource of the snapshot references (every EDS resource
// must match a CDS reference, every RDS resource an LDS reference; see
// snapshotConsistencyError).
//
// Accounting for the known bootstrap local cluster, this is a universal
// invariant: it must hold on every publish, including the deliberately
// incomplete ones (bounded first publish, flip release, CLAs left out so a
// client keeps its endpoints), because their incompleteness lives in edges
// the check does not model: route->cluster and EDS cluster->CLA. That makes
// this a zero-false-positive oracle. Those closures are deliberately NOT
// asserted here and remain an opt-in assertion (assertSnapshotCoherent) for
// tests that expect full coherence.
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
		c.t.Errorf("published snapshot for %q publishes unreferenced resources: %v", node, err)
	}
	return c.SnapshotCache.SetSnapshot(ctx, node, snapshot)
}
