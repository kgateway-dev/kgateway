package statussync

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/test"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

type applyResult struct {
	calls   atomic.Int32
	lastErr atomic.Value
}

func (r *applyResult) err() error {
	v := r.lastErr.Load()
	if v == nil {
		return nil
	}
	e, _ := v.(error)
	return e
}

// newTestWriter wires a Writer against a fake API server holding one HTTPRoute, and returns
// the writer, a handle on its OnSync observations, and a count of status update attempts.
func newTestWriter(t *testing.T, createRoute bool) (Writer[*gwv1.HTTPRoute, gwv1.RouteStatus], *applyResult, *gatewayfake.Clientset) {
	t.Helper()
	stop := test.NewStop(t)
	c := kube.NewFakeClient()
	routesClient := kclient.NewFiltered[*gwv1.HTTPRoute](c, kclient.Filter{})

	if createRoute {
		_, err := c.GatewayAPI().GatewayV1().HTTPRoutes("default").Create(context.Background(), &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", ResourceVersion: "1"},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	c.RunAndWait(stop)

	result := &applyResult{}
	writer := Writer[*gwv1.HTTPRoute, gwv1.RouteStatus]{
		Name:   "httpRoute",
		Client: routesClient,
		Desired: func(*gwv1.HTTPRoute) (gwv1.RouteStatus, bool) {
			return gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{{
				ParentRef:      gwv1.ParentReference{Name: "gw"},
				ControllerName: ourController,
			}}}, true
		},
		Build: func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.HTTPRoute {
			return &gwv1.HTTPRoute{ObjectMeta: om, Status: gwv1.HTTPRouteStatus{RouteStatus: st}}
		},
		GetStatus: func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		OnSync: func(_ Resource, _ *gwv1.HTTPRoute, _ gwv1.RouteStatus, _ time.Duration, err error) {
			result.calls.Add(1)
			if err != nil {
				result.lastErr.Store(err)
			}
		},
	}

	if createRoute {
		require.Eventually(t, func() bool {
			return routesClient.Get("route", "default") != nil
		}, 5*time.Second, 10*time.Millisecond, "informer should observe the route")
	}
	return writer, result, c.GatewayAPI().(*gatewayfake.Clientset)
}

func testRouteResource() Resource {
	return Resource{
		GroupVersionKind: wellknown.HTTPRouteGVK,
		NamespacedName:   types.NamespacedName{Namespace: "default", Name: "route"},
	}
}

func countUpdates(fake *gatewayfake.Clientset) int {
	n := 0
	for _, a := range fake.Actions() {
		if a.GetVerb() == "update" && a.GetSubresource() == "status" {
			n++
		}
	}
	return n
}

// A conflict means another writer got there first. The raw collection re-enqueues once the
// informer delivers the newer object, so retrying here would only burn the retry budget and
// widen the window in which we write stale data.
func TestApplyStatusSwallowsConflict(t *testing.T) {
	writer, result, fake := newTestWriter(t, true)
	fake.PrependReactor("update", "httproutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(
			schema.GroupResource{Group: gwv1.GroupName, Resource: "httproutes"}, "route", nil)
	})

	writer.ApplyStatus(context.Background(), testRouteResource())

	require.Equal(t, int32(1), result.calls.Load(), "OnSync must run once for a resource with a desired status")
	require.NoError(t, result.err(), "a conflict is expected and must not be reported as a sync failure")
	require.Equal(t, 1, countUpdates(fake), "a conflict must not be retried")
}

// A NotFound on write means the object was deleted between our read and our write. There is
// nothing left to update, so this is not a failure.
func TestApplyStatusSwallowsNotFound(t *testing.T) {
	writer, result, fake := newTestWriter(t, true)
	fake.PrependReactor("update", "httproutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(
			schema.GroupResource{Group: gwv1.GroupName, Resource: "httproutes"}, "route")
	})

	writer.ApplyStatus(context.Background(), testRouteResource())

	require.Equal(t, int32(1), result.calls.Load())
	require.NoError(t, result.err(), "a deleted resource must not be reported as a sync failure")
	require.Equal(t, 1, countUpdates(fake), "a NotFound must not be retried")
}

// Transient failures are the one case that must be retried: nothing changes on the informer
// after a failed write, so no event is guaranteed to re-enqueue the resource.
func TestApplyStatusRetriesTransientErrorsAndReportsFailure(t *testing.T) {
	writer, result, fake := newTestWriter(t, true)
	fake.PrependReactor("update", "httproutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewTooManyRequests("slow down", 1)
	})

	writer.ApplyStatus(context.Background(), testRouteResource())

	require.Equal(t, int32(1), result.calls.Load())
	require.Error(t, result.err(), "a persistently failing write must surface as a sync error")
	require.Equal(t, maxRetryAttempts, countUpdates(fake), "transient failures must exhaust the retry budget")
}

func TestApplyStatusSkipsMissingResource(t *testing.T) {
	writer, result, fake := newTestWriter(t, false)

	writer.ApplyStatus(context.Background(), testRouteResource())

	require.Zero(t, result.calls.Load(), "OnSync must not run when the object is gone")
	require.Zero(t, countUpdates(fake))
}
