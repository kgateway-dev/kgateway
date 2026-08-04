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
