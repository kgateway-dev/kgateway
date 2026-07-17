package statussync

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

type countingQueue struct {
	inner  WorkerQueue
	pushes *atomic.Int32
}

func (q countingQueue) Push(target Resource, data any) {
	q.pushes.Add(1)
	q.inner.Push(target, data)
}

// TestStatusCollectionEnqueueWriteNoopCycle exercises the full declarative write path:
// a status collection event enqueues a write (live != desired), the writer persists the
// desired status, and the resulting informer update is suppressed (live == desired) both
// at the collection level (no new push) and at the writer level (no new API write).
func TestStatusCollectionEnqueueWriteNoopCycle(t *testing.T) {
	stop := test.NewStop(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const controllerName = "kgateway.test/controller"
	gvk := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "HTTPRoute"}

	c := kube.NewFakeClient()
	routesClient := kclient.NewFiltered[*gwv1.HTTPRoute](c, kclient.Filter{})
	routes := krt.WrapClient(routesClient, krt.WithStop(stop))

	// Fixed desired status so recomputations are byte-identical, mirroring how production
	// collections normalize desired statuses through the writer's merge.
	transitionTime := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	desired := gwv1.RouteStatus{
		Parents: []gwv1.RouteParentStatus{{
			ParentRef:      gwv1.ParentReference{Name: "gw"},
			ControllerName: controllerName,
			Conditions: []metav1.Condition{{
				Type:               string(gwv1.RouteConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(gwv1.RouteReasonAccepted),
				LastTransitionTime: transitionTime,
			}},
		}},
	}
	statusCol := krt.NewCollection(routes, func(kctx krt.HandlerContext, r *gwv1.HTTPRoute) *krt.ObjectWithStatus[*gwv1.HTTPRoute, gwv1.RouteStatus] {
		return &krt.ObjectWithStatus[*gwv1.HTTPRoute, gwv1.RouteStatus]{Obj: r, Status: desired}
	}, krt.WithStop(stop))

	var syncs atomic.Int32
	writer := Writer[*gwv1.HTTPRoute, gwv1.RouteStatus]{
		Name:   "httpRoute",
		Client: routesClient,
		Build: func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.HTTPRoute {
			return &gwv1.HTTPRoute{ObjectMeta: om, Status: gwv1.HTTPRouteStatus{RouteStatus: st}}
		},
		GetStatus: func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		Merge: func(current *gwv1.HTTPRoute, d gwv1.RouteStatus) gwv1.RouteStatus {
			return gwv1.RouteStatus{Parents: MergeRouteParentStatuses(controllerName, current.Status.Parents, d.Parents)}
		},
		OnSync: func(res Resource, current *gwv1.HTTPRoute, status gwv1.RouteStatus, took time.Duration, err error) {
			require.NoError(t, err, "status sync must not error")
			syncs.Add(1)
		},
	}

	pool := NewWorkerPool(ctx, func(ctx context.Context, res Resource, data any) {
		writer.ApplyStatus(ctx, res, data)
	}, 2)
	var pushes atomic.Int32
	sc := NewStatusCollections()
	RegisterStatus(sc, gvk, statusCol, func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus })
	sc.SetQueue(countingQueue{inner: pool, pushes: &pushes})

	c.RunAndWait(stop)

	fakeGwAPI := c.GatewayAPI().(*gatewayfake.Clientset)
	countStatusWrites := func() int {
		n := 0
		for _, a := range fakeGwAPI.Actions() {
			if a.GetVerb() == "update" && a.GetSubresource() == "status" && a.GetResource().Resource == "httproutes" {
				n++
			}
		}
		return n
	}

	_, err := c.GatewayAPI().GatewayV1().HTTPRoutes("default").Create(ctx, &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Phase 1: the create event flows collection -> queue -> writer and persists the status.
	require.Eventually(t, func() bool {
		got, err := c.GatewayAPI().GatewayV1().HTTPRoutes("default").Get(ctx, "route", metav1.GetOptions{})
		return err == nil && krt.Equal(got.Status.RouteStatus, desired)
	}, 5*time.Second, 10*time.Millisecond, "desired status should be written to the API server")

	// Phase 2: the informer update from our own write must be suppressed at the
	// collection level (live == desired), so no second push arrives.
	require.Never(t, func() bool {
		return pushes.Load() > 1
	}, 500*time.Millisecond, 50*time.Millisecond, "self-inflicted informer update must not re-enqueue a write")
	require.Equal(t, int32(1), pushes.Load(), "exactly one enqueue for the initial write")
	require.Equal(t, 1, countStatusWrites(), "exactly one API status write")

	// Phase 3: a duplicate push (e.g. leader re-election replay) reaches the writer, which
	// must detect live == merged desired and skip the API write.
	prevSyncs := syncs.Load()
	pool.Push(Resource{GroupVersionKind: gvk, NamespacedName: types.NamespacedName{Namespace: "default", Name: "route"}}, desired)
	require.Eventually(t, func() bool {
		return syncs.Load() > prevSyncs
	}, 5*time.Second, 10*time.Millisecond, "writer should process the duplicate push")
	require.Equal(t, 1, countStatusWrites(), "no-op write must be suppressed by the writer")
}
