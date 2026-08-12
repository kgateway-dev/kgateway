package proxy_syncer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

const (
	// The controller name the writers are built with, as the controller runs them.
	cycleController      = wellknown.DefaultGatewayControllerName
	cycleOtherController = "other.example/controller"
	cycleNamespace       = "default"
)

// cycleParentRef is both the route's spec parentRef and the ref its stale status entry
// carries. They must be the same value: the builder finds the live conditions to preserve
// LastTransitionTime from by matching the parentRef exactly.
var cycleParentRef = gwv1.ParentReference{Name: "gw"}

func staleTime() metav1.Time {
	return metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
}

// cycleGateway is a Gateway whose live status is stale in every way that matters here: our
// own conditions carry an old LastTransitionTime and an outdated reason/observedGeneration,
// and one condition belongs to a different writer entirely.
func cycleGateway() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gw", Namespace: cycleNamespace, Generation: 2, ResourceVersion: "1",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: wellknown.DefaultGatewayClassName,
			Listeners: []gwv1.Listener{{
				Name:     "http",
				Protocol: gwv1.HTTPProtocolType,
				Port:     80,
			}},
		},
		Status: gwv1.GatewayStatus{
			Conditions: []metav1.Condition{
				{
					Type:               string(gwv1.GatewayConditionAccepted),
					Status:             metav1.ConditionFalse,
					Reason:             string(gwv1.GatewayReasonPending),
					Message:            "waiting for controller",
					ObservedGeneration: 1,
					LastTransitionTime: staleTime(),
				},
				{
					Type:               "example.com/SomeCondition",
					Status:             metav1.ConditionTrue,
					Reason:             "Whatever",
					LastTransitionTime: staleTime(),
				},
			},
			Listeners: []gwv1.ListenerStatus{{
				Name:           "http",
				SupportedKinds: []gwv1.RouteGroupKind{},
				Conditions: []metav1.Condition{{
					Type:               string(gwv1.ListenerConditionProgrammed),
					Status:             metav1.ConditionFalse,
					Reason:             string(gwv1.ListenerReasonPending),
					ObservedGeneration: 1,
					LastTransitionTime: staleTime(),
				}},
			}},
		},
	}
}

// cycleRoute is a route parented by both us and another controller, whose foreign parents
// are stored in the reverse of the order our merge canonicalizes to. Publishing reorders
// them once; if anything about that reordering were unstable, the informer echo of our own
// write would ask us to reorder again, forever.
func cycleRoute() *gwv1.HTTPRoute {
	foreign := func(name string) gwv1.RouteParentStatus {
		return gwv1.RouteParentStatus{
			ParentRef:      gwv1.ParentReference{Name: gwv1.ObjectName(name)},
			ControllerName: cycleOtherController,
			Conditions: []metav1.Condition{{
				Type:               string(gwv1.RouteConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(gwv1.RouteReasonAccepted),
				LastTransitionTime: staleTime(),
			}},
		}
	}
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: "route", Namespace: cycleNamespace, Generation: 2, ResourceVersion: "1",
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{cycleParentRef}},
		},
		Status: gwv1.HTTPRouteStatus{RouteStatus: gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{
			foreign("zzz-their-gw"),
			foreign("aaa-their-gw"),
			{
				ParentRef:      cycleParentRef,
				ControllerName: cycleController,
				Conditions: []metav1.Condition{{
					Type:               string(gwv1.RouteConditionAccepted),
					Status:             metav1.ConditionFalse,
					Reason:             string(gwv1.RouteReasonNoMatchingParent),
					ObservedGeneration: 1,
					LastTransitionTime: staleTime(),
				}},
			},
		}}},
	}
}

// cycleListenerSet is a promoted ListenerSet whose live status is stale in the same ways
// cycleGateway's is, plus one per-listener condition: ListenerSet status nests conditions two
// levels deep, and the nested level preserves LastTransitionTime through a separate lookup
// (by listener name) than the top-level one.
func cycleListenerSet() *gwv1.ListenerSet {
	return &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ls", Namespace: cycleNamespace, Generation: 2, ResourceVersion: "1",
		},
		Spec: gwv1.ListenerSetSpec{
			ParentRef: gwv1.ParentGatewayReference{Name: "gw"},
			Listeners: []gwv1.ListenerEntry{{
				Name:     "http",
				Protocol: gwv1.HTTPProtocolType,
				Port:     8080,
			}},
		},
		Status: gwv1.ListenerSetStatus{
			Conditions: []metav1.Condition{
				{
					Type:               string(gwv1.ListenerSetConditionAccepted),
					Status:             metav1.ConditionFalse,
					Reason:             string(gwv1.ListenerSetReasonPending),
					Message:            "waiting for controller",
					ObservedGeneration: 1,
					LastTransitionTime: staleTime(),
				},
				{
					Type:               "example.com/SomeCondition",
					Status:             metav1.ConditionTrue,
					Reason:             "Whatever",
					LastTransitionTime: staleTime(),
				},
			},
			Listeners: []gwv1.ListenerEntryStatus{{
				Name:           "http",
				SupportedKinds: []gwv1.RouteGroupKind{},
				Conditions: []metav1.Condition{{
					Type:               string(gwv1.ListenerConditionProgrammed),
					Status:             metav1.ConditionFalse,
					Reason:             string(gwv1.ListenerReasonPending),
					ObservedGeneration: 1,
					LastTransitionTime: staleTime(),
				}},
			}},
		},
	}
}

// recordingQueue records the identities the status collections enqueue. The cycle under test
// is "write -> informer event -> enqueue -> writer decides again", so the test drives the
// writer itself and uses this to observe that the echo really arrived; running a worker pool
// instead would race the informer for who reads the object first and make the write count
// nondeterministic for reasons unrelated to convergence.
type recordingQueue struct {
	mu     sync.Mutex
	pushes map[statussync.Resource]int
}

func (q *recordingQueue) Push(res statussync.Resource) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pushes[res]++
}

func (q *recordingQueue) count(res statussync.Resource) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.pushes[res]
}

type statusCycleFixture struct {
	gateways          krt.Collection[*gwv1.Gateway]
	routes            krt.Collection[*gwv1.HTTPRoute]
	listenerSets      krt.Collection[*gwv1.ListenerSet]
	gatewayWriter     statussync.Writer[*gwv1.Gateway, gwv1.GatewayStatus]
	routeWriter       statussync.Writer[*gwv1.HTTPRoute, gwv1.RouteStatus]
	listenerSetWriter statussync.Writer[*gwv1.ListenerSet, gwv1.ListenerSetStatus]
	queue             *recordingQueue
	// statusWrites counts status subresource writes that reached the API server for one
	// resource plural (e.g. "gateways").
	statusWrites func(resource string) int
}

// liveGateway and liveRoute read the object the writers read: the collection the informer
// feeds, which is where our own writes land once they echo back.
func (f statusCycleFixture) liveGateway(t *testing.T) *gwv1.Gateway {
	t.Helper()
	gw := f.gateways.GetKey(cycleNamespace + "/gw")
	require.NotNil(t, gw, "the gateway collection should hold the seeded gateway")
	return *gw
}

func (f statusCycleFixture) liveRoute(t *testing.T) *gwv1.HTTPRoute {
	t.Helper()
	route := f.routes.GetKey(cycleNamespace + "/route")
	require.NotNil(t, route, "the route collection should hold the seeded route")
	return *route
}

func (f statusCycleFixture) liveListenerSet(t *testing.T) *gwv1.ListenerSet {
	t.Helper()
	ls := f.listenerSets.GetKey(cycleNamespace + "/ls")
	require.NotNil(t, ls, "the listener set collection should hold the seeded listener set")
	return *ls
}

// newStatusCycleFixture wires the production Gateway, HTTPRoute and ListenerSet writers
// against a fake API server whose informers echo every write back into the collections those
// writers read.
func newStatusCycleFixture(t *testing.T) statusCycleFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	gw, route, ls := cycleGateway(), cycleRoute(), cycleListenerSet()
	c := apifake.NewClient(t, gw, route, ls)
	fake := c.GatewayAPI().(*gatewayfake.Clientset)
	// Without this the first status write erases the spec these builders read, and every
	// rebuild afterwards would legitimately differ from the last.
	apifake.InstallStatusSubresourceReactor(fake)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	f := kclient.Filter{ObjectFilter: c.ObjectFilter()}

	gatewayClient := kclient.NewFiltered[*gwv1.Gateway](c, f)
	gateways := krt.WrapClient(gatewayClient, krtopts.ToOptions("Gateways")...)
	routeClient := kclient.NewFiltered[*gwv1.HTTPRoute](c, f)
	routes := krt.WrapClient(routeClient, krtopts.ToOptions("HTTPRoutes")...)
	// The controller joins the promoted and legacy flavors into one collection; the promoted
	// half alone is what the writer under test reads, and the only half a typed client sees.
	listenerSetClient := kclient.NewFiltered[*gwv1.ListenerSet](c, f)
	listenerSets := krt.WrapClient(listenerSetClient, krtopts.ToOptions("ListenerSets")...)

	// Report fragments from the real reporter, as translation would produce them.
	reportMap := reports.NewReportMap()
	reporter := reports.NewReporter(&reportMap)
	reporter.Gateway(gw)
	reporter.Route(route).ParentRef(&cycleParentRef)
	reporter.ListenerSet(ls)
	contributions := krt.NewStaticCollection(nil,
		reports.StatusContributionsFromReportMap(
			reports.StatusSource{Kind: reports.GatewayStatusSource, Name: cycleNamespace + "/gw"}, reportMap),
		krtopts.ToOptions("StatusContributions")...)
	byTarget := krtpkg.UnnamedIndex(contributions, func(contribution reports.StatusContribution) []reports.StatusKey {
		return []reports.StatusKey{contribution.Target}
	})

	collections := statussync.NewStatusCollections()
	gatewayReports := statussync.RegisterKind(collections, wellknown.GatewayGVK, gateways,
		contributions, byTarget, krtopts.ToOptions("GatewayStatusReports")...)
	routeReports := statussync.RegisterKind(collections, wellknown.HTTPRouteGVK, routes,
		contributions, byTarget, krtopts.ToOptions("HTTPRouteStatusReports")...)
	listenerSetReports := statussync.RegisterKindByObjectGVK(collections, wellknown.ListenerSetGVK,
		listenerSets, contributions, byTarget, krtopts.ToOptions("ListenerSetStatusReports")...)

	fixture := statusCycleFixture{
		gateways:          gateways,
		routes:            routes,
		listenerSets:      listenerSets,
		gatewayWriter:     gatewayWriter(c, f, gateways, gatewayReports),
		listenerSetWriter: listenerSetWriter(c, f, listenerSets, listenerSetReports),
		routeWriter: routeWriter[*gwv1.HTTPRoute, *gwv1.HTTPRoute](c, f, routes, routeReports,
			wellknown.HTTPRouteGVK, "httpRoute", wellknown.HTTPRouteGVR, wellknown.HTTPRouteKind, cycleController,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.HTTPRoute {
				return &gwv1.HTTPRoute{ObjectMeta: om, Status: gwv1.HTTPRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1.HTTPRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		),
		queue: &recordingQueue{pushes: map[statussync.Resource]int{}},
	}

	c.RunAndWait(ctx.Done())
	require.Eventually(t, collections.HasSynced, 5*time.Second, 10*time.Millisecond,
		"report reducers should sync")
	require.Eventually(t, func() bool {
		return gateways.GetKey(cycleNamespace+"/gw") != nil &&
			routes.GetKey(cycleNamespace+"/route") != nil &&
			listenerSets.GetKey(cycleNamespace+"/ls") != nil
	}, 5*time.Second, 10*time.Millisecond, "collections should observe the seeded objects")

	// Attaching the queue is what leadership does; from here on every informer event,
	// including the echo of our own writes, enqueues the resource again.
	collections.SetQueue(fixture.queue)

	fixture.statusWrites = func(resource string) int {
		n := 0
		for _, a := range fake.Actions() {
			if a.GetVerb() == "update" && a.GetSubresource() == "status" && a.GetResource().Resource == resource {
				n++
			}
		}
		return n
	}
	return fixture
}

// statusOf reads one object's live status out of the collection its writer reads, returning
// nil while the object is absent. The status is boxed because the three writers under test
// publish three unrelated status types.
func statusOf[O controllers.ComparableObject](col krt.Collection[O], name string, status func(O) any) func() any {
	return func() any {
		obj := col.GetKey(cycleNamespace + "/" + name)
		if obj == nil || controllers.IsNil(*obj) {
			return nil
		}
		return status(*obj)
	}
}

func gatewayResource() statussync.Resource {
	return statussync.Resource{
		GroupVersionKind: wellknown.GatewayGVK,
		NamespacedName:   types.NamespacedName{Namespace: cycleNamespace, Name: "gw"},
	}
}

func routeResource() statussync.Resource {
	return statussync.Resource{
		GroupVersionKind: wellknown.HTTPRouteGVK,
		NamespacedName:   types.NamespacedName{Namespace: cycleNamespace, Name: "route"},
	}
}

func listenerSetResource() statussync.Resource {
	return statussync.Resource{
		GroupVersionKind: wellknown.ListenerSetGVK,
		NamespacedName:   types.NamespacedName{Namespace: cycleNamespace, Name: "ls"},
	}
}

// TestStatusWritersConvergeAfterOneWrite is the convergence guard for the writers this
// controller actually runs.
//
// Every status write echoes back as an informer event that re-enqueues the resource, so each
// writer is always asked a second time about the status it just wrote, and the only thing
// that stops the cycle is its live-vs-desired skip. That skip only fires if rebuilding from
// what we wrote reproduces what we wrote — so a builder that regenerated LastTransitionTime,
// or that reordered entries unstably, would turn the designed one-shot echo into a permanent
// write loop while every unit test of the skip itself kept passing.
func TestStatusWritersConvergeAfterOneWrite(t *testing.T) {
	f := newStatusCycleFixture(t)
	ctx := context.Background()

	tests := map[string]struct {
		resource statussync.Resource
		plural   string
		apply    func()
		// liveStatus reads the status the writer reads, from the collection it reads it from.
		// It is how the test observes its own write coming back, which the queue alone cannot
		// tell it: attaching the queue replays every seeded object as an Add, so a push may
		// arrive that is not the echo of our write.
		liveStatus func() any
		verify     func(t *testing.T)
	}{
		"gateway": {
			resource:   gatewayResource(),
			plural:     "gateways",
			apply:      func() { f.gatewayWriter.ApplyStatus(ctx, gatewayResource()) },
			liveStatus: statusOf(f.gateways, "gw", func(o *gwv1.Gateway) any { return o.Status }),
			verify: func(t *testing.T) {
				live := f.liveGateway(t)
				accepted := meta.FindStatusCondition(live.Status.Conditions, string(gwv1.GatewayConditionAccepted))
				require.NotNil(t, accepted, "our Accepted condition must be published")
				require.Equal(t, metav1.ConditionTrue, accepted.Status)
				require.Equal(t, int64(2), accepted.ObservedGeneration)
				require.NotNil(t,
					meta.FindStatusCondition(live.Status.Conditions, "example.com/SomeCondition"),
					"a condition we do not own must survive the write")
			},
		},
		"listenerset": {
			resource:   listenerSetResource(),
			plural:     "listenersets",
			apply:      func() { f.listenerSetWriter.ApplyStatus(ctx, listenerSetResource()) },
			liveStatus: statusOf(f.listenerSets, "ls", func(o *gwv1.ListenerSet) any { return o.Status }),
			verify: func(t *testing.T) {
				live := f.liveListenerSet(t)
				accepted := meta.FindStatusCondition(live.Status.Conditions, string(gwv1.ListenerSetConditionAccepted))
				require.NotNil(t, accepted, "our Accepted condition must be published")
				require.Equal(t, metav1.ConditionTrue, accepted.Status)
				require.Equal(t, int64(2), accepted.ObservedGeneration)
				require.NotNil(t,
					meta.FindStatusCondition(live.Status.Conditions, "example.com/SomeCondition"),
					"a condition we do not own must survive the write")
				require.Len(t, live.Status.Listeners, 1, "the spec listener must get a status entry")
				programmed := meta.FindStatusCondition(live.Status.Listeners[0].Conditions,
					string(gwv1.ListenerConditionProgrammed))
				require.NotNil(t, programmed, "the nested listener conditions must be published too")
				require.Equal(t, metav1.ConditionTrue, programmed.Status)
			},
		},
		"httproute": {
			resource:   routeResource(),
			plural:     "httproutes",
			apply:      func() { f.routeWriter.ApplyStatus(ctx, routeResource()) },
			liveStatus: statusOf(f.routes, "route", func(o *gwv1.HTTPRoute) any { return o.Status.RouteStatus }),
			verify: func(t *testing.T) {
				live := f.liveRoute(t)
				names := make([]gwv1.ObjectName, 0, len(live.Status.Parents))
				for _, p := range live.Status.Parents {
					names = append(names, p.ParentRef.Name)
				}
				require.Equal(t, []gwv1.ObjectName{"aaa-their-gw", "gw", "zzz-their-gw"}, names,
					"the merge publishes one canonical order, including the parents we do not own")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			before := f.queue.count(tc.resource)
			stale := tc.liveStatus()

			tc.apply()
			require.Equal(t, 1, f.statusWrites(tc.plural), "the stale status must be corrected in one write")

			// The write comes back through the informer and re-enqueues the resource; that
			// event is the one every writer must absorb without writing again. Both halves
			// matter: the status is what the next two passes rebuild from, and the enqueue is
			// what would drive them in production.
			require.Eventually(t, func() bool {
				return !krt.Equal(tc.liveStatus(), stale) && f.queue.count(tc.resource) > before
			}, 5*time.Second, 10*time.Millisecond,
				"our own write must reach the collection the writer reads, and re-enqueue the resource")

			tc.verify(t)

			// Two more passes over the echoed object: the first proves the skip fires, the
			// second proves it is not merely a one-time coincidence of the first rebuild.
			tc.apply()
			tc.apply()
			require.Equal(t, 1, f.statusWrites(tc.plural),
				"rebuilding from the status we just wrote must produce the same status, or writes never stop")
		})
	}
}

// TestStatusWritersAreIdempotent runs the same invariant through the exported harness, on
// the live objects, so a writer added or changed later fails here rather than in production.
// Writers registered downstream via WithStatusRegistration should run this too.
func TestStatusWritersAreIdempotent(t *testing.T) {
	f := newStatusCycleFixture(t)

	require.True(t, statussync.WriterWouldWrite(f.gatewayWriter, f.liveGateway(t)),
		"the seeded gateway status must actually be written, or the check below proves nothing")
	require.NoError(t, statussync.CheckWriterIdempotent(f.gatewayWriter, f.liveGateway(t),
		func(current *gwv1.Gateway, status gwv1.GatewayStatus) *gwv1.Gateway {
			next := current.DeepCopy()
			next.Status = *status.DeepCopy()
			return next
		}))

	require.True(t, statussync.WriterWouldWrite(f.routeWriter, f.liveRoute(t)),
		"the seeded route status must actually be written, or the check below proves nothing")
	require.NoError(t, statussync.CheckWriterIdempotent(f.routeWriter, f.liveRoute(t),
		func(current *gwv1.HTTPRoute, status gwv1.RouteStatus) *gwv1.HTTPRoute {
			next := current.DeepCopy()
			next.Status.RouteStatus = *status.DeepCopy()
			return next
		}))

	require.True(t, statussync.WriterWouldWrite(f.listenerSetWriter, f.liveListenerSet(t)),
		"the seeded listener set status must actually be written, or the check below proves nothing")
	require.NoError(t, statussync.CheckWriterIdempotent(f.listenerSetWriter, f.liveListenerSet(t),
		func(current *gwv1.ListenerSet, status gwv1.ListenerSetStatus) *gwv1.ListenerSet {
			next := current.DeepCopy()
			next.Status = *status.DeepCopy()
			return next
		}))
}
