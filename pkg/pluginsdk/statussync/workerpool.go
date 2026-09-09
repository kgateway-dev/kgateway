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
// resource. Multiple calls to Push() will not schedule concurrent executions for the same resource, but an enqueue
// while that resource is in flight schedules one follow-up reconciliation.
type WorkerQueue interface {
	// Push a task.
	Push(target Resource)
}

type WorkQueue struct {
	// a lock to govern access to data in the cache
	mu sync.Mutex
	// queue maintains all pending items awaiting processing
	queue []Resource
	// pending tracks membership in the queue.
	pending map[Resource]struct{}

	// processing stores resources that have been Dequeue()d but not MarkDone(). The bool
	// is set when another enqueue arrives, requesting one follow-up reconciliation.
	processing map[Resource]bool

	shuttingDown bool
}

// Enqueue adds an item to the queue. Pending duplicates are coalesced; an enqueue while
// processing sets the dirty bit that schedules one follow-up pass.
func (p *WorkQueue) Enqueue(con Resource) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.shuttingDown {
		return
	}

	// If it is already in progress, remember that it needs one follow-up pass.
	if _, f := p.processing[con]; f {
		p.processing[con] = true
		return
	}

	// It is already waiting; resource identity is the complete request.
	if _, f := p.pending[con]; f {
		return
	}

	p.pending[con] = struct{}{}
	p.queue = append(p.queue, con)
}

// Dequeue removes an item from the queue, returning ok=false when nothing is ready.
func (p *WorkQueue) Dequeue() (r Resource, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.queue) == 0 {
		return Resource{}, false
	}

	con := p.queue[0]
	// Zero the removed slot: the underlying array outlives the reslice, so a stale
	// Resource would otherwise keep its strings reachable.
	p.queue[0] = Resource{}
	p.queue = p.queue[1:]

	delete(p.pending, con)

	// Mark the resource as in progress
	p.processing[con] = false

	return con, true
}

func (p *WorkQueue) MarkDone(con Resource) {
	p.mu.Lock()
	defer p.mu.Unlock()
	requeue := p.processing[con]
	delete(p.processing, con)

	// An enqueue while this resource was processing requests one follow-up pass.
	if requeue && !p.shuttingDown {
		p.pending[con] = struct{}{}
		p.queue = append(p.queue, con)
	}
}

func (p *WorkQueue) ShutDown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shuttingDown = true
	clear(p.pending)
	clear(p.processing)
	clear(p.queue)
	p.queue = nil
}

// Length returns the number of pending items
func (p *WorkQueue) Length() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queue)
}

func NewWorkerPool(ctx context.Context, work func(ctx context.Context, resource Resource), maxWorkers uint) *WorkerPool {
	wp := &WorkerPool{
		work:       work,
		maxWorkers: maxWorkers,
		ctx:        ctx,
		q: WorkQueue{
			pending:    make(map[Resource]struct{}),
			processing: make(map[Resource]bool),
		},
	}
	context.AfterFunc(ctx, func() {
		wp.lock.Lock()
		wp.closing = true
		wp.q.ShutDown()
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
	work func(ctx context.Context, resource Resource)
	// current worker routine count
	workerCount uint
	// maximum worker routine count
	maxWorkers uint
	lock       sync.Mutex
	ctx        context.Context
}

func (wp *WorkerPool) Push(target Resource) {
	wp.lock.Lock()
	if wp.closing {
		wp.lock.Unlock()
		return
	}
	wp.q.Enqueue(target)
	wp.maybeAddWorkerLocked()
	wp.lock.Unlock()
}

// maybeAddWorker adds a worker unless we are at maxWorkers. Workers exit when there are no more tasks.
func (wp *WorkerPool) maybeAddWorkerLocked() {
	if wp.workerCount >= wp.maxWorkers || wp.q.Length() == 0 {
		return
	}
	wp.workerCount++
	go func() {
		for {
			wp.lock.Lock()
			if wp.closing || wp.q.Length() == 0 {
				wp.workerCount--
				wp.lock.Unlock()
				return
			}
			wp.lock.Unlock()

			res, ok := wp.q.Dequeue()
			if !ok {
				continue
			}

			// work should be done without holding the lock
			wp.work(wp.ctx, res)
			wp.q.MarkDone(res)
		}
	}()
}
