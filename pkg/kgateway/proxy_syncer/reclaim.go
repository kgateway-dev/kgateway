package proxy_syncer

import (
	"sync"
	"time"
)

// snapshotReclaimGracePeriod is how long a client identity must remain
// departed before its xDS snapshot cache entry is reclaimed. The grace period
// absorbs reconnect flaps: an Envoy whose stream resets (network blip) usually
// re-identifies with the same resource name within a second or two, and is
// then answered from the cached entry instead of waiting for a fresh publish.
//
// It is short because that entry is also served to a client returning later,
// and the gate does not trust it (connectionSnapshotLocked): a client that
// spent the interval on another controller replica holds newer config, so the
// entry rolls it back until this replica's first publish to it, which a
// deferred build may withhold for up to the first-publish budget, or the warm
// withhold bound. The grace caps how old an entry a returning client can be
// served; once it is reclaimed, the client waits for that first publish while
// keeping the config it holds.
//
// Variable rather than const so tests can shorten it.
var snapshotReclaimGracePeriod = 5 * time.Second

// snapshotReclaimer clears SnapshotCache entries of clients that have left
// the connected set and stayed gone for a grace period.
//
// The per-client snapshot subscriber intentionally does not clear the cache on
// Delete events (Envoy must keep its last coherent config while updates are
// withheld), so without this, entries for truly-departed clients — every
// replaced gateway pod, every scale-down, every crashloop restart — accumulate
// for the controller's lifetime. Each entry holds a full per-client snapshot,
// so client-churn-heavy environments see unbounded controller memory growth.
//
// Departure and return are signalled by the UniquelyConnectedClients
// collection, where a Delete fires only when the last stream for an identity
// closes. That collection lags stream admission, so it only schedules and
// cancels timers; whether the client is still gone is decided at fire time by
// reclaim, against the synchronous connection state (see
// publishGate.reclaimDeparted).
type snapshotReclaimer struct {
	// reclaim clears the client's entry if it is still departed and reports
	// whether it did.
	reclaim func(resourceName string) bool

	mu      sync.Mutex
	pending map[string]*time.Timer
}

func newSnapshotReclaimer(reclaim func(resourceName string) bool) *snapshotReclaimer {
	return &snapshotReclaimer{
		reclaim: reclaim,
		pending: make(map[string]*time.Timer),
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
		r.fire(resourceName)
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

func (r *snapshotReclaimer) fire(resourceName string) {
	r.mu.Lock()
	delete(r.pending, resourceName)
	r.mu.Unlock()

	if !r.reclaim(resourceName) {
		return
	}
	logger.Info("reclaimed xds snapshot cache entry for departed client", "client", resourceName)
	recordSnapshotReclaim(resourceName)
}
