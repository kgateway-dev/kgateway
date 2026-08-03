package statussync

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const testController = "kgateway.test/controller"

type recordingQueue struct {
	mu     sync.Mutex
	pushed []any
}

func (q *recordingQueue) Push(_ Resource, data any) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pushed = append(q.pushed, data)
}

func (q *recordingQueue) writes() []any {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]any(nil), q.pushed...)
}

func (q *recordingQueue) reset() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pushed = nil
}

// awaitWrites waits for exactly n enqueued writes and returns them. krt delivers handler
// events asynchronously, so callers must not read q.writes() directly after a mutation.
func (q *recordingQueue) awaitWrites(t *testing.T, n int, msgAndArgs ...any) []any {
	t.Helper()
	require.Eventually(t, func() bool {
		return len(q.writes()) >= n
	}, 5*time.Second, 10*time.Millisecond, msgAndArgs...)
	// Give any surplus event a chance to arrive so the count assertion is meaningful.
	require.Never(t, func() bool {
		return len(q.writes()) > n
	}, 200*time.Millisecond, 20*time.Millisecond, msgAndArgs...)
	return q.writes()
}

func routeWithParents(parents ...gwv1.RouteParentStatus) *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
		Status:     gwv1.HTTPRouteStatus{RouteStatus: gwv1.RouteStatus{Parents: parents}},
	}
}

func ourParent(gateway string) gwv1.RouteParentStatus {
	return gwv1.RouteParentStatus{
		ParentRef:      gwv1.ParentReference{Name: gwv1.ObjectName(gateway)},
		ControllerName: testController,
	}
}

// registerRouteRemoval wires a static status collection through RegisterStatus with the
// given removal policy and returns the collection plus the queue it feeds.
func registerRouteRemoval(
	t *testing.T,
	removal RemovalPolicy,
	initial krt.ObjectWithStatus[*gwv1.HTTPRoute, gwv1.RouteStatus],
) (krt.StaticCollection[krt.ObjectWithStatus[*gwv1.HTTPRoute, gwv1.RouteStatus]], *recordingQueue) {
	t.Helper()
	stop := test.NewStop(t)
	col := krt.NewStaticCollection(nil, []krt.ObjectWithStatus[*gwv1.HTTPRoute, gwv1.RouteStatus]{initial}, krt.WithStop(stop))

	sc := NewStatusCollections()
	q := &recordingQueue{}
	RegisterStatus(
		sc,
		schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "HTTPRoute"},
		col,
		func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		removal,
	)
	sc.SetQueue(q)
	return col, q
}

// TestRegisterStatusClearOnRemovePublishesEmptyStatus covers a route that outlives its
// desired status (it dropped out of the translation report while the object still exists).
// For multi-writer statuses we must publish an empty desired status so the writer's merge
// drops the parents we own.
//
// This is also the regression test for the suppression-ordering bug: the live status here
// equals the pre-removal desired status, so comparing against that (rather than against the
// empty status we intend to write) would suppress the clear entirely and leak stale parents.
func TestRegisterStatusClearOnRemovePublishesEmptyStatus(t *testing.T) {
	// live status already matches the desired status, so the initial Add is suppressed.
	live := routeWithParents(ourParent("gw"))
	col, q := registerRouteRemoval(t, ClearOnRemove, krt.ObjectWithStatus[*gwv1.HTTPRoute, gwv1.RouteStatus]{
		Obj:    live,
		Status: live.Status.RouteStatus,
	})
	require.Never(t, func() bool {
		return len(q.writes()) > 0
	}, 200*time.Millisecond, 20*time.Millisecond, "live == desired on Add must not enqueue a write")

	col.DeleteObject("default/route")

	writes := q.awaitWrites(t, 1, "removal must enqueue a clearing write")
	require.Equal(t, gwv1.RouteStatus{}, writes[0],
		"removal must publish an empty status so the merge drops only our parents")
}

// TestRegisterStatusClearOnRemoveIsNotGatedOnPriorDesired is the companion to the test
// above: clearing must happen whether or not the write for the prior desired status had
// landed yet, so that a removal has one deterministic outcome rather than depending on
// in-flight write timing.
func TestRegisterStatusClearOnRemoveIsNotGatedOnPriorDesired(t *testing.T) {
	live := routeWithParents(ourParent("gw"))
	col, q := registerRouteRemoval(t, ClearOnRemove, krt.ObjectWithStatus[*gwv1.HTTPRoute, gwv1.RouteStatus]{
		Obj: live,
		// Desired differs from live, i.e. the write for it had not landed yet.
		Status: gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{ourParent("other-gw")}},
	})
	// Discard the initial Add, which legitimately enqueues.
	q.awaitWrites(t, 1, "live != desired on Add must enqueue a write")
	q.reset()

	col.DeleteObject("default/route")

	writes := q.awaitWrites(t, 1, "removal must clear regardless of whether the prior write landed")
	require.Equal(t, gwv1.RouteStatus{}, writes[0])
}

// TestRegisterStatusKeepOnRemoveLeavesStatusUntouched covers single-writer statuses, where
// an empty desired status would wipe every condition rather than just our own entries.
func TestRegisterStatusKeepOnRemoveLeavesStatusUntouched(t *testing.T) {
	live := routeWithParents(ourParent("gw"))
	col, q := registerRouteRemoval(t, KeepOnRemove, krt.ObjectWithStatus[*gwv1.HTTPRoute, gwv1.RouteStatus]{
		Obj:    live,
		Status: gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{ourParent("other-gw")}},
	})
	// The initial Add enqueues (live != desired); the removal must not.
	q.awaitWrites(t, 1, "live != desired on Add must enqueue a write")
	q.reset()

	col.DeleteObject("default/route")

	require.Never(t, func() bool {
		return len(q.writes()) > 0
	}, 500*time.Millisecond, 20*time.Millisecond, "KeepOnRemove must not enqueue anything on removal")
}
