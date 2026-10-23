package proxy_syncer

import (
	"context"
	"errors"
	"sync"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

// errConnectionSuperseded reports a publish for the client's previous
// connection, which is not installed. The new connection gets its own publish.
var errConnectionSuperseded = errors.New("the client's connection restarted; publish not installed")

// connectionFence orders cache installs against connection starts (see
// registerConnectionStartHooks). A start clears the client's entry under the
// client state's lock, so it cannot take the gate lock. A publisher for the
// previous connection may already hold that lock; without the fence it would
// refill the entry for the new stream's watches. So:
//
//   - start advances the key's generation and clears its entry atomically;
//   - connectionStarted resets the gate's state and adopts that generation;
//   - install checks the generation and writes the cache atomically.
//
// Lock order: gate, client state, fence, cache. Keys without start hooks stay
// at generation zero, so the fence never rejects them.
type connectionFence struct {
	mu sync.Mutex
	// next numbers generations across keys; none is reused, even after forget.
	next uint64
	gen  map[string]uint64
}

// start advances proxyKey's generation and clears its cache entry, under
// stream admission.
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

// install sets snap only if gen still matches proxyKey's generation.
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

// logPublishFailure logs failed installation; expected fence rejections use debug.
func logPublishFailure(proxyKey string, err error) {
	if errors.Is(err, errConnectionSuperseded) {
		logger.Debug("not installing a publish computed for the client's previous connection", "proxy_key", proxyKey)
		return
	}
	logger.Error("failed to set xds snapshot", "proxy_key", proxyKey, "error", err)
}
