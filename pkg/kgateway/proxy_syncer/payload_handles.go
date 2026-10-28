package proxy_syncer

import (
	"sync"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// payloadHandle stands in for one large row (a client's EDS or CDS payload, a
// gateway's translation output) on its way to the per-client snapshot
// transform. KRT queues each dependency event for a collection with the rows it
// carries, Old and New, until the collection's transform gets to it. The
// snapshot transform walks every cluster and CLA, so during a burst it falls
// behind and each queued event would pin two full payload versions. A handle
// pins its key and version; the payload waits in a payloadCells cell, which
// keeps only the latest.
type payloadHandle struct {
	key     string
	version uint64
}

func (h payloadHandle) ResourceName() string { return h.key }

func (h payloadHandle) Equals(in payloadHandle) bool {
	return h.key == in.key && h.version == in.version
}

// payloadRow is a row type KRT deduplicates by its Equals.
type payloadRow[T any] interface {
	Equals(T) bool
}

// payloadCells holds the latest payload behind each key's handle.
//
// A reader may find a payload newer than the handle event that woke it, when
// the producer has moved on while the event sat in the queue. That is correct:
// KRT reruns a transform against the latest state anyway, never the state an
// event carried, and the newer payload's own handle event reruns the reader
// again, which then finds nothing new. A reader never finds one older than its
// handle, since the cell is written before the handle is returned.
//
// Versions come from one counter, so a version names one key's payload. A
// payload that equals the cell's keeps the cell's version and older value, as
// KRT keeps a stored row, so handles change exactly when payloads do.
type payloadCells[T payloadRow[T]] struct {
	mu    sync.Mutex
	last  uint64
	cells map[string]payloadCell[T]
}

type payloadCell[T any] struct {
	version uint64
	payload *T
}

func newPayloadCells[T payloadRow[T]]() *payloadCells[T] {
	return &payloadCells[T]{cells: make(map[string]payloadCell[T])}
}

// store makes payload key's latest and returns its handle.
func (c *payloadCells[T]) store(key string, payload T) payloadHandle {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cell, ok := c.cells[key]; ok && (*cell.payload).Equals(payload) {
		return payloadHandle{key: key, version: cell.version}
	}
	c.last++
	c.cells[key] = payloadCell[T]{version: c.last, payload: &payload}
	return payloadHandle{key: key, version: c.last}
}

// load returns key's latest payload and its version, or nil.
func (c *payloadCells[T]) load(key string) (*T, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cell := c.cells[key]
	return cell.payload, cell.version
}

func (c *payloadCells[T]) forget(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cells, key)
}

// forgetIfAbsent drops key's cell if absent reports that the handles' parent
// no longer has the key. Checking under the lock that store takes means a
// re-added key's payload is never dropped: its transform ran with the key
// present, so either the check sees it or the store comes after the delete.
// A reader that finds a handle without a cell returns nothing, and the
// parent's pending delete or re-add event for the key reruns it.
func (c *payloadCells[T]) forgetIfAbsent(key string, absent func() bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if absent() {
		delete(c.cells, key)
	}
}

// payloadSource is a collection of handles and the cells behind them.
type payloadSource[T payloadRow[T]] struct {
	handles krt.Collection[payloadHandle]
	cells   *payloadCells[T]
}

// fetch fetches key's handle, registering the KRT dependency, and returns the
// key's latest payload and its version, or nil if either is missing.
func (s payloadSource[T]) fetch(kctx krt.HandlerContext, key string) (*T, uint64) {
	if krt.FetchOne(kctx, s.handles, krt.FilterKey(key)) == nil {
		return nil, 0
	}
	return s.cells.load(key)
}

// newPayloadSource builds, from each parent row, the payload build returns,
// stores it in a cell, and emits the cell's handle under the parent row's key.
// A nil payload clears the cell, as does the parent deleting the key.
func newPayloadSource[I any, T payloadRow[T]](
	krtopts krtutil.KrtOptions,
	name string,
	parent krt.Collection[I],
	build func(krt.HandlerContext, I) *T,
) payloadSource[T] {
	cells := newPayloadCells[T]()
	handles := krt.NewCollection(parent, func(kctx krt.HandlerContext, row I) *payloadHandle {
		key := krt.GetKey(row)
		payload := build(kctx, row)
		if payload == nil {
			cells.forget(key)
			return nil
		}
		handle := cells.store(key, *payload)
		return &handle
	}, krtopts.ToOptions(name)...)
	handles.RegisterBatch(func(events []krt.Event[payloadHandle]) {
		for _, e := range events {
			if e.Event != controllers.EventDelete {
				continue
			}
			key := e.Old.key
			cells.forgetIfAbsent(key, func() bool { return parent.GetKey(key) == nil })
		}
	}, false)
	return payloadSource[T]{handles: handles, cells: cells}
}

// payloadSourceFor fronts rows with handles for one consumer, leaving the
// rows' other consumers as they are. Its transform only compares and stores,
// so its own queue, which does carry full rows, drains as fast as they arrive,
// and coalesces a run of row changes into one handle change.
func payloadSourceFor[T payloadRow[T]](
	krtopts krtutil.KrtOptions,
	name string,
	rows krt.Collection[T],
) payloadSource[T] {
	return newPayloadSource(krtopts, name, rows, func(_ krt.HandlerContext, row T) *T {
		return &row
	})
}
