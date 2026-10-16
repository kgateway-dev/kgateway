package proxy_syncer

import (
	"context"
	"errors"
	"sync"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// errConnectionSuperseded is returned for a publish computed for a client's
// previous connection after its new connection started: it is not installed.
// The new connection gets its own publish once the gate adopts it.
var errConnectionSuperseded = errors.New("the client's connection restarted; publish not installed")

// connectionFence orders snapshot-cache installs against connection starts,
// for clients whose state reports them (see registerConnectionStartHooks).
//
// A connection start clears the client's cache entry while stream admission is
// excluded, so the new stream cannot be answered from it. That alone does not
// stop a publisher that was computing for the previous connection when the
// stream arrived: it holds the gate lock, which the start hook cannot take
// (the reclaimer takes the gate lock before the client state's), and would
// install its stale snapshot after the clear, where the new stream's watches
// would find it. The fence closes that gap:
//
//   - start gives the key a fresh generation and clears its entry, as one step;
//   - the gate adopts the key's current generation when it starts the new
//     connection (publishGate.connectionStarted), after resetting what it knew
//     about the previous one;
//   - install checks the publisher's generation and installs as one step, so a
//     publish computed for an earlier connection is rejected, never installed
//     after the clear.
//
// Its lock is taken after the gate's and after the client state's, and before
// the cache's. Keys whose state reports no connection starts keep generation
// zero, which is also the gate's, so their publishes install as before.
type connectionFence struct {
	mu sync.Mutex
	// next numbers generations across all keys, so a generation is never
	// reused, even after forget.
	next uint64
	gen  map[string]uint64
}

// start begins a new connection generation for proxyKey and clears its cache
// entry. Runs inside stream admission (the connection-start hook).
func (f *connectionFence) start(cache envoycache.SnapshotCache, proxyKey string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gen == nil {
		f.gen = make(map[string]uint64)
	}
	f.next++
	f.gen[proxyKey] = f.next
	cache.ClearSnapshot(proxyKey)
}

// current returns proxyKey's connection generation.
func (f *connectionFence) current(proxyKey string) uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gen[proxyKey]
}

// install sets snap for proxyKey only if gen is still its connection
// generation.
func (f *connectionFence) install(ctx context.Context, cache envoycache.SnapshotCache, proxyKey string, gen uint64, snap envoycache.ResourceSnapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gen[proxyKey] != gen {
		return errConnectionSuperseded
	}
	return cache.SetSnapshot(ctx, proxyKey, snap)
}

// forget drops proxyKey's generation once its entry is reclaimed.
func (f *connectionFence) forget(proxyKey string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.gen, proxyKey)
}

// logPublishFailure logs a publish that did not install. One rejected by the
// fence is expected and logged at debug.
func logPublishFailure(proxyKey string, err error) {
	if errors.Is(err, errConnectionSuperseded) {
		logger.Debug("not installing a publish computed for the client's previous connection", "proxy_key", proxyKey)
		return
	}
	logger.Error("failed to set xds snapshot", "proxy_key", proxyKey, "error", err)
}
