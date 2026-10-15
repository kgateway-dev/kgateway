package statussync

import (
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
)

// EndResourceStatusSyncOnWriteSuccess closes a resource's status sync unless the write
// failed in a way that nothing will retry. A resource whose write failed after every retry
// still carries a stale status and has no pending event to correct it — nothing changed on
// the informer — so its sync stays open and keeps the resources-out-of-sync signal raised
// until a later attempt persists it.
//
// The precise condition is "no retry is pending", not "the status was persisted". Writer
// deliberately reports conflicts as success, so a conflicting write ends the sync without
// having persisted anything. That is deliberate rather than an oversight: a conflict means
// the API server already holds a newer resourceVersion, which the informer must deliver,
// which both re-enqueues the resource and starts a fresh sync. Distinguishing the two would
// need a persisted/deferred outcome on the OnSync callback; the residual today is a
// bounded skew in the sync counters, never an unwritten status.
//
// writeErr must be the write error alone: a status carrying invalid conditions was still
// persisted, so the resource is in sync.
func EndResourceStatusSyncOnWriteSuccess(writeErr error, details kmetrics.ResourceSyncDetails) {
	if writeErr != nil {
		return
	}
	kmetrics.EndResourceStatusSync(details)
}
