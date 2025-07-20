package krtcollections

import (
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	collectionSubsystem           = "collection"
	collectionNameLabel           = "collection"
	gatewayResourceCollectionName = "gateway_resources"
)

var (
	transformsTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: collectionSubsystem,
			Name:      "transforms_total",
			Help:      "Total transforms",
		},
		[]string{collectionNameLabel, "result"},
	)
	transformDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       collectionSubsystem,
			Name:                            "transform_duration_seconds",
			Help:                            "Transform duration",
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{collectionNameLabel},
	)
	transformsRunning = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: collectionSubsystem,
			Name:      "transforms_running",
			Help:      "Number of transforms currently running",
		},
		[]string{collectionNameLabel},
	)
	collectionResources = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: collectionSubsystem,
			Name:      "resources",
			Help:      "Current number of resources managed by the collection",
		},
		[]string{collectionNameLabel, "name", "namespace", "resource"},
	)

	// Metric to track the number of gateway resources
	gatewayResourceCollection = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: collectionSubsystem,
			Name:      "gateway_resources",
			Help:      "Current number of gateway resources managed by the collection",
		},
		[]string{"namespace", "resource"},
	)
)

func gwResourceMetricEventHandler[T client.Object](o krt.Event[T], resourceName string) {
	switch o.Event {
	case controllers.EventAdd:
		gatewayResourceCollection.Add(1, GatewayResourceMetricLabels{
			Namespace: o.Latest().GetNamespace(),
			Resource:  resourceName,
		}.toMetricsLabels()...)

	case controllers.EventDelete:
		gatewayResourceCollection.Sub(1, GatewayResourceMetricLabels{
			Namespace: o.Latest().GetNamespace(),
			Resource:  resourceName,
		}.toMetricsLabels()...)
	}
}

type GatewayResourceMetricLabels struct {
	Namespace string
	Resource  string
}

func (r GatewayResourceMetricLabels) toMetricsLabels() []metrics.Label {
	return []metrics.Label{
		{Name: "namespace", Value: r.Namespace},
		{Name: "resource", Value: r.Resource},
	}
}

type CollectionResourcesMetricLabels struct {
	Name      string
	Namespace string
	Resource  string
}

// toMetricsLabels converts CollectionResourcesLabels to a slice of metrics.Labels.
func (r CollectionResourcesMetricLabels) toMetricsLabels(collection string) []metrics.Label {
	return []metrics.Label{
		{Name: collectionNameLabel, Value: collection},
		{Name: "name", Value: r.Name},
		{Name: "namespace", Value: r.Namespace},
		{Name: "resource", Value: r.Resource},
	}
}

// CollectionMetricsRecorder defines the interface for recording collection metrics.
type CollectionMetricsRecorder interface {
	TransformStart() func(error)
	SetResources(labels CollectionResourcesMetricLabels, count int)
	IncResources(labels CollectionResourcesMetricLabels)
	DecResources(labels CollectionResourcesMetricLabels)
}

// collectionMetrics records metrics for collection operations.
type collectionMetrics struct {
	collectionName    string
	transformsTotal   metrics.Counter
	transformDuration metrics.Histogram
	transformsRunning metrics.Gauge
	resources         metrics.Gauge
}

var _ CollectionMetricsRecorder = &collectionMetrics{}

// NewCollectionMetricsRecorder creates a new recorder for collection metrics.
func NewCollectionMetricsRecorder(collectionName string) CollectionMetricsRecorder {
	if !metrics.Active() {
		return &nullCollectionMetricsRecorder{}
	}

	m := &collectionMetrics{
		collectionName:    collectionName,
		transformsTotal:   transformsTotal,
		transformDuration: transformDuration,
		transformsRunning: transformsRunning,
		resources:         collectionResources,
	}

	return m
}

// TransformStart is called at the start of a transform function to begin metrics
// collection and returns a function called at the end to complete metrics recording.
func (m *collectionMetrics) TransformStart() func(error) {
	start := time.Now()

	m.transformsRunning.Add(1,
		metrics.Label{Name: collectionNameLabel, Value: m.collectionName})

	return func(err error) {
		duration := time.Since(start)

		m.transformDuration.Observe(duration.Seconds(),
			metrics.Label{Name: collectionNameLabel, Value: m.collectionName})

		result := "success"
		if err != nil {
			result = "error"
		}

		m.transformsTotal.Inc([]metrics.Label{
			{Name: collectionNameLabel, Value: m.collectionName},
			{Name: "result", Value: result},
		}...)

		m.transformsRunning.Sub(1,
			metrics.Label{Name: collectionNameLabel, Value: m.collectionName})
	}
}

// SetResources updates the resource count gauge.
func (m *collectionMetrics) SetResources(labels CollectionResourcesMetricLabels, count int) {
	m.resources.Set(float64(count), labels.toMetricsLabels(m.collectionName)...)
}

// IncResources increments the resource count gauge.
func (m *collectionMetrics) IncResources(labels CollectionResourcesMetricLabels) {
	m.resources.Add(1, labels.toMetricsLabels(m.collectionName)...)
}

// DecResources decrements the resource count gauge.
func (m *collectionMetrics) DecResources(labels CollectionResourcesMetricLabels) {
	m.resources.Sub(1, labels.toMetricsLabels(m.collectionName)...)
}

type nullCollectionMetricsRecorder struct{}

var _ CollectionMetricsRecorder = &nullCollectionMetricsRecorder{}

func (m *nullCollectionMetricsRecorder) TransformStart() func(error) {
	return func(err error) {}
}

func (m *nullCollectionMetricsRecorder) ResetResources(resource string) {}

func (m *nullCollectionMetricsRecorder) SetResources(labels CollectionResourcesMetricLabels, count int) {
}

func (m *nullCollectionMetricsRecorder) IncResources(labels CollectionResourcesMetricLabels) {}

func (m *nullCollectionMetricsRecorder) DecResources(labels CollectionResourcesMetricLabels) {}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	transformsTotal.Reset()
	transformDuration.Reset()
	transformsRunning.Reset()
	collectionResources.Reset()
}
