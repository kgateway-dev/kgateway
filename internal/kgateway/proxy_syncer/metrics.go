package proxy_syncer

import (
	"sync"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	statusSubsystem   = "status_syncer"
	syncerNameLabel   = "syncer"
	resourceTypeLabel = "resource_type"
	resourceSubsystem = "snaphot"
)

var (
	statusSyncsTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: statusSubsystem,
			Name:      "status_syncs_total",
			Help:      "Total status syncs",
		},
		[]string{syncerNameLabel, "result"},
	)
	statusSyncDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       statusSubsystem,
			Name:                            "status_sync_duration_seconds",
			Help:                            "Status sync duration",
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{syncerNameLabel},
	)
	statusSyncResources = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: statusSubsystem,
			Name:      "resources",
			Help:      "Current number of resources managed by the status syncer",
		},
		[]string{syncerNameLabel, "name", "namespace", "resource"},
	)
)

// StatusSyncResourcesMetricLabels defines the labels for the syncer resources metric.
type StatusSyncResourcesMetricLabels struct {
	Name      string
	Namespace string
	Resource  string
}

func (r StatusSyncResourcesMetricLabels) toMetricsLabels(syncer string) []metrics.Label {
	return []metrics.Label{
		{Name: syncerNameLabel, Value: syncer},
		{Name: "name", Value: r.Name},
		{Name: "namespace", Value: r.Namespace},
		{Name: "resource", Value: r.Resource},
	}
}

// statusSyncMetricsRecorder defines the interface for recording status syncer metrics.
type statusSyncMetricsRecorder interface {
	StatusSyncStart() func(error)
	ResetResources(resource string)
	SetResources(labels StatusSyncResourcesMetricLabels, count int)
	IncResources(labels StatusSyncResourcesMetricLabels)
	DecResources(labels StatusSyncResourcesMetricLabels)
}

// statusSyncMetrics records metrics for status syncer operations.
type statusSyncMetrics struct {
	syncerName         string
	statusSyncsTotal   metrics.Counter
	statusSyncDuration metrics.Histogram
	resources          metrics.Gauge
	resourceNames      map[string]map[string]map[string]struct{}
	resourcesLock      sync.Mutex
}

// NewStatusSyncMetricsRecorder creates a new recorder for status syncer metrics.
func NewStatusSyncMetricsRecorder(syncerName string) statusSyncMetricsRecorder {
	if !metrics.Active() {
		return &nullStatusSyncMetricsRecorder{}
	}

	m := &statusSyncMetrics{
		syncerName:         syncerName,
		statusSyncsTotal:   statusSyncsTotal,
		statusSyncDuration: statusSyncDuration,
		resources:          statusSyncResources,
		resourceNames:      make(map[string]map[string]map[string]struct{}),
		resourcesLock:      sync.Mutex{},
	}

	return m
}

type nullStatusSyncMetricsRecorder struct{}

func (m *nullStatusSyncMetricsRecorder) StatusSyncStart() func(error) {
	return func(err error) {}
}

func (m *nullStatusSyncMetricsRecorder) ResetResources(resource string) {}

func (m *nullStatusSyncMetricsRecorder) SetResources(labels StatusSyncResourcesMetricLabels, count int) {
}

func (m *nullStatusSyncMetricsRecorder) IncResources(labels StatusSyncResourcesMetricLabels) {}

func (m *nullStatusSyncMetricsRecorder) DecResources(labels StatusSyncResourcesMetricLabels) {}

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

// ResetResources resets the resource count gauge for a specified resource.
func (m *statusSyncMetrics) ResetResources(resource string) {
	m.resourcesLock.Lock()

	namespaces, exists := m.resourceNames[resource]
	if !exists {
		m.resourcesLock.Unlock()

		return
	}

	delete(m.resourceNames, resource)

	m.resourcesLock.Unlock()

	for namespace, names := range namespaces {
		for name := range names {
			m.resources.Set(0, []metrics.Label{
				{Name: syncerNameLabel, Value: m.syncerName},
				{Name: "name", Value: name},
				{Name: "namespace", Value: namespace},
				{Name: "resource", Value: resource},
			}...)
		}
	}
}

// updateResourceNames updates the internal map of resource names.
func (m *statusSyncMetrics) updateResourceNames(labels StatusSyncResourcesMetricLabels) {
	m.resourcesLock.Lock()

	if _, exists := m.resourceNames[labels.Resource]; !exists {
		m.resourceNames[labels.Resource] = make(map[string]map[string]struct{})
	}

	if _, exists := m.resourceNames[labels.Resource][labels.Namespace]; !exists {
		m.resourceNames[labels.Resource][labels.Namespace] = make(map[string]struct{})
	}

	m.resourceNames[labels.Resource][labels.Namespace][labels.Name] = struct{}{}

	m.resourcesLock.Unlock()
}

// SetResources updates the resource count gauge.
func (m *statusSyncMetrics) SetResources(labels StatusSyncResourcesMetricLabels, count int) {
	m.updateResourceNames(labels)

	m.resources.Set(float64(count), labels.toMetricsLabels(m.syncerName)...)
}

// IncResources increments the resource count gauge.
func (m *statusSyncMetrics) IncResources(labels StatusSyncResourcesMetricLabels) {
	m.updateResourceNames(labels)

	m.resources.Add(1, labels.toMetricsLabels(m.syncerName)...)
}

// DecResources decrements the resource count gauge.
func (m *statusSyncMetrics) DecResources(labels StatusSyncResourcesMetricLabels) {
	m.updateResourceNames(labels)

	m.resources.Sub(1, labels.toMetricsLabels(m.syncerName)...)
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	statusSyncDuration.Reset()
	statusSyncsTotal.Reset()
	statusSyncResources.Reset()
}
