package krtcollections

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
)

type debounceItem struct {
	Name  string
	Value int
}

func (d debounceItem) ResourceName() string {
	return d.Name
}

func (d debounceItem) Equals(other debounceItem) bool {
	return d.Name == other.Name && d.Value == other.Value
}

func sortedItems(c krt.Collection[debounceItem]) []debounceItem {
	return slices.SortedFunc(slices.Values(c.List()), func(a, b debounceItem) int {
		return strings.Compare(a.Name, b.Name)
	})
}

func TestDebouncedCollectionDisabledReturnsSource(t *testing.T) {
	src := krt.NewStaticCollection[debounceItem](nil, nil)

	out := newDebouncedCollection[debounceItem](src, 0, time.Second, test.NewStop(t))

	assert.Equal(t, krt.Collection[debounceItem](src), out, "a zero window should not wrap the source")
}

func TestDebouncedCollectionSeedsBeforeSync(t *testing.T) {
	stop := test.NewStop(t)
	src := krt.NewStaticCollection(nil, []debounceItem{{Name: "a", Value: 1}, {Name: "b", Value: 2}})

	out := newDebouncedCollection[debounceItem](src, time.Hour, time.Hour, stop)

	require.True(t, out.WaitUntilSynced(stop), "the mirror should sync")
	assert.Equal(t, sortedItems(src), sortedItems(out), "the mirror should hold the source state as soon as it syncs")
}

func TestDebouncedCollectionBatchesBurst(t *testing.T) {
	stop := test.NewStop(t)
	src := krt.NewStaticCollection[debounceItem](nil, nil)
	out := newDebouncedCollection[debounceItem](src, 200*time.Millisecond, 10*time.Second, stop)
	require.True(t, out.WaitUntilSynced(stop), "the mirror should sync")

	var batches, events atomic.Int32
	out.RegisterBatch(func(evs []krt.Event[debounceItem]) {
		batches.Add(1)
		events.Add(int32(len(evs))) //nolint:gosec // G115: test sizes are tiny
	}, false)

	const n = 100
	for i := range n {
		src.UpdateObject(debounceItem{Name: fmt.Sprint("item-", i), Value: i})
	}

	assert.Eventually(t, func() bool { return events.Load() == n }, 5*time.Second, 10*time.Millisecond,
		"every change should reach the mirror")
	assert.Equal(t, int32(1), batches.Load(), "a burst should be delivered as one event set")
	assert.Equal(t, sortedItems(src), sortedItems(out), "the mirror should match the source after the flush")
}

func TestDebouncedCollectionFlushesWhileChangesKeepArriving(t *testing.T) {
	stop := test.NewStop(t)
	src := krt.NewStaticCollection[debounceItem](nil, nil)
	// The window never elapses while changes arrive every 5ms, so only the ceiling can flush.
	out := newDebouncedCollection[debounceItem](src, 100*time.Millisecond, 200*time.Millisecond, stop)
	require.True(t, out.WaitUntilSynced(stop), "the mirror should sync")

	done := make(chan struct{})
	t.Cleanup(func() { <-done })
	go func() {
		defer close(done)
		for i := range 400 {
			src.UpdateObject(debounceItem{Name: "churning", Value: i})
			time.Sleep(5 * time.Millisecond)
		}
	}()

	assert.Eventually(t, func() bool { return out.GetKey("churning") != nil }, time.Second, 5*time.Millisecond,
		"changes should be flushed by the ceiling while new changes keep arriving")
}

func TestDebouncedCollectionConvergesToSource(t *testing.T) {
	stop := test.NewStop(t)
	src := krt.NewStaticCollection[debounceItem](nil, nil)
	for i := range 50 {
		src.UpdateObject(debounceItem{Name: fmt.Sprint("item-", i), Value: i})
	}
	out := newDebouncedCollection[debounceItem](src, 5*time.Millisecond, 20*time.Millisecond, stop)
	require.True(t, out.WaitUntilSynced(stop), "the mirror should sync")

	// Concurrent updates and deletes, so some land while a flush is reading the source.
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Go(func() {
			for i := range 50 {
				name := fmt.Sprint("item-", (i*7+w)%50)
				if i%5 == w {
					src.DeleteObject(name)
				} else {
					src.UpdateObject(debounceItem{Name: name, Value: 1000*w + i})
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
	wg.Wait()

	assert.Eventually(t, func() bool { return slices.Equal(sortedItems(src), sortedItems(out)) }, 5*time.Second,
		10*time.Millisecond, "the mirror should converge to the source, including deletes")
}
