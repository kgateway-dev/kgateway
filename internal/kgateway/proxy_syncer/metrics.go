package proxy_syncer

import (
	"strings"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	statusSubsystem    = "status_syncer"
	snapshotSubsystem  = "xds_snapshot"
	resourcesSubsystem = "resources"
	syncerNameLabel    = "syncer"
	gatewayLabel       = "gateway"
	nameLabel          = "name"
	namespaceLabel     = "namespace"
	resultLabel        = "result"
	resourceLabel      = "resource"
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

	transformsHistogramBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}
	snapshotTransformsTotal    = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: snapshotSubsystem,
			Name:      "transforms_total",
			Help:      "Total number of XDS snapshot transforms",
		},
		[]string{gatewayLabel, namespaceLabel, resultLabel},
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
		[]string{gatewayLabel, namespaceLabel},
	)
	snapshotResources = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: snapshotSubsystem,
			Name:      "resources",
			Help:      "Current number of resources in XDS snapshot",
		},
		[]string{gatewayLabel, namespaceLabel, resourceLabel},
	)

	resourcesHistogramBuckets          = []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600, 1200, 1800}
	resourcesStatusSyncsCompletedTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: resourcesSubsystem,
			Name:      "status_syncs_completed_total",
			Help:      "Total number of status syncs completed for resources",
		},
		[]string{gatewayLabel, namespaceLabel, resourceLabel})
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
		[]string{gatewayLabel, namespaceLabel, resourceLabel},
	)
	resourcesXDSSyncsTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: resourcesSubsystem,
			Name:      "xds_snapshot_syncs_total",
			Help:      "Total number of XDS snapshot syncs for resources",
		},
		[]string{gatewayLabel, namespaceLabel, resourceLabel})
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
		[]string{gatewayLabel, namespaceLabel, resourceLabel},
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
		{Name: gatewayLabel, Value: r.Gateway},
		{Name: namespaceLabel, Value: r.Namespace},
		{Name: resourceLabel, Value: r.Resource},
	}
}

// statusSyncMetricLabels defines the labels for status sync metrics.
type statusSyncMetricLabels struct {
	Name      string
	Namespace string
	Syncer    string
}

func (s statusSyncMetricLabels) toMetricsLabels() []metrics.Label {
	return []metrics.Label{
		{Name: nameLabel, Value: s.Name},
		{Name: namespaceLabel, Value: s.Namespace},
		{Name: syncerNameLabel, Value: s.Syncer},
	}
}

// collectStatusSyncMetrics is called at the start of a status sync function to
// begin metrics collection and returns a function called at the end to complete
// metrics recording.
func collectStatusSyncMetrics(labels statusSyncMetricLabels) func(error) {
	if !metrics.Active() {
		return func(err error) {}
	}

	start := time.Now()

	return func(err error) {
		duration := time.Since(start)

		statusSyncDuration.Observe(duration.Seconds(), labels.toMetricsLabels()...)

		result := "success"
		if err != nil {
			result = "error"
		}

		statusSyncsTotal.Inc(append(labels.toMetricsLabels(),
			metrics.Label{Name: resultLabel, Value: result},
		)...)
	}
}

// collectXDSTransformMetrics is called at the start of a transform function to
// begin metrics collection and returns a function called at the end to complete
// metrics recording.
func collectXDSTransformMetrics(clientKey string) func(error) {
	if !metrics.Active() {
		return func(err error) {}
	}

	start := time.Now()

	cd := getDetailsFromXDSClientResourceName(clientKey)
	return func(err error) {
		result := "success"
		if err != nil {
			result = "error"
		}

		snapshotTransformsTotal.Inc([]metrics.Label{
			{Name: gatewayLabel, Value: cd.Gateway},
			{Name: namespaceLabel, Value: cd.Namespace},
			{Name: resultLabel, Value: result},
		}...)

		duration := time.Since(start)

		snapshotTransformDuration.Observe(duration.Seconds(), []metrics.Label{
			{Name: gatewayLabel, Value: cd.Gateway},
			{Name: namespaceLabel, Value: cd.Namespace},
		}...)
	}
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
