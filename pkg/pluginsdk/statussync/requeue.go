package statussync

import (
	"sync"
	"time"
)

const (
	// notReadyRequeueLimit bounds how many times one resource is re-queued while no client
	// can see it. With notReadyRequeueDelay this covers roughly 30s of informer start-up,
	// after which an object still invisible to every client is treated as gone.
	notReadyRequeueLimit = 6
	// notReadyRequeueDelay is the delay before the first requeue; each subsequent requeue
	// doubles it.
	notReadyRequeueDelay = 500 * time.Millisecond
)

// RequeueFunc schedules another reconciliation of res after delay. Implementations must be
// safe to call while status writing is disabled (this replica is not the leader), where
// dropping the request is correct: acquiring leadership replays every resource anyway.
type RequeueFunc func(res Resource, delay time.Duration)

// NotReadyRequeuer re-queues resources that no client could see yet.
//
// Writers read through delayed clients, which return nil from Get until their own informer
// has been swapped in. That readiness is independent of the raw collection that enqueued the
// resource: kclient builds a separate delayed wrapper per client, and a delayed client's
// HasSynced reports true while Get still returns nil. So neither the status syncer's
// cache-sync barrier nor a HasResource probe can tell "my informer is not ready" from "the
// object was deleted".
//
// Nothing upstream re-fires once the writer's informer does load: the raw collection has
// already delivered the object, and the report reducer has no new reduction to emit. Without
// a requeue the resource silently carries no status until something unrelated touches it —
// the late-CRD-installation outage that versioned writer dispatch exists to prevent.
//
// Requeues are capped so a resource genuinely deleted between enqueue and write stops being
// retried, and so a version no cluster ever serves cannot requeue forever.
type NotReadyRequeuer struct {
	requeue RequeueFunc

	mu sync.Mutex
	// attempts holds only resources currently in the not-ready window; entries are dropped
	// as soon as a resource becomes visible or exhausts its budget.
	attempts map[Resource]int
}

func NewNotReadyRequeuer(requeue RequeueFunc) *NotReadyRequeuer {
	return &NotReadyRequeuer{requeue: requeue, attempts: map[Resource]int{}}
}

// Schedule re-queues res unless it has already exhausted its requeue budget. It is safe to
// call on a nil receiver, which disables requeuing.
func (r *NotReadyRequeuer) Schedule(res Resource) {
	if r == nil || r.requeue == nil {
		return
	}

	r.mu.Lock()
	attempt := r.attempts[res]
	exhausted := attempt >= notReadyRequeueLimit
	if !exhausted {
		r.attempts[res] = attempt + 1
	}
	r.mu.Unlock()

	if exhausted {
		// Treat the object as gone rather than retrying forever. The entry is kept so
		// further enqueues stay cheap no-ops; Done clears it if the resource ever becomes
		// visible again, which is also what bounds the map for anything still in use.
		logger.Warn("giving up waiting for a client that can see this resource; its status was not written",
			"gvk", res.GroupVersionKind.String(),
			"resource", res.NamespacedName.String(),
			"attempts", attempt,
		)
		return
	}

	delay := notReadyRequeueDelay << attempt
	logger.Debug("no client can see this resource yet; requeueing status write",
		"gvk", res.GroupVersionKind.String(),
		"resource", res.NamespacedName.String(),
		"attempt", attempt+1,
		"delay", delay,
	)
	r.requeue(res, delay)
}

// Done clears any requeue budget consumed by res, so a resource that became visible and
// later disappears again is not charged for its earlier wait. Safe on a nil receiver.
func (r *NotReadyRequeuer) Done(res Resource) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Avoid growing the map for the overwhelmingly common already-visible case.
	if len(r.attempts) == 0 {
		return
	}
	delete(r.attempts, res)
}
