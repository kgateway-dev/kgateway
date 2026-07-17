// Derived from https://github.com/istio/istio/blob/master/pilot/pkg/status/resourcelock.go (Apache 2.0),
// by way of https://github.com/agentgateway/agentgateway controller/pkg/syncer/status/workerpool.go.

package statussync

import (
	"context"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// Resource identifies a single object whose status should be written. It is used as the
// queue's coalescing key, so it must contain only the object's identity: including
// anything update-specific (e.g. resourceVersion) would defeat coalescing and break the
// at-most-one-in-flight-write-per-resource guarantee. Writers read the current object
// (including its resourceVersion) from the informer cache at write time.
type Resource struct {
	schema.GroupVersionKind
	types.NamespacedName
}

// WorkerQueue implements an expandable goroutine pool which executes at most one concurrent routine per target
// resource. Multiple calls to Push() will not schedule multiple executions per target resource, but will ensure that
// the single execution uses the latest value.
type WorkerQueue interface {
	// Push a task.
	Push(target Resource, data any)
	// Run the loop until a signal on the context
	Run(ctx context.Context)
}

type WorkQueue struct {
	// a lock to govern access to data in the cache
	mu sync.Mutex
	// queue maintains all pending items awaiting processing
	queue []Resource
	// pending stores information about each item in the queue
	pending map[Resource]any

	// processing stores all resources that have been Dequeue(), but not MarkDone().
	// The value stored will initially be nil, but may be populated if the resource is Enqueue()d again.
	// If the value is not nil, it will be Enqueued again once MarkDone has been called.
	// This lets us build up pending data while ensuring we don't process the same key concurrently.
	processing map[Resource]any

	shuttingDown bool
}

// Enqueue adds an item to the queue. If the item is already pending or being processed,
// its data is replaced with the latest value instead of being queued twice.
func (p *WorkQueue) Enqueue(con Resource, data any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.shuttingDown {
		return
	}

	// If it's already in progress, replace the info and return
	if _, f := p.processing[con]; f {
		p.processing[con] = data
		return
	}

	// We already have this item waiting, replace it with the latest data
	if _, f := p.pending[con]; f {
		p.pending[con] = data
		return
	}

	p.pending[con] = data
	p.queue = append(p.queue, con)
}

// Dequeue removes an item from the queue, returning ok=false when nothing is ready.
func (p *WorkQueue) Dequeue() (r Resource, d any, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.queue) == 0 {
		return Resource{}, nil, false
	}

	con := p.queue[0]
	// The underlying array will still exist, despite the slice changing, so the object may not GC without this
	p.queue = p.queue[1:]

	data := p.pending[con]
	delete(p.pending, con)

	// Mark the resource as in progress
	p.processing[con] = nil

	return con, data, true
}

func (p *WorkQueue) MarkDone(con Resource) {
	p.mu.Lock()
	defer p.mu.Unlock()
	request := p.processing[con]
	delete(p.processing, con)

	// If the info is present, that means Enqueue was called while the resource was not yet marked done.
	// This means we need to add it back to the queue.
	if request != nil {
		p.pending[con] = request
		p.queue = append(p.queue, con)
	}
}

func (p *WorkQueue) ShutDown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shuttingDown = true
}

// Length returns the number of pending items
func (p *WorkQueue) Length() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queue)
}

func NewWorkerPool(ctx context.Context, work func(ctx context.Context, resource Resource, data any), maxWorkers uint) *WorkerPool {
	wp := &WorkerPool{
		work:       work,
		maxWorkers: maxWorkers,
		ctx:        ctx,
		q: WorkQueue{
			pending:    make(map[Resource]any),
			processing: make(map[Resource]any),
		},
	}
	context.AfterFunc(ctx, func() {
		wp.lock.Lock()
		wp.closing = true
		wp.lock.Unlock()
	})
	return wp
}

// WorkerPool executes queued status writes with at most maxWorkers concurrent
// goroutines and at most one in-flight write per resource.
type WorkerPool struct {
	q WorkQueue
	// indicates the queue is closing
	closing bool
	// the function which will be run for each task in queue
	work func(ctx context.Context, resource Resource, data any)
	// current worker routine count
	workerCount uint
	// maximum worker routine count
	maxWorkers uint
	lock       sync.Mutex
	ctx        context.Context
}

func (wp *WorkerPool) Push(target Resource, data any) {
	wp.q.Enqueue(target, data)
	wp.maybeAddWorker()
}

func (wp *WorkerPool) Run(ctx context.Context) {
	context.AfterFunc(ctx, func() {
		wp.lock.Lock()
		wp.closing = true
		wp.lock.Unlock()
	})
}

// maybeAddWorker adds a worker unless we are at maxWorkers. Workers exit when there are no more tasks.
func (wp *WorkerPool) maybeAddWorker() {
	wp.lock.Lock()
	if wp.workerCount >= wp.maxWorkers || wp.q.Length() == 0 {
		wp.lock.Unlock()
		return
	}
	wp.workerCount++
	wp.lock.Unlock()
	go func() {
		for {
			wp.lock.Lock()
			if wp.closing || wp.q.Length() == 0 {
				wp.workerCount--
				wp.lock.Unlock()
				return
			}
			wp.lock.Unlock()

			res, data, ok := wp.q.Dequeue()
			if !ok {
				continue
			}

			// work should be done without holding the lock
			wp.work(wp.ctx, res, data)
			wp.q.MarkDone(res)
		}
	}()
}
