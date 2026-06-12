package validator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubValidator is an inner Validator used to count and program responses.
type stubValidator struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (s *stubValidator) Validate(_ context.Context, _ *envoybootstrapv3.Bootstrap) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.err
}

func (s *stubValidator) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func bootstrapForNode(id string) *envoybootstrapv3.Bootstrap {
	return &envoybootstrapv3.Bootstrap{
		Node: &envoycorev3.Node{Id: id, Cluster: "c"},
	}
}

func TestCachingValidator_HitsAndMisses(t *testing.T) {
	stub := &stubValidator{}
	v := NewCaching(stub, 16)

	bs := bootstrapForNode("a")
	require.NoError(t, v.Validate(context.Background(), bs))
	require.NoError(t, v.Validate(context.Background(), bs))
	require.NoError(t, v.Validate(context.Background(), bs))
	assert.Equal(t, 1, stub.Calls(), "identical input should hit cache")

	require.NoError(t, v.Validate(context.Background(), bootstrapForNode("b")))
	assert.Equal(t, 2, stub.Calls(), "different input should miss cache")
}

func TestCachingValidator_CachesErrInvalidXDS(t *testing.T) {
	stub := &stubValidator{err: fmt.Errorf("%w: bad cluster cfg", ErrInvalidXDS)}
	v := NewCaching(stub, 16)

	bs := bootstrapForNode("a")
	err1 := v.Validate(context.Background(), bs)
	err2 := v.Validate(context.Background(), bs)
	require.Error(t, err1)
	require.Error(t, err2)
	assert.True(t, errors.Is(err1, ErrInvalidXDS), "first error should chain ErrInvalidXDS")
	assert.True(t, errors.Is(err2, ErrInvalidXDS), "cached error should chain ErrInvalidXDS")
	assert.Equal(t, err1.Error(), err2.Error(), "cached message should match original")
	assert.Equal(t, 1, stub.Calls(), "ErrInvalidXDS should be cached")
}

func TestCachingValidator_DoesNotCacheTransientErrors(t *testing.T) {
	stub := &stubValidator{err: errors.New("envoy validate invocation failed: exec format error")}
	v := NewCaching(stub, 16)

	bs := bootstrapForNode("a")
	for range 3 {
		err := v.Validate(context.Background(), bs)
		require.Error(t, err)
	}
	assert.Equal(t, 3, stub.Calls(), "transient errors must not be cached")
}

func TestCachingValidator_KeyStability(t *testing.T) {
	// Two structurally-identical bootstraps must hash to the same key.
	a := bootstrapForNode("same")
	b := bootstrapForNode("same")
	keyA, err := cacheKeyFor(a)
	require.NoError(t, err)
	keyB, err := cacheKeyFor(b)
	require.NoError(t, err)
	assert.Equal(t, keyA, keyB)

	// Different content must produce different keys.
	keyC, err := cacheKeyFor(bootstrapForNode("different"))
	require.NoError(t, err)
	assert.NotEqual(t, keyA, keyC)
}

func TestCachingValidator_Eviction(t *testing.T) {
	stub := &stubValidator{}
	v := NewCaching(stub, 2) // size 2 to force eviction quickly

	a := bootstrapForNode("a")
	b := bootstrapForNode("b")
	c := bootstrapForNode("c")

	require.NoError(t, v.Validate(context.Background(), a))
	require.NoError(t, v.Validate(context.Background(), b))
	require.NoError(t, v.Validate(context.Background(), c)) // evicts a
	assert.Equal(t, 3, stub.Calls())

	require.NoError(t, v.Validate(context.Background(), a)) // a was evicted
	assert.Equal(t, 4, stub.Calls(), "evicted entry should re-call inner validator")

	require.NoError(t, v.Validate(context.Background(), c)) // c still cached
	assert.Equal(t, 4, stub.Calls(), "still-cached entry should not re-call")
}

// gatedValidator blocks every Validate call until release is closed,
// counting entries — used to hold a singleflight leader in-flight while
// concurrent callers pile up on the same key.
type gatedValidator struct {
	calls   atomic.Int32
	release chan struct{}
}

func (b *gatedValidator) Validate(_ context.Context, _ *envoybootstrapv3.Bootstrap) error {
	b.calls.Add(1)
	<-b.release
	return nil
}

func TestCachingValidator_ConcurrentMissesSingleflight(t *testing.T) {
	// Concurrent misses on the same key must collapse to ONE inner invocation:
	// during initial sync, independent collections validate identical content
	// concurrently, and without singleflight each concurrent miss forks its
	// own envoy. The leader is held in-flight until all callers have launched,
	// so every other goroutine either joins the in-flight call or — if it
	// arrives after completion — hits the LRU (populated before the leader
	// returns). Both paths make a second inner call impossible.
	inner := &gatedValidator{release: make(chan struct{})}
	v := NewCaching(inner, 16)
	bs := bootstrapForNode("hot")

	const goroutines = 16
	var wg sync.WaitGroup
	var errs atomic.Int32
	for range goroutines {
		wg.Go(func() {
			if err := v.Validate(context.Background(), bs); err != nil {
				errs.Add(1)
			}
		})
	}
	// Wait for the leader to enter the inner validator, give the remaining
	// goroutines time to reach the singleflight, then release.
	require.Eventually(t, func() bool { return inner.calls.Load() >= 1 },
		5*time.Second, time.Millisecond, "leader never reached inner validator")
	time.Sleep(100 * time.Millisecond)
	close(inner.release)
	wg.Wait()

	assert.Zero(t, errs.Load())
	assert.Equal(t, int32(1), inner.calls.Load(),
		"concurrent misses on one key must collapse to a single inner call")
}
