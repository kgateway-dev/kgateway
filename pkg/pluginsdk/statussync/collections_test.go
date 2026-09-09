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

func (q *recordingQueue) Push(resource Resource) {
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

func TestRegisterResourceUsesConfiguredGVKForNormalizedCollections(t *testing.T) {
	stop := test.NewStop(t)
	fallback := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1alpha3", Kind: "ListenerSet"}
	objectGVK := schema.GroupVersionKind{Group: "gateway.networking.x-k8s.io", Version: "v1alpha1", Kind: "XListenerSet"}
	ls := &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: "listeners", Namespace: "default"}}
	ls.SetGroupVersionKind(objectGVK)
	col := krt.NewStaticCollection(nil, []*gwv1.ListenerSet{ls}, krt.WithStop(stop))

	sources := NewStatusCollections()
	queue := &recordingQueue{}
	RegisterResource(sources, fallback, col)
	sources.SetQueue(queue)

	require.Equal(t, fallback, queue.awaitResources(t, 1)[0].GroupVersionKind,
		"the configured write GVK must remain the coalescing key even when a normalized object has different TypeMeta")
}

func TestRegisterResourceByObjectGVKPreservesMixedSourceKinds(t *testing.T) {
	stop := test.NewStop(t)
	fallback := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1alpha3", Kind: "ListenerSet"}
	actual := schema.GroupVersionKind{Group: "gateway.networking.x-k8s.io", Version: "v1alpha1", Kind: "XListenerSet"}
	ls := &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Name: "listeners", Namespace: "default"}}
	ls.SetGroupVersionKind(actual)
	col := krt.NewStaticCollection(nil, []*gwv1.ListenerSet{ls}, krt.WithStop(stop))

	sources := NewStatusCollections()
	queue := &recordingQueue{}
	RegisterResourceByObjectGVK(sources, fallback, col)
	sources.SetQueue(queue)

	require.Equal(t, actual, queue.awaitResources(t, 1)[0].GroupVersionKind)
}

func TestResourceReportsReducesAndObservesContributionRemoval(t *testing.T) {
	stop := test.NewStop(t)
	gvk := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "HTTPRoute"}
	route := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"}}
	otherRoute := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default"}}
	objects := krt.NewStaticCollection(nil, []*gwv1.HTTPRoute{route, otherRoute}, krt.WithStop(stop))

	first := routeContribution(t, route, reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/first"}, "first")
	second := routeContribution(t, route, reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/second"}, "second")
	other := routeContribution(t, otherRoute, reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/first"}, "first")
	contributions := krt.NewStaticCollection(nil, []reports.StatusContribution{first, second, other}, krt.WithStop(stop))
	byTarget := krt.NewIndex(contributions, "status-target", func(c reports.StatusContribution) []reports.StatusKey {
		return []reports.StatusKey{c.Target}
	})
	reduced := NewResourceReports(
		objects,
		contributions,
		byTarget,
		func(object *gwv1.HTTPRoute) Resource {
			return Resource{GroupVersionKind: gvk, NamespacedName: types.NamespacedName{Namespace: object.Namespace, Name: object.Name}}
		},
		krt.WithStop(stop),
	)
	require.True(t, reduced.WaitUntilSynced(nil))

	routeKey := reports.StatusKey{GroupKind: gvk.GroupKind(), NamespacedName: types.NamespacedName{Namespace: route.Namespace, Name: route.Name}}
	otherKey := reports.StatusKey{GroupKind: gvk.GroupKind(), NamespacedName: types.NamespacedName{Namespace: otherRoute.Namespace, Name: otherRoute.Name}}
	require.Eventually(t, func() bool {
		current := reduced.GetKey(routeKey.String())
		return routeParentCount(current) == 2
	}, 5*time.Second, 10*time.Millisecond)

	updatedSecond := routeContribution(t, route, second.Source, "second-updated")
	contributions.UpdateObject(updatedSecond)
	require.Eventually(t, func() bool {
		current := reduced.GetKey(routeKey.String())
		return routeParentCount(current) == 2 &&
			hasRouteParent(current, "second-updated") &&
			!hasRouteParent(current, "second")
	}, 5*time.Second, 10*time.Millisecond, "updating one producer must replace only that producer's facts")

	contributions.DeleteObject(second.ResourceName())
	require.Eventually(t, func() bool {
		current := reduced.GetKey(routeKey.String())
		return routeParentCount(current) == 1
	}, 5*time.Second, 10*time.Millisecond, "removing one producer must remove only its parent status")
	require.Len(t, reduced.GetKey(otherKey.String()).Report.Route.Parents, 1,
		"an unrelated status owner must not be recomputed into a different value")

	contributions.DeleteObject(first.ResourceName())
	require.Eventually(t, func() bool {
		current := reduced.GetKey(routeKey.String())
		return current != nil && current.Report.Route == nil
	}, 5*time.Second, 10*time.Millisecond, "the raw owner must retain an empty reduction after its final contribution disappears")
}

// TestFetchedContributionsAreNotAliasedByIndexStorage pins the krt allocation behavior that
// reports.ReduceStatusContributions relies on: it sorts its input in place, which is only safe
// because krt.Fetch hands back a slice the caller owns rather than a view into the index's
// storage. If an upstream krt bump ever returned shared storage, sorting would silently permute
// index-owned state; this test fails loudly instead.
//
// It overwrites every slot of the fetched slice rather than merely sorting it: a sort can only
// permute elements within the slice's length, so slot-level writes detect every aliasing a sort
// could corrupt, and detect it regardless of the input's incidental order.
func TestFetchedContributionsAreNotAliasedByIndexStorage(t *testing.T) {
	stop := test.NewStop(t)
	gvk := schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "HTTPRoute"}
	route := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"}}
	otherRoute := &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default"}}
	objects := krt.NewStaticCollection(nil, []*gwv1.HTTPRoute{route, otherRoute}, krt.WithStop(stop))

	want := []reports.StatusContribution{
		routeContribution(t, route, reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/first"}, "first"),
		routeContribution(t, route, reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/second"}, "second"),
		routeContribution(t, otherRoute, reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/first"}, "first"),
		routeContribution(t, otherRoute, reports.StatusSource{Kind: reports.GatewayStatusSource, Name: "default/second"}, "second"),
	}
	contributions := krt.NewStaticCollection(nil, want, krt.WithStop(stop))
	byTarget := krt.NewIndex(contributions, "status-target", func(c reports.StatusContribution) []reports.StatusKey {
		return []reports.StatusKey{c.Target}
	})

	// Stands in for NewResourceReports, but destroys what it fetches instead of reducing it.
	var mu sync.Mutex
	overwritten := 0
	saboteur := krt.NewCollection(objects, func(kctx krt.HandlerContext, object *gwv1.HTTPRoute) *ResourceReports {
		res := Resource{GroupVersionKind: gvk, NamespacedName: types.NamespacedName{Namespace: object.Namespace, Name: object.Name}}
		target := reports.StatusKey{GroupKind: gvk.GroupKind(), NamespacedName: res.NamespacedName}
		fetched := krt.Fetch(kctx, contributions, krt.FilterIndex(byTarget, target))
		for i := range fetched {
			fetched[i] = reports.StatusContribution{}
		}
		mu.Lock()
		overwritten += len(fetched)
		mu.Unlock()
		return &ResourceReports{Resource: res}
	}, krt.WithStop(stop))
	require.True(t, saboteur.WaitUntilSynced(nil))
	// Without this the test passes vacuously if the fetch ever stops matching anything.
	mu.Lock()
	require.Equal(t, len(want), overwritten, "every contribution must have been fetched and overwritten")
	mu.Unlock()

	for _, expected := range [][]reports.StatusContribution{want[:2], want[2:]} {
		target := expected[0].Target
		require.ElementsMatch(t, expected, byTarget.Lookup(target),
			"mutating a krt.Fetch result must not reach the index's own storage for %s", target)
	}
	require.ElementsMatch(t, want, contributions.List(),
		"mutating a krt.Fetch result must not reach the source collection")
}

func routeParentCount(current *ResourceReports) int {
	if current == nil || current.Report.Route == nil {
		return 0
	}
	return len(current.Report.Route.Parents)
}

func hasRouteParent(current *ResourceReports, name string) bool {
	if current == nil || current.Report.Route == nil {
		return false
	}
	for parent := range current.Report.Route.Parents {
		if parent.Name == name {
			return true
		}
	}
	return false
}

func routeContribution(t *testing.T, route *gwv1.HTTPRoute, source reports.StatusSource, gatewayName string) reports.StatusContribution {
	t.Helper()
	reportMap := reports.NewReportMap()
	reports.NewReporter(&reportMap).Route(route).ParentRef(&gwv1.ParentReference{Name: gwv1.ObjectName(gatewayName)})
	contributions := reports.StatusContributionsFromReportMap(source, reportMap)
	require.Len(t, contributions, 1)
	return contributions[0]
}
