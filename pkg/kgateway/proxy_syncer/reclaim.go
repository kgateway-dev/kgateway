package proxy_syncer

import (
	"sync"
	"time"
)

// snapshotReclaimGracePeriod is how long a departed client's entry is kept
// before reclaim. It only absorbs the connected-client collection's lag; a
// returning client's entry is cleared at admission anyway
// (registerConnectionStartHooks). Variable so tests can shorten it.
var snapshotReclaimGracePeriod = 5 * time.Second

// snapshotReclaimer clears the cache entries of clients gone for a grace
// period. Row Deletes leave the cache alone, since Envoy must keep its last
// coherent config, so without this every departed client's full snapshot
// would stay for the controller's lifetime.
//
// UniquelyConnectedClients events, fired when an identity's last stream closes
// or it returns, schedule and cancel timers. They lag admission, so departure
// is rechecked against the synchronous state at fire time (reclaimDeparted).
type snapshotReclaimer struct {
	// reclaim clears the entry only if the client is still absent and reports success.
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

// clientDeparted arms the identity's reclaim timer, keeping one already pending.
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

// clientConnected cancels the identity's pending reclaim.
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
	recordSnapshotReclaim()
}
