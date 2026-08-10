package proxy_syncer

import (
	"sync"
	"time"
)

// snapshotReclaimGracePeriod is how long a client identity must remain
// departed before its xDS snapshot cache entry is reclaimed. The grace period
// absorbs reconnect flaps: an Envoy whose stream resets (network blip, xDS
// LB rebalance) re-identifies with the same resource name within seconds, and
// reclaiming eagerly would force it to wait for a full per-client rebuild
// instead of being served the cached snapshot immediately.
//
// Variable rather than const so tests can shorten it.
var snapshotReclaimGracePeriod = 60 * time.Second

// xdsSnapshotClearer is the slice of envoycache.SnapshotCache the reclaimer
// needs; narrowed for testability.
type xdsSnapshotClearer interface {
	ClearSnapshot(node string)
}

// snapshotReclaimer clears SnapshotCache entries of clients that have left
// the connected set and stayed gone for a grace period.
//
// Why this exists: the per-client snapshot subscriber intentionally does NOT
// clear the cache on Delete events (Envoy must keep its last coherent config
// while updates are withheld), so entries for truly-departed clients — every
// replaced gateway pod, every scale-down, every crashloop restart — accumulate
// for the controller's lifetime. Each entry holds a full per-client snapshot,
// so client-churn-heavy environments see unbounded controller memory growth.
//
// The departure signal is the UniquelyConnectedClients collection, where a
// Delete fires only when the last stream for an identity closes — unlike the
// per-client snapshot collection, whose Deletes can also mean "publication
// deferred". At reclaim time the identity is re-checked against the connected
// set, so a client that returned during the grace period is never reclaimed.
type snapshotReclaimer struct {
	cache xdsSnapshotClearer
	// stillDeparted reports whether the client identity is still absent from
	// the connected-client set at reclaim time.
	stillDeparted func(resourceName string) bool

	mu      sync.Mutex
	pending map[string]*time.Timer
}

func newSnapshotReclaimer(cache xdsSnapshotClearer, stillDeparted func(string) bool) *snapshotReclaimer {
	return &snapshotReclaimer{
		cache:         cache,
		stillDeparted: stillDeparted,
		pending:       make(map[string]*time.Timer),
	}
}

// clientDeparted arms the reclaim timer for the identity; no-op if one is
// already pending.
func (r *snapshotReclaimer) clientDeparted(resourceName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pending[resourceName]; ok {
		return
	}
	logger.Debug("client departed; scheduling xds snapshot reclaim",
		"client", resourceName, "grace", snapshotReclaimGracePeriod)
	r.pending[resourceName] = time.AfterFunc(snapshotReclaimGracePeriod, func() {
		r.reclaim(resourceName)
	})
}

// clientConnected cancels any pending reclaim for the identity (a reconnect
// within the grace period).
func (r *snapshotReclaimer) clientConnected(resourceName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.pending[resourceName]; ok {
		t.Stop()
		delete(r.pending, resourceName)
	}
}

func (r *snapshotReclaimer) reclaim(resourceName string) {
	r.mu.Lock()
	delete(r.pending, resourceName)
	r.mu.Unlock()

	// Re-check at fire time: the timer may race a reconnect whose Add event
	// has not reached clientConnected yet.
	if !r.stillDeparted(resourceName) {
		return
	}
	logger.Info("reclaiming xds snapshot cache entry for departed client", "client", resourceName)
	r.cache.ClearSnapshot(resourceName)
	recordSnapshotReclaim(resourceName)
}
