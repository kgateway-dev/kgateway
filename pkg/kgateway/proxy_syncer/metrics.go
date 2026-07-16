package proxy_syncer

import (
	"strings"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
)

const (
	snapshotSubsystem = "xds_snapshot"
	gatewayLabel      = "gateway"
	nameLabel         = "name"
	namespaceLabel    = "namespace"
	resultLabel       = "result"
	resourceLabel     = "resource"
)

var (
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

		snapshotTransformsTotal.Inc(
			metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
			metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
			metrics.Label{Name: resultLabel, Value: result},
		)

		duration := time.Since(start)

		snapshotTransformDuration.Observe(duration.Seconds(),
			metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
			metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
		)
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
	statussync.ResetMetrics()
	snapshotTransformsTotal.Reset()
	snapshotTransformDuration.Reset()
	snapshotResources.Reset()
}
