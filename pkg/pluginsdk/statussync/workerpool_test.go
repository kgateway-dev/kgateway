package statussync

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func testResource(name string) Resource {
	return Resource{
		GroupVersionKind: schema.GroupVersionKind{Group: "g", Version: "v", Kind: "K"},
		NamespacedName:   types.NamespacedName{Namespace: "ns", Name: name},
	}
}

func newTestQueue() *WorkQueue {
	return &WorkQueue{
		pending:    make(map[Resource]struct{}),
		processing: make(map[Resource]bool),
	}
}

func TestWorkQueueCoalescesPendingItems(t *testing.T) {
	q := newTestQueue()
	res := testResource("a")

	q.Enqueue(res)
	q.Enqueue(res)

	require.Equal(t, 1, q.Length(), "same resource must be queued once")
	got, ok := q.Dequeue()
	require.True(t, ok)
	require.Equal(t, res, got)
}

func TestWorkQueueReenqueuesWhileProcessing(t *testing.T) {
	q := newTestQueue()
	res := testResource("a")

	q.Enqueue(res)
	got, ok := q.Dequeue()
	require.True(t, ok)
	require.Equal(t, res, got)

	// While the item is processing, a new push must not be dequeued concurrently...
	q.Enqueue(res)
	_, ok = q.Dequeue()
	require.False(t, ok, "an in-flight resource must never be processed concurrently")

	// ...but must be requeued once the in-flight work completes.
	q.MarkDone(res)
	require.Equal(t, 1, q.Length())
	got, ok = q.Dequeue()
	require.True(t, ok)
	require.Equal(t, res, got)
}

func TestWorkQueueShutDownStopsEnqueue(t *testing.T) {
	q := newTestQueue()
	q.Enqueue(testResource("pending"))
	q.ShutDown()
	q.Enqueue(testResource("late"))
	require.Zero(t, q.Length())
}

func TestWorkerPoolRejectsPushAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	pool := NewWorkerPool(ctx, func(context.Context, Resource) {
		calls.Add(1)
	}, 1)
	cancel()
	require.Eventually(t, func() bool {
		pool.lock.Lock()
		defer pool.lock.Unlock()
		return pool.closing
	}, time.Second, time.Millisecond)

	pool.Push(testResource("late"))
	require.Zero(t, pool.q.Length(), "late pushes must not accumulate after shutdown")
	require.Zero(t, calls.Load(), "work pushed after cancellation must not run")
}

func TestWorkerPoolProcessesAllItems(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]struct{}{}
	done := make(chan struct{}, 10)

	pool := NewWorkerPool(context.Background(), func(_ context.Context, res Resource) {
		mu.Lock()
		seen[res.Name] = struct{}{}
		mu.Unlock()
		done <- struct{}{}
	}, 4)

	names := []string{"a", "b", "c", "d", "e"}
	for _, n := range names {
		pool.Push(testResource(n))
	}

	for range names {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for worker pool to drain")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for _, n := range names {
		_, found := seen[n]
		require.True(t, found)
	}
}

// The at-most-one-write-per-resource guarantee is what keeps two workers from racing on the
// same object's status and writing each other's stale data. Exercised at the pool level:
// the queue can uphold it and still be defeated by how the pool dequeues and marks done.
func TestWorkerPoolNeverRunsOneResourceConcurrently(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	var completed atomic.Int32
	release := make(chan struct{})

	pool := NewWorkerPool(ctx, func(context.Context, Resource) {
		current := inFlight.Add(1)
		for {
			previous := maxInFlight.Load()
			if current <= previous || maxInFlight.CompareAndSwap(previous, current) {
				break
			}
		}
		// Hold every worker inside the work function so overlap, if the pool allowed any,
		// is guaranteed to be observed rather than merely possible.
		<-release
		inFlight.Add(-1)
		completed.Add(1)
	}, 8)

	res := testResource("contended")
	for range 20 {
		pool.Push(res)
	}
	// One worker is now blocked in work; every other push coalesced behind it.
	require.Eventually(t, func() bool { return inFlight.Load() == 1 }, 5*time.Second, time.Millisecond)

	close(release)
	require.Eventually(t, func() bool {
		return pool.q.Length() == 0 && inFlight.Load() == 0
	}, 5*time.Second, time.Millisecond, "pool should drain")

	require.Equal(t, int32(1), maxInFlight.Load(),
		"the same resource must never be processed by two workers at once")
	require.LessOrEqual(t, completed.Load(), int32(2),
		"pushes arriving during work must coalesce into a single follow-up pass")
}

// A push that lands while a resource is being written must schedule exactly one follow-up:
// dropping it loses the update that triggered the push, and scheduling one per push turns a
// burst of contributions into a burst of redundant writes.
func TestWorkerPoolPushDuringWorkSchedulesExactlyOneFollowUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var runs atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})

	pool := NewWorkerPool(ctx, func(context.Context, Resource) {
		runs.Add(1)
		entered <- struct{}{}
		<-release
	}, 4)

	res := testResource("busy")
	pool.Push(res)
	<-entered // the first pass is now in flight

	// Several pushes arrive while that pass is running.
	for range 5 {
		pool.Push(res)
	}
	close(release)

	<-entered // the single follow-up pass
	require.Eventually(t, func() bool {
		return pool.q.Length() == 0
	}, 5*time.Second, time.Millisecond)
	require.Never(t, func() bool {
		return runs.Load() > 2
	}, 200*time.Millisecond, 10*time.Millisecond,
		"five pushes during one pass must collapse into exactly one follow-up")
	require.Equal(t, int32(2), runs.Load())
}
