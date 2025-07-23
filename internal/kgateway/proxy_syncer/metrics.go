package proxy_syncer

import (
	"strings"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	statusSubsystem    = "status_syncer"
	syncerNameLabel    = "syncer"
	snapshotSubsystem  = "xds_snapshot"
	resourcesSubsystem = "resources"
)

var (
	statusSyncHistogramBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}
	statusSyncsTotal           = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: statusSubsystem,
			Name:      "status_syncs_total",
			Help:      "Total number of status syncs",
		},
		[]string{syncerNameLabel, "result"},
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
		[]string{syncerNameLabel},
	)

	transformsHistogramBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}
	snapshotTransformsTotal    = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: snapshotSubsystem,
			Name:      "transforms_total",
			Help:      "Total number of XDS snapshot transforms",
		},
		[]string{"gateway", "namespace", "result"},
	)
	snapshotTransformDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       snapshotSubsystem,
			Name:                            "transform_duration_seconds",
			Help:                            "XDS snapshot transform duration",
			Buckets:                         transformsHistogramBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{"gateway", "namespace"},
	)
	snapshotResources = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: snapshotSubsystem,
			Name:      "resources",
			Help:      "Current number of resources in XDS snapshot",
		},
		[]string{"gateway", "namespace", "resource"},
	)

	resourcesHistogramBuckets          = []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600, 1200, 1800}
	resourcesStatusSyncsCompletedTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: resourcesSubsystem,
			Name:      "status_syncs_completed_total",
			Help:      "Total number of status syncs completed for resources",
		},
		[]string{"gateway", "namespace", "resource"})
	resourcesStatusSyncDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       resourcesSubsystem,
			Name:                            "status_sync_duration_seconds",
			Help:                            "Duration of time for a resource update to receive a status report",
			Buckets:                         resourcesHistogramBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{"gateway", "namespace", "resource"},
	)
	resourcesXDSSyncsTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: resourcesSubsystem,
			Name:      "xds_snapshot_syncs_total",
			Help:      "Total number of XDS snapshot syncs for resources",
		},
		[]string{"gateway", "namespace", "resource"})
	resourcesXDSyncDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       resourcesSubsystem,
			Name:                            "xds_snapshot_sync_duration_seconds",
			Help:                            "Duration of time for a resource update to be synced in XDS snapshots",
			Buckets:                         resourcesHistogramBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{"gateway", "namespace", "resource"},
	)
)

// snapshotResourcesMetricLabels defines the labels for XDS snapshot resources metrics.
type snapshotResourcesMetricLabels struct {
	Gateway   string
	Namespace string
	Resource  string
}

func (r snapshotResourcesMetricLabels) toMetricsLabels() []metrics.Label {
	return []metrics.Label{
		{Name: "gateway", Value: r.Gateway},
		{Name: "namespace", Value: r.Namespace},
		{Name: "resource", Value: r.Resource},
	}
}

// statusSyncMetricsRecorder defines the interface for recording status syncer metrics.
type statusSyncMetricsRecorder interface {
	StatusSyncStart() func(error)
}

// statusSyncMetrics records metrics for status syncer operations.
type statusSyncMetrics struct {
	syncerName         string
	statusSyncsTotal   metrics.Counter
	statusSyncDuration metrics.Histogram
}

// newStatusSyncMetricsRecorder creates a new recorder for status syncer metrics.
func newStatusSyncMetricsRecorder(syncerName string) statusSyncMetricsRecorder {
	if !metrics.Active() {
		return &nullStatusSyncMetricsRecorder{}
	}

	m := &statusSyncMetrics{
		syncerName:         syncerName,
		statusSyncsTotal:   statusSyncsTotal,
		statusSyncDuration: statusSyncDuration,
	}

	return m
}

type nullStatusSyncMetricsRecorder struct{}

func (m *nullStatusSyncMetricsRecorder) StatusSyncStart() func(error) {
	return func(err error) {}
}

// StatusSyncStart is called at the start of a status sync function to begin metrics
// collection and returns a function called at the end to complete metrics recording.
func (m *statusSyncMetrics) StatusSyncStart() func(error) {
	start := time.Now()

	return func(err error) {
		duration := time.Since(start)

		m.statusSyncDuration.Observe(duration.Seconds(),
			metrics.Label{Name: syncerNameLabel, Value: m.syncerName})

		result := "success"
		if err != nil {
			result = "error"
		}

		m.statusSyncsTotal.Inc([]metrics.Label{
			{Name: syncerNameLabel, Value: m.syncerName},
			{Name: "result", Value: result},
		}...)
	}
}

// snapshotMetricsRecorder defines the interface for recording XDS snapshot metrics.
type snapshotMetricsRecorder interface {
	transformStart(string) func(error)
}

// snapshotMetrics records metrics for collection operations.
type snapshotMetrics struct {
	transformsTotal   metrics.Counter
	transformDuration metrics.Histogram
}

var _ snapshotMetricsRecorder = &snapshotMetrics{}

// newSnapshotMetricsRecorder creates a new recorder for XDS snapshot metrics.
func newSnapshotMetricsRecorder() snapshotMetricsRecorder {
	if !metrics.Active() {
		return &nullSnapshotMetricsRecorder{}
	}

	m := &snapshotMetrics{
		transformsTotal:   snapshotTransformsTotal,
		transformDuration: snapshotTransformDuration,
	}

	return m
}

// transformStart is called at the start of a transform function to begin metrics
// collection and returns a function called at the end to complete metrics recording.
func (m *snapshotMetrics) transformStart(clientKey string) func(error) {
	start := time.Now()

	cd := getDetailsFromXDSClientResourceName(clientKey)
	return func(err error) {
		result := "success"
		if err != nil {
			result = "error"
		}

		m.transformsTotal.Inc([]metrics.Label{
			{Name: "gateway", Value: cd.Gateway},
			{Name: "namespace", Value: cd.Namespace},
			{Name: "result", Value: result},
		}...)

		duration := time.Since(start)

		m.transformDuration.Observe(duration.Seconds(), []metrics.Label{
			{Name: "gateway", Value: cd.Gateway},
			{Name: "namespace", Value: cd.Namespace},
		}...)
	}
}

type nullSnapshotMetricsRecorder struct{}

func (m *nullSnapshotMetricsRecorder) transformStart(string) func(error) {
	return func(err error) {}
}

type resourceNameDetails struct {
	Role      string
	Namespace string
	Gateway   string
}

// getDetailsFromXDSClientResourceName extracts details from an XDS client resource name.
func getDetailsFromXDSClientResourceName(resourceName string) resourceNameDetails {
	res := resourceNameDetails{
		Role:      "unknown",
		Namespace: "unknown",
		Gateway:   "unknown",
	}

	pks := strings.SplitN(resourceName, "~", 5)

	if len(pks) > 0 {
		res.Role = pks[0]
	}

	if len(pks) > 1 {
		res.Namespace = pks[1]
	}

	if len(pks) > 2 {
		res.Gateway = pks[2]
	}

	return res
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	statusSyncDuration.Reset()
	statusSyncsTotal.Reset()
	snapshotTransformsTotal.Reset()
	snapshotTransformDuration.Reset()
	snapshotResources.Reset()
	resourcesStatusSyncsCompletedTotal.Reset()
}
