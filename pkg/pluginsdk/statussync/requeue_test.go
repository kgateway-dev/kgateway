package statussync

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type recordedRequeue struct {
	res   Resource
	delay time.Duration
}

type requeueRecorder struct {
	mu     sync.Mutex
	calls  []recordedRequeue
	replay func(Resource)
}

func (r *requeueRecorder) fn(res Resource, delay time.Duration) {
	r.mu.Lock()
	r.calls = append(r.calls, recordedRequeue{res: res, delay: delay})
	replay := r.replay
	r.mu.Unlock()
	if replay != nil {
		replay(res)
	}
}

func (r *requeueRecorder) recorded() []recordedRequeue {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequeue(nil), r.calls...)
}

// A chain is what production actually produces: each requeue leads to another write pass,
// which schedules again while the resource stays invisible. The chain must terminate.
func TestNotReadyRequeuerChainBacksOffAndGivesUp(t *testing.T) {
	rec := &requeueRecorder{}
	requeuer := NewNotReadyRequeuer(rec.fn)
	res := testRouteResource()
	rec.replay = func(r Resource) { requeuer.Schedule(r) }

	requeuer.Schedule(res)

	calls := rec.recorded()
	require.Len(t, calls, notReadyRequeueLimit,
		"a resource that never becomes visible must stop being requeued")
	require.Equal(t, notReadyRequeueDelay, calls[0].delay)
	require.Equal(t, notReadyRequeueDelay*2, calls[1].delay, "each requeue must back off")
	require.Greater(t, calls[len(calls)-1].delay, calls[0].delay)

	requeuer.mu.Lock()
	defer requeuer.mu.Unlock()
	require.Empty(t, requeuer.attempts,
		"an exhausted resource must not leave a tombstone: a resource deleted before its write "+
			"is never seen again, so nothing would ever clear it")
}

// Only the chain is capped, not the resource. A later enqueue is new evidence that the
// resource may be visible now, and must get a fresh budget rather than being blacklisted.
func TestNotReadyRequeuerGivesLaterEnqueuesAFreshBudget(t *testing.T) {
	rec := &requeueRecorder{}
	requeuer := NewNotReadyRequeuer(rec.fn)
	res := testRouteResource()
	rec.replay = func(r Resource) { requeuer.Schedule(r) }

	requeuer.Schedule(res)
	require.Len(t, rec.recorded(), notReadyRequeueLimit)

	requeuer.Schedule(res)
	require.Len(t, rec.recorded(), 2*notReadyRequeueLimit)
	require.Equal(t, notReadyRequeueDelay, rec.recorded()[notReadyRequeueLimit].delay,
		"the second chain must start from the base delay")
}

func TestNotReadyRequeuerResetsAfterResourceBecomesVisible(t *testing.T) {
	rec := &requeueRecorder{}
	requeuer := NewNotReadyRequeuer(rec.fn)
	res := testRouteResource()

	requeuer.Schedule(res)
	requeuer.Schedule(res)
	// The informer loaded and the write went through; the budget spent waiting must not be
	// charged against a later disappearance of the same resource.
	requeuer.Done(res)
	requeuer.Schedule(res)

	calls := rec.recorded()
	require.Len(t, calls, 3)
	require.Equal(t, notReadyRequeueDelay, calls[2].delay, "the budget must reset after a visible pass")
}

func TestNotReadyRequeuerNilIsInert(t *testing.T) {
	var requeuer *NotReadyRequeuer
	require.NotPanics(t, func() {
		requeuer.Schedule(testRouteResource())
		requeuer.Done(testRouteResource())
	}, "a writer without a requeuer must still work")
}

// The regression this whole mechanism exists for: a delayed client returns nil from Get
// until its own informer loads, which is independent of the raw collection that enqueued the
// resource. Without a requeue nothing re-fires afterwards and the resource keeps no status.
func TestApplyStatusRequeuesUntilClientCanSeeResource(t *testing.T) {
	writer, result, fake := newTestWriter(t, true)

	// Stand in for a client whose informer has not loaded yet: invisible for the first two
	// passes, then visible.
	realClient := writer.Client
	var passes atomic.Int32
	writer.Client = notReadyClient[*gwv1.HTTPRoute]{
		Client:  realClient,
		visible: func() bool { return passes.Add(1) > 2 },
	}

	var requeues atomic.Int32
	rec := &requeueRecorder{}
	rec.replay = func(res Resource) {
		requeues.Add(1)
		writer.ApplyStatus(context.Background(), res)
	}
	writer.NotReady = NewNotReadyRequeuer(rec.fn)

	writer.ApplyStatus(context.Background(), testRouteResource())

	require.Equal(t, int32(2), requeues.Load(), "each invisible pass must schedule one more")
	require.Equal(t, 1, countUpdates(fake), "the status must be written once the client can see it")
	require.Equal(t, int32(1), result.calls.Load())
	require.NoError(t, result.err())
}

func TestApplyStatusDoesNotRequeueWhenResourceIsVisible(t *testing.T) {
	writer, _, _ := newTestWriter(t, true)
	rec := &requeueRecorder{}
	writer.NotReady = NewNotReadyRequeuer(rec.fn)

	writer.ApplyStatus(context.Background(), testRouteResource())

	require.Empty(t, rec.recorded(), "a visible resource must not be requeued")
}

// notReadyClient makes Get invisible until visible() says otherwise, modelling a delayed
// client that has not swapped its informer in.
type notReadyClient[T controllers.ComparableObject] struct {
	kclient.Client[T]
	visible func() bool
}

func (c notReadyClient[T]) Get(name, namespace string) T {
	if !c.visible() {
		var empty T
		return empty
	}
	return c.Client.Get(name, namespace)
}

func TestStatusCollectionsRequeueDropsWhenNotLeader(t *testing.T) {
	sc := NewStatusCollections()
	// No queue set: this replica is not the leader, so requeues must be dropped rather than
	// panicking or queuing work that nothing will drain.
	require.NotPanics(t, func() { sc.Requeue(testRouteResource(), time.Millisecond) })

	var pushed atomic.Int32
	sc.SetQueue(pushQueue{onPush: func(Resource) { pushed.Add(1) }})
	sc.Requeue(testRouteResource(), time.Millisecond)
	require.Eventually(t, func() bool { return pushed.Load() == 1 }, 5*time.Second, time.Millisecond,
		"a requeue must reach the queue once this replica is the leader")
}

type pushQueue struct {
	onPush func(Resource)
}

func (q pushQueue) Push(target Resource) { q.onPush(target) }
