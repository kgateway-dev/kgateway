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

// ReportsWrapper wraps a merged reports.ReportMap as a krt singleton value so
// derived status collections can Fetch it.
type ReportsWrapper struct {
	// lower case so krt doesn't error in debug handler
	reports reports.ReportMap
}

func NewReportsWrapper(rm reports.ReportMap) ReportsWrapper {
	return ReportsWrapper{reports: rm}
}

func (r ReportsWrapper) Reports() reports.ReportMap {
	return r.reports
}

func (r ReportsWrapper) ResourceName() string {
	return "report"
}

func (r ReportsWrapper) Equals(in ReportsWrapper) bool {
	return reports.EqualReportMaps(r.reports, in.reports)
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

// RegisterResourceReports enqueues an owner whenever its reduced contribution
// set changes. Deletes are ignored because the corresponding raw object is gone.
func RegisterResourceReports(s *StatusCollections, col krt.Collection[ResourceReports]) {
	s.Register(func(statusWriter WorkerQueue) krt.HandlerRegistration {
		return col.Register(func(event krt.Event[ResourceReports]) {
			if event.Event == controllers.EventDelete {
				return
			}
			statusWriter.Push(event.Latest().Resource)
		})
	})
}

// RegisterReports registers a report singleton as a reconciliation source. Initial Add
// events are skipped because RegisterResource replays every current object when leadership
// is acquired. On later report changes, changedResources returns only the object identities
// whose report fragments changed.
func RegisterReports(
	s *StatusCollections,
	reportCol krt.Collection[ReportsWrapper],
	changedResources func(old, current reports.ReportMap) []Resource,
) {
	s.Register(func(statusWriter WorkerQueue) krt.HandlerRegistration {
		return reportCol.Register(func(o krt.Event[ReportsWrapper]) {
			if o.Event == controllers.EventAdd {
				return
			}
			oldReports := reports.NewReportMap()
			if o.Old != nil {
				oldReports = o.Old.Reports()
			}
			currentReports := reports.NewReportMap()
			if o.New != nil {
				currentReports = o.New.Reports()
			}
			for _, res := range changedResources(oldReports, currentReports) {
				statusWriter.Push(res)
			}
		})
	})
}
