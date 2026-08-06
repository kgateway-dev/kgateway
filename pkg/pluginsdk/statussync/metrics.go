package statussync

import (
	"time"

	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	statusSubsystem = "status_syncer"
	syncerNameLabel = "syncer"
	nameLabel       = "name"
	namespaceLabel  = "namespace"
	resultLabel     = "result"
)

var (
	statusSyncHistogramBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}
	statusSyncsTotal           = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: statusSubsystem,
			Name:      "status_syncs_total",
			Help:      "Total number of status syncs",
		},
		[]string{nameLabel, namespaceLabel, syncerNameLabel, resultLabel},
	)
	statusSyncDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       statusSubsystem,
			Name:                            "status_sync_duration_seconds",
			Help:                            "Status sync duration",
			Buckets:                         statusSyncHistogramBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{nameLabel, namespaceLabel, syncerNameLabel},
	)
)

// SyncMetricLabels defines the labels for status sync metrics.
type SyncMetricLabels struct {
	Name      string
	Namespace string
	Syncer    string
}

func (s SyncMetricLabels) toMetricsLabels() []metrics.Label {
	return []metrics.Label{
		{Name: nameLabel, Value: s.Name},
		{Name: namespaceLabel, Value: s.Namespace},
		{Name: syncerNameLabel, Value: s.Syncer},
	}
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	statusSyncDuration.Reset()
	statusSyncsTotal.Reset()
}

// RecordStatusSync records the duration and result metrics for one status sync.
func RecordStatusSync(labels SyncMetricLabels, took time.Duration, err error) {
	if !metrics.Active() {
		return
	}
	statusSyncDuration.Observe(took.Seconds(), labels.toMetricsLabels()...)
	result := "success"
	if err != nil {
		result = "error"
	}
	statusSyncsTotal.Inc(append(labels.toMetricsLabels(),
		metrics.Label{Name: resultLabel, Value: result},
	)...)
}

// EndResourceStatusSyncOnWriteSuccess closes a resource's status sync unless the write
// failed in a way that nothing will retry. A resource whose write failed after every retry
// still carries a stale status and has no pending event to correct it — nothing changed on
// the informer — so its sync stays open and keeps the resources-out-of-sync signal raised
// until a later attempt persists it. RecordStatusSync separately reports the attempt as
// result=error.
//
// The precise condition is "no retry is pending", not "the status was persisted". Writer
// deliberately reports conflicts as success, so a conflicting write ends the sync without
// having persisted anything. That is deliberate rather than an oversight: a conflict means
// the API server already holds a newer resourceVersion, which the informer must deliver,
// which both re-enqueues the resource and starts a fresh sync. Distinguishing the two would
// need a persisted/deferred outcome on the OnSync callback; the residual today is a
// bounded skew in the sync counters, never an unwritten status.
//
// writeErr must be the write error alone, never a condition-derived one: a status carrying
// invalid conditions was still persisted, so the resource is in sync.
func EndResourceStatusSyncOnWriteSuccess(writeErr error, details kmetrics.ResourceSyncDetails) {
	if writeErr != nil {
		return
	}
	kmetrics.EndResourceStatusSync(details)
}
