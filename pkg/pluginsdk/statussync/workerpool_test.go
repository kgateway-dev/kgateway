package statussync

import (
	"context"
	"sync"
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
		pending:    make(map[Resource]any),
		processing: make(map[Resource]any),
	}
}

func TestWorkQueueCoalescesPendingItems(t *testing.T) {
	q := newTestQueue()
	res := testResource("a")

	q.Enqueue(res, "v1")
	q.Enqueue(res, "v2")

	require.Equal(t, 1, q.Length(), "same resource must be queued once")
	_, data, ok := q.Dequeue()
	require.True(t, ok)
	require.Equal(t, "v2", data, "the latest data must win")
}

func TestWorkQueueReenqueuesWhileProcessing(t *testing.T) {
	q := newTestQueue()
	res := testResource("a")

	q.Enqueue(res, "v1")
	got, _, ok := q.Dequeue()
	require.True(t, ok)
	require.Equal(t, res, got)

	// While the item is processing, a new push must not be dequeued concurrently...
	q.Enqueue(res, "v2")
	_, _, ok = q.Dequeue()
	require.False(t, ok, "an in-flight resource must never be processed concurrently")

	// ...but must be requeued once the in-flight work completes.
	q.MarkDone(res)
	require.Equal(t, 1, q.Length())
	_, data, ok := q.Dequeue()
	require.True(t, ok)
	require.Equal(t, "v2", data)
}

func TestWorkQueueShutDownStopsEnqueue(t *testing.T) {
	q := newTestQueue()
	q.ShutDown()
	q.Enqueue(testResource("a"), "v1")
	require.Zero(t, q.Length())
}

func TestWorkerPoolProcessesAllItems(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]any{}
	done := make(chan struct{}, 10)

	pool := NewWorkerPool(context.Background(), func(_ context.Context, res Resource, data any) {
		mu.Lock()
		seen[res.Name] = data
		mu.Unlock()
		done <- struct{}{}
	}, 4)

	names := []string{"a", "b", "c", "d", "e"}
	for _, n := range names {
		pool.Push(testResource(n), n+"-data")
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
		require.Equal(t, n+"-data", seen[n])
	}
}
