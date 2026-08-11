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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type countingQueue struct {
	inner  WorkerQueue
	pushes *atomic.Int32
}

func (q countingQueue) Push(target Resource) {
	q.pushes.Add(1)
	q.inner.Push(target)
}

// TestStatusCollectionEnqueueWriteNoopCycle exercises the full just-in-time write path:
// a raw collection event enqueues an identity, the writer builds and persists desired
// status, and the resulting informer update reaches the writer but becomes a no-op.
func TestStatusCollectionEnqueueWriteNoopCycle(t *testing.T) {
	stop := test.NewStop(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const controllerName = "kgateway.test/controller"
	gvk := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "HTTPRoute"}

	c := kube.NewFakeClient()
	routesClient := kclient.NewFiltered[*gwv1.HTTPRoute](c, kclient.Filter{})
	routes := krt.WrapClient(routesClient, krt.WithStop(stop))

	// Build desired status with the real production builder rather than a fixed value. The
	// no-op skip only holds if rebuilding from the same report against the status we just
	// wrote reproduces that status exactly; a builder that renormalized anything (condition
	// order, defaulted parentRef fields, a fresh timestamp) would write forever. Using the
	// real builder is what makes this a regression guard for #12278 rather than a test of
	// the skip mechanism alone.
	routeReport := reports.NewReportMap()
	reports.NewReporter(&routeReport).
		Route(&gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"}}).
		ParentRef(&gwv1.ParentReference{Name: "gw"})
	buildDesired := func(current *gwv1.HTTPRoute) (gwv1.RouteStatus, bool) {
		status := reports.BuildRouteStatus(
			routeReport.HTTPRoutes[types.NamespacedName{Namespace: "default", Name: "route"}],
			current,
			controllerName,
		)
		if status == nil {
			return gwv1.RouteStatus{}, false
		}
		return *status, true
	}

	var syncs atomic.Int32
	writer := Writer[*gwv1.HTTPRoute, gwv1.RouteStatus]{
		Name:    "httpRoute",
		Current: CollectionSource(routes),
		Desired: buildDesired,
		UpdateStatus: ClientWriter(routesClient, func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.HTTPRoute {
			return &gwv1.HTTPRoute{ObjectMeta: om, Status: gwv1.HTTPRouteStatus{RouteStatus: st}}
		}),
		GetStatus: func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		Merge: func(current *gwv1.HTTPRoute, d gwv1.RouteStatus) gwv1.RouteStatus {
			return gwv1.RouteStatus{Parents: MergeRouteParentStatuses(controllerName, current.Status.Parents, d.Parents)}
		},
		OnSync: func(res Resource, current *gwv1.HTTPRoute, status gwv1.RouteStatus, took time.Duration, err error) {
			require.NoError(t, err, "status sync must not error")
			syncs.Add(1)
		},
	}

	pool := NewWorkerPool(ctx, func(ctx context.Context, res Resource) {
		writer.ApplyStatus(ctx, res)
	}, 2)
	var pushes atomic.Int32
	sc := NewStatusCollections()
	RegisterResource(sc, gvk, routes)
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
		if err != nil || len(got.Status.Parents) != 1 {
			return false
		}
		parent := got.Status.Parents[0]
		return parent.ControllerName == controllerName &&
			parent.ParentRef.Name == "gw" &&
			meta.IsStatusConditionTrue(parent.Conditions, string(gwv1.RouteConditionAccepted))
	}, 5*time.Second, 10*time.Millisecond, "desired status should be written to the API server")

	// The written status must be a fixed point of the builder: feeding it back in produces
	// the same value, which is the precondition for the no-op skip asserted below. This is
	// the same check CheckWriterIdempotent exports for writers outside this package.
	written := routesClient.Get("route", "default")
	require.NotNil(t, written)
	require.NoError(t, CheckWriterIdempotent(writer, written,
		func(current *gwv1.HTTPRoute, status gwv1.RouteStatus) *gwv1.HTTPRoute {
			next := current.DeepCopy()
			next.Status.RouteStatus = *status.DeepCopy()
			return next
		}), "rebuilding desired status from what we wrote must reproduce it, or writes never converge")

	// Phase 2: the informer update from our own write re-enqueues the identity, and the
	// writer suppresses the API call after rebuilding the same desired status.
	require.Eventually(t, func() bool {
		return pushes.Load() >= 2
	}, 5*time.Second, 10*time.Millisecond, "status informer update must trigger reconciliation")
	require.Equal(t, 1, countStatusWrites(), "exactly one API status write")

	// Phase 3: a duplicate push (e.g. leader re-election replay) reaches the writer, which
	// must detect live == merged desired and skip the API write.
	prevSyncs := syncs.Load()
	pool.Push(Resource{GroupVersionKind: gvk, NamespacedName: types.NamespacedName{Namespace: "default", Name: "route"}})
	require.Eventually(t, func() bool {
		return syncs.Load() > prevSyncs
	}, 5*time.Second, 10*time.Millisecond, "writer should process the duplicate push")
	require.Equal(t, 1, countStatusWrites(), "no-op write must be suppressed by the writer")
}
