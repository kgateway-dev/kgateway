// Derived from https://github.com/agentgateway/agentgateway controller/pkg/syncer/status/collection.go (Apache 2.0).

package statussync

import (
	"sync"

	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var logger = logging.New("statussync")

// ResourceReports is the current reduction of all status contributions for one
// Kubernetes object. It remains present while the raw object exists, even when
// Report is empty, so disappearance of the last contribution is observable.
type ResourceReports struct {
	Resource Resource
	Target   reports.StatusKey
	Report   reports.StatusReport
}

func (r ResourceReports) ResourceName() string {
	return r.Target.String()
}

func (r ResourceReports) Equals(other ResourceReports) bool {
	return r.Resource == other.Resource &&
		r.Target == other.Target &&
		r.Report.Equals(other.Report)
}

// NewResourceReports builds one lightweight report reduction per raw object.
// KRT tracks the filtered contribution dependency, so only the owner of a
// changed contribution recomputes.
func NewResourceReports[I controllers.Object](
	objects krt.Collection[I],
	contributions krt.Collection[reports.StatusContribution],
	byTarget krt.Index[reports.StatusKey, reports.StatusContribution],
	resource func(I) Resource,
	opts ...krt.CollectionOption,
) krt.Collection[ResourceReports] {
	return krt.NewCollection(objects, func(kctx krt.HandlerContext, object I) *ResourceReports {
		res := resource(object)
		target := reports.StatusKey{GroupKind: res.GroupVersionKind.GroupKind(), NamespacedName: res.NamespacedName}
		fragments := krt.Fetch(kctx, contributions, krt.FilterIndex(byTarget, target))
		return &ResourceReports{
			Resource: res,
			Target:   target,
			Report:   reports.ReduceStatusContributions(fragments),
		}
	}, opts...)
}

// StatusRegistration attaches a source handler that feeds the given queue. It is invoked
// when status writing becomes enabled on the leader.
type StatusRegistration = func(statusWriter WorkerQueue) krt.HandlerRegistration

// StatusCollections stores the raw-resource and report event sources that can trigger a
// status reconciliation. Handlers are attached only on the leader, and they enqueue only
// resource identities; desired statuses are built just-in-time by the writer.
type StatusCollections struct {
	mu           sync.Mutex
	constructors []StatusRegistration
	active       []krt.HandlerRegistration
	queue        WorkerQueue
	// reportSyncs are the HasSynced funcs of every registered report reducer. Tracking
	// them here rather than at each call site is what makes RegisterResourceReports the
	// only way to register a reducer: there is no unsafe variant to reach for.
	reportSyncs []func() bool
}

func NewStatusCollections() *StatusCollections {
	return &StatusCollections{}
}

func (s *StatusCollections) Register(sr StatusRegistration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.constructors = append(s.constructors, sr)
	// If the queue is already active (registration raced leader election), attach immediately.
	if s.queue != nil {
		s.active = append(s.active, sr(s.queue))
	}
}

// UnsetQueue disables status writing, detaching all handlers.
func (s *StatusCollections) UnsetQueue() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = nil
	for _, act := range s.active {
		act.UnregisterHandler()
	}
	s.active = nil
}

// SetQueue enables status writing. All registered sources attach handlers to the queue;
// raw KRT collections replay current objects as Add events to sweep them on leadership.
func (s *StatusCollections) SetQueue(queue WorkerQueue) []krt.Syncer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = queue
	s.active = slices.Map(s.constructors, func(reg StatusRegistration) krt.HandlerRegistration {
		return reg(queue)
	})
	return slices.Map(s.active, func(e krt.HandlerRegistration) krt.Syncer {
		return e
	})
}

// RegisterResource registers an existing raw KRT collection as a reconciliation source.
// Status-only informer updates are intentionally included: they provide the self-healing
// event after a write conflict or after another controller updates a shared status field.
// Deletes are ignored because there is no remaining object to update.
func RegisterResource[I controllers.Object](
	s *StatusCollections,
	gvk schema.GroupVersionKind,
	col krt.Collection[I],
) {
	registerResource(s, col, func(I) schema.GroupVersionKind { return gvk })
}

// RegisterResourceByObjectGVK registers a normalized collection whose objects retain
// distinct source GVKs in TypeMeta, such as the combined ListenerSet/XListenerSet source.
func RegisterResourceByObjectGVK[I controllers.Object](
	s *StatusCollections,
	fallback schema.GroupVersionKind,
	col krt.Collection[I],
) {
	registerResource(s, col, func(obj I) schema.GroupVersionKind {
		gvk := obj.GetObjectKind().GroupVersionKind()
		if gvk.Empty() {
			return fallback
		}
		return gvk
	})
}

func registerResource[I controllers.Object](
	s *StatusCollections,
	col krt.Collection[I],
	gvkFor func(I) schema.GroupVersionKind,
) {
	reg := func(statusWriter WorkerQueue) krt.HandlerRegistration {
		return col.Register(func(o krt.Event[I]) {
			if o.Event == controllers.EventDelete {
				return
			}
			obj := o.Latest()
			res := Resource{
				GroupVersionKind: gvkFor(obj),
				NamespacedName:   config.NamespacedName(obj),
			}
			statusWriter.Push(res)
			logger.Debug("enqueued status reconciliation", "resource", res.NamespacedName.String(), "resource_version", obj.GetResourceVersion())
		})
	}
	s.Register(reg)
}

// HasSynced reports whether every registered report reducer has synced. The leader's
// startup sweep must not write status built from a reducer that has not yet observed its
// contributions, so this is part of the status syncer's cache-sync barrier. It reads the
// current registration set on every call, so reducers registered after the barrier was
// installed are still covered.
func (s *StatusCollections) HasSynced() bool {
	s.mu.Lock()
	syncs := slices.Clone(s.reportSyncs)
	s.mu.Unlock()
	for _, hasSynced := range syncs {
		if !hasSynced() {
			return false
		}
	}
	return true
}

// RegisterResourceReports enqueues an owner whenever its reduced contribution
// set changes, and records the reducer in the cache-sync barrier reported by HasSynced.
// Deletes are ignored because the corresponding raw object is gone.
func RegisterResourceReports(s *StatusCollections, col krt.Collection[ResourceReports]) {
	s.mu.Lock()
	s.reportSyncs = append(s.reportSyncs, col.HasSynced)
	s.mu.Unlock()
	s.Register(func(statusWriter WorkerQueue) krt.HandlerRegistration {
		return col.Register(func(event krt.Event[ResourceReports]) {
			if event.Event == controllers.EventDelete {
				return
			}
			statusWriter.Push(event.Latest().Resource)
		})
	})
}
