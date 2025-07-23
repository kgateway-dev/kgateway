package controller

import (
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	controllerSubsystem = "controller"
	controllerNameLabel = "controller"
)

var (
	reconcileHistogramBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}
	reconciliationsTotal      = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: controllerSubsystem,
			Name:      "reconciliations_total",
			Help:      "Total number of controller reconciliations",
		},
		[]string{controllerNameLabel, "result"},
	)
	reconcileDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       controllerSubsystem,
			Name:                            "reconcile_duration_seconds",
			Help:                            "Reconcile duration for controller",
			Buckets:                         reconcileHistogramBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{controllerNameLabel},
	)
	reconciliationsRunning = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: controllerSubsystem,
			Name:      "reconciliations_running",
			Help:      "Number of reconciliations currently running",
		},
		[]string{controllerNameLabel},
	)
)

// controllerMetricsRecorder defines the interface for recording controller metrics.
type controllerMetricsRecorder interface {
	reconcileStart() func(error)
}

// controllerMetrics provides metrics for controller operations.
type controllerMetrics struct {
	controllerName         string
	reconciliationsTotal   metrics.Counter
	reconcileDuration      metrics.Histogram
	reconciliationsRunning metrics.Gauge
}

var _ controllerMetricsRecorder = &controllerMetrics{}

// newControllerMetricsRecorder creates a new ControllerMetrics instance.
func newControllerMetricsRecorder(controllerName string) controllerMetricsRecorder {
	if !metrics.Active() {
		return &nullControllerMetricsRecorder{}
	}

	m := &controllerMetrics{
		controllerName:         controllerName,
		reconciliationsTotal:   reconciliationsTotal,
		reconcileDuration:      reconcileDuration,
		reconciliationsRunning: reconciliationsRunning,
	}

	return m
}

// reconcileStart is called at the start of a controller reconciliation function
// to begin metrics collection and returns a function called at the end to
// complete metrics recording.
func (m *controllerMetrics) reconcileStart() func(error) {
	start := time.Now()

	m.reconciliationsRunning.Add(1,
		metrics.Label{Name: controllerNameLabel, Value: m.controllerName})

	return func(err error) {
		duration := time.Since(start)

		m.reconcileDuration.Observe(duration.Seconds(),
			metrics.Label{Name: controllerNameLabel, Value: m.controllerName})

		result := "success"
		if err != nil {
			result = "error"
		}

		m.reconciliationsTotal.Inc([]metrics.Label{
			{Name: controllerNameLabel, Value: m.controllerName},
			{Name: "result", Value: result},
		}...)

		m.reconciliationsRunning.Sub(1,
			metrics.Label{Name: controllerNameLabel, Value: m.controllerName})
	}
}

type nullControllerMetricsRecorder struct{}

func (m *nullControllerMetricsRecorder) reconcileStart() func(error) {
	return func(err error) {}
}

var _ controllerMetricsRecorder = &nullControllerMetricsRecorder{}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	reconciliationsTotal.Reset()
	reconciliationsRunning.Reset()
	reconcileDuration.Reset()
}
