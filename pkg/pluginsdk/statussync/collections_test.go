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
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type recordingQueue struct {
	mu     sync.Mutex
	pushed []Resource
}

func (q *recordingQueue) Push(resource Resource, _ any) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pushed = append(q.pushed, resource)
}

func (q *recordingQueue) resources() []Resource {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]Resource(nil), q.pushed...)
}

func (q *recordingQueue) awaitResources(t *testing.T, n int) []Resource {
	t.Helper()
	require.Eventually(t, func() bool {
		return len(q.resources()) >= n
	}, 5*time.Second, 10*time.Millisecond)
	require.Never(t, func() bool {
		return len(q.resources()) > n
	}, 200*time.Millisecond, 20*time.Millisecond)
	return q.resources()
}

func TestRegisterResourceReplaysAndTracksRawObjects(t *testing.T) {
	stop := test.NewStop(t)
	gvk := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "HTTPRoute"}
	route := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"}}
	col := krt.NewStaticCollection(nil, []*gwv1.HTTPRoute{route}, krt.WithStop(stop))

	sources := NewStatusCollections()
	queue := &recordingQueue{}
	RegisterResource(sources, gvk, col)
	sources.SetQueue(queue)

	want := Resource{GroupVersionKind: gvk, NamespacedName: types.NamespacedName{Namespace: "default", Name: "route"}}
	require.Equal(t, want, queue.awaitResources(t, 1)[0], "leadership acquisition must sweep current objects")

	updated := route.DeepCopy()
	updated.ResourceVersion = "2"
	updated.Status.Parents = []gwv1.RouteParentStatus{{ControllerName: "example.test/controller"}}
	col.UpdateObject(updated)
	require.Equal(t, want, queue.awaitResources(t, 2)[1], "status-only updates must trigger self-healing reconciliation")

	col.DeleteObject("default/route")
	require.Never(t, func() bool {
		return len(queue.resources()) > 2
	}, 200*time.Millisecond, 20*time.Millisecond, "deleted objects cannot have status written")
}

func TestRegisterResourceUsesObjectGVKForNormalizedCollections(t *testing.T) {
	stop := test.NewStop(t)
	fallback := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1alpha3", Kind: "ListenerSet"}
	actual := schema.GroupVersionKind{Group: "gateway.networking.x-k8s.io", Version: "v1alpha1", Kind: "XListenerSet"}
	ls := &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: "listeners", Namespace: "default"}}
	ls.SetGroupVersionKind(actual)
	col := krt.NewStaticCollection(nil, []*gwv1.ListenerSet{ls}, krt.WithStop(stop))

	sources := NewStatusCollections()
	queue := &recordingQueue{}
	RegisterResource(sources, fallback, col)
	sources.SetQueue(queue)

	require.Equal(t, actual, queue.awaitResources(t, 1)[0].GroupVersionKind)
}

func TestRegisterReportsSkipsInitialReplayAndQueuesChangedKeys(t *testing.T) {
	stop := test.NewStop(t)
	initial := reports.NewReportMap()
	reportCol := krt.NewStaticCollection(nil, []ReportsWrapper{NewReportsWrapper(initial)}, krt.WithStop(stop))
	gvk := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "Gateway"}
	nn := types.NamespacedName{Namespace: "default", Name: "gateway"}
	want := Resource{GroupVersionKind: gvk, NamespacedName: nn}

	sources := NewStatusCollections()
	queue := &recordingQueue{}
	RegisterReports(sources, reportCol, func(old, current reports.ReportMap) []Resource {
		if len(old.Gateways) == len(current.Gateways) {
			return nil
		}
		return []Resource{want}
	})
	sources.SetQueue(queue)
	require.Never(t, func() bool {
		return len(queue.resources()) > 0
	}, 200*time.Millisecond, 20*time.Millisecond, "raw object replay owns the initial sweep")

	updated := reports.NewReportMap()
	updated.Gateways[nn] = nil
	reportCol.UpdateObject(NewReportsWrapper(updated))
	require.Equal(t, want, queue.awaitResources(t, 1)[0])
}
