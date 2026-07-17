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

// StatusRegistration attaches a handler to a status collection, feeding the
// given queue. It is invoked when status writing becomes enabled (on the leader).
type StatusRegistration = func(statusWriter WorkerQueue) krt.HandlerRegistration

// StatusCollections stores a set of collections that can write status.
// These are fed into a queue which can be dynamically set and unset to handle
// leader election: desired statuses are computed on every pod, but events only
// flow to the write queue while a queue is set (i.e. on the leader).
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

// SetQueue enables status writing. All registered collections attach handlers to the
// queue; krt replays existing state as Add events, so the full current desired state
// is swept on leadership acquisition.
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

// RegisterStatus takes a status collection and registers it to be managed by the status queue.
// The ObjectWithStatus elements must carry the live object (including its current status) in
// Obj and the desired status in Status; getStatus extracts the live status from the object.
// An event only reaches the write queue when the live status differs from the desired status,
// so lost writes self-heal: a conflicted or dropped write leaves live != desired, and the next
// informer event re-enqueues it.
//
// gvk identifies the resource kind for write dispatch. If the live object carries its own
// GroupVersionKind (e.g. legacy variants normalized into a shared type), that takes precedence.
func RegisterStatus[I controllers.Object, IS any](
	s *StatusCollections,
	gvk schema.GroupVersionKind,
	statusCol krt.StatusCollection[I, IS],
	getStatus func(I) IS,
) {
	reg := func(statusWriter WorkerQueue) krt.HandlerRegistration {
		return statusCol.Register(func(o krt.Event[krt.ObjectWithStatus[I, IS]]) {
			l := o.Latest()
			liveStatus := getStatus(l.Obj)
			if krt.Equal(liveStatus, l.Status) {
				// We want the same status we already have! No need for a write so skip this.
				// Note: the Equals() function on ObjectWithStatus does not compare these. It only
				// compares "old live + desired" == "new live + desired", so this callback triggers
				// when either the live OR the desired status changes; here we can do smarter
				// filtering and just check whether we already meet the desired state.
				logger.Debug("suppressing status change, live == desired", "resource", l.ResourceName(), "resource_version", l.Obj.GetResourceVersion())
				return
			}
			desired := l.Status
			if o.Event == controllers.EventDelete {
				// if the object is being deleted, we should not reset status
				var empty IS
				desired = empty
			}
			res := Resource{
				GroupVersionKind: gvk,
				NamespacedName:   config.NamespacedName(l.Obj),
			}
			if og := l.Obj.GetObjectKind().GroupVersionKind(); !og.Empty() {
				res.GroupVersionKind = og
			}
			statusWriter.Push(res, desired)
			logger.Debug("enqueued status update", "resource", l.ResourceName(), "resource_version", l.Obj.GetResourceVersion())
		})
	}
	s.Register(reg)
}
