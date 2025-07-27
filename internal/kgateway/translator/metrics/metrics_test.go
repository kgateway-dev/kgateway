package metrics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	. "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

func setupTest() {
	ResetMetrics()
}

func TestNewTranslatorRecorder(t *testing.T) {
	setupTest()

	translatorName := "test-translator"
	m := NewTranslatorMetricsRecorder(translatorName)

	finishFunc := m.TranslationStart()
	finishFunc(nil)

	expectedMetrics := []string{
		"kgateway_translator_translations_total",
		"kgateway_translator_translation_duration_seconds",
		"kgateway_translator_translations_running",
	}

	currentMetrics := metricstest.MustGatherMetrics(t)
	for _, expected := range expectedMetrics {
		currentMetrics.AssertMetricExists(expected)
	}
}

func assertTranslationsRunning(currentMetrics metricstest.GatheredMetrics, translatorName string, count int) {
	currentMetrics.AssertMetric("kgateway_translator_translations_running", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "translator", Value: translatorName},
		},
		Value: float64(count),
	})
}

func TestTranslationStart_Success(t *testing.T) {
	setupTest()

	m := NewTranslatorMetricsRecorder("test-translator")

	// Start translation
	finishFunc := m.TranslationStart()
	time.Sleep(10 * time.Millisecond)

	// Check that the translations_running metric is 1
	currentMetrics := metricstest.MustGatherMetrics(t)
	assertTranslationsRunning(currentMetrics, "test-translator", 1)

	// Finish translation
	finishFunc(nil)
	time.Sleep(10 * time.Millisecond)
	currentMetrics = metricstest.MustGatherMetrics(t)

	// Check the translations_running metric
	assertTranslationsRunning(currentMetrics, "test-translator", 0)

	currentMetrics.AssertMetric("kgateway_translator_translations_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "result", Value: "success"},
			{Name: "translator", Value: "test-translator"},
		},
		Value: 1,
	})

	// Check the translation_duration_seconds metric
	currentMetrics.AssertMetricLabels("kgateway_translator_translation_duration_seconds", []metrics.Label{
		{Name: "translator", Value: "test-translator"},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_translator_translation_duration_seconds")
}

func TestTranslationStart_Error(t *testing.T) {
	setupTest()

	m := NewTranslatorMetricsRecorder("test-translator")

	finishFunc := m.TranslationStart()
	currentMetrics := metricstest.MustGatherMetrics(t)
	assertTranslationsRunning(currentMetrics, "test-translator", 1)

	finishFunc(assert.AnError)
	currentMetrics = metricstest.MustGatherMetrics(t)
	assertTranslationsRunning(currentMetrics, "test-translator", 0)

	currentMetrics.AssertMetric(
		"kgateway_translator_translations_total",
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "result", Value: "error"},
				{Name: "translator", Value: "test-translator"},
			},
			Value: 1,
		},
	)
	currentMetrics.AssertHistogramPopulated("kgateway_translator_translation_duration_seconds")
}

func TestResourceSync(t *testing.T) {
	setupTest()

	details := ResourceSyncDetails{
		Gateway:      "test-gateway",
		Namespace:    "test-namespace",
		ResourceType: "test",
		ResourceName: "test-resource",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	StartResourceSyncMetricsProcessing(ctx)

	// Test for status sync metrics.
	resourcesStatusSyncsCompletedTotal := metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: "resources",
			Name:      "status_syncs_completed_total",
			Help:      "Total number of status syncs completed for resources",
		},
		[]string{"gateway", "namespace", "resource"})
	resourcesStatusSyncDuration := metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       "resources",
			Name:                            "status_sync_duration_seconds",
			Help:                            "Initial resource update until status sync duration",
			Buckets:                         metrics.DefaultBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{"gateway", "namespace", "resource"},
	)

	StartResourceSync(details.ResourceName, ResourceMetricLabels{
		Gateway:   details.Gateway,
		Namespace: details.Namespace,
		Resource:  details.ResourceType,
	})

	IncXDSSnapshotSync(details.Gateway, details.Namespace)

	EndResourceSync(details, false, resourcesStatusSyncsCompletedTotal, resourcesStatusSyncDuration)

	time.Sleep(50 * time.Millisecond) // Allow some time for metrics to be processed.

	gathered := metricstest.MustGatherMetrics(t)

	gathered.AssertMetric("kgateway_resources_syncs_started_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: details.Gateway},
			{Name: "namespace", Value: details.Namespace},
			{Name: "resource", Value: details.ResourceType},
		},
		Value: 1,
	})

	gathered.AssertMetric("kgateway_resources_status_syncs_completed_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: details.Gateway},
			{Name: "namespace", Value: details.Namespace},
			{Name: "resource", Value: details.ResourceType},
		},
		Value: 1,
	})

	gathered.AssertMetricsLabels("kgateway_resources_status_sync_duration_seconds", [][]metrics.Label{{
		{Name: "gateway", Value: details.Gateway},
		{Name: "namespace", Value: details.Namespace},
		{Name: "resource", Value: details.ResourceType},
	}})
	gathered.AssertHistogramPopulated("kgateway_resources_status_sync_duration_seconds")

	// Test for XDS snapshot sync metrics.
	resourcesXDSSyncsTotal := metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: "resources",
			Name:      "xds_snapshot_syncs_total",
			Help:      "Total number of XDS snapshot syncs for resources",
		},
		[]string{"gateway", "namespace", "resource"})
	resourcesXDSyncDuration := metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       "resources",
			Name:                            "xds_snapshot_sync_duration_seconds",
			Help:                            "Initial resource update until XDS snapshot sync duration",
			Buckets:                         metrics.DefaultBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{"gateway", "namespace", "resource"},
	)

	EndResourceSync(details, true, resourcesXDSSyncsTotal, resourcesXDSyncDuration)

	time.Sleep(50 * time.Millisecond) // Allow some time for metrics to be processed.

	gathered = metricstest.MustGatherMetrics(t)

	gathered.AssertMetric("kgateway_resources_syncs_started_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: details.Gateway},
			{Name: "namespace", Value: details.Namespace},
			{Name: "resource", Value: details.ResourceType},
		},
		Value: 1,
	})

	gathered.AssertMetric("kgateway_xds_snapshot_syncs_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: details.Gateway},
			{Name: "namespace", Value: details.Namespace},
		},
		Value: 1,
	})

	gathered.AssertMetric("kgateway_resources_xds_snapshot_syncs_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: details.Gateway},
			{Name: "namespace", Value: details.Namespace},
			{Name: "resource", Value: details.ResourceType},
		},
		Value: 1,
	})

	gathered.AssertMetricsLabels("kgateway_resources_xds_snapshot_sync_duration_seconds", [][]metrics.Label{{
		{Name: "gateway", Value: details.Gateway},
		{Name: "namespace", Value: details.Namespace},
		{Name: "resource", Value: details.ResourceType},
	}})
	gathered.AssertHistogramPopulated("kgateway_resources_xds_snapshot_sync_duration_seconds")
}

func TestSyncChannelFull(t *testing.T) {
	setupTest()

	m := NewTranslatorMetricsRecorder("test-translator")

	// Start translation
	m.TranslationStart()

	details := ResourceSyncDetails{
		Gateway:      "test-gateway",
		Namespace:    "test-namespace",
		ResourceType: "test",
		ResourceName: "test-resource",
	}

	resourcesXDSSyncsCompletedTotal := metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: "resources",
			Name:      "xds_snapshot_syncs_channel_full_total",
			Help:      "Total number of XDS snapshot syncs",
		},
		[]string{"gateway", "namespace", "resource"})

	resourcesXDSyncDuration := metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem: "resources",
			Name:      "xds_snapshot_sync_duration_channel_full",
			Help:      "XDS snapshot sync duration",
		},
		[]string{"gateway", "namespace", "resource"},
	)

	for i := 0; i < 1024; i++ {
		success := EndResourceSync(details, false, resourcesXDSSyncsCompletedTotal, resourcesXDSyncDuration)
		assert.True(t, success)
	}

	// Channel will be full. Validate that EndResourceSync returns and logs an error and that the kgateway_resources_updates_dropped_total metric is incremented.
	c := make(chan struct{})
	defer close(c)

	overflowCount := 0
	numOverflows := 20

	for overflowCount < numOverflows {
		go func() {
			success := EndResourceSync(details, false, resourcesXDSSyncsCompletedTotal, resourcesXDSyncDuration)
			assert.False(t, success)
			c <- struct{}{}
		}()

		select {
		case <-c: // Expect to return quickly
		case <-time.After(10 * time.Millisecond):
			t.Fatal("Expected EndResourceSync to return and log an error")
		}

		overflowCount++

		currentMetrics := metricstest.MustGatherMetrics(t)
		currentMetrics.AssertMetric("kgateway_resources_updates_dropped_total", &metricstest.ExpectedMetric{
			Labels: []metrics.Label{},
			Value:  float64(overflowCount),
		})
	}
}

func TestTranslationMetricsNotActive(t *testing.T) {
	metrics.SetActive(false)
	defer metrics.SetActive(true)

	setupTest()

	assert.False(t, metrics.Active())

	m := NewTranslatorMetricsRecorder("test-translator")

	// Start translation
	finishFunc := m.TranslationStart()
	time.Sleep(10 * time.Millisecond)

	// Check that the translations_running metric is 1
	currentMetrics := metricstest.MustGatherMetrics(t)
	currentMetrics.AssertMetricNotExists("kgateway_translator_translations_running")

	// Finish translation
	finishFunc(nil)
	time.Sleep(10 * time.Millisecond)
	currentMetrics = metricstest.MustGatherMetrics(t)

	// Check the translations_running metric
	currentMetrics.AssertMetricNotExists("kgateway_translator_translations_running")

	// Check the translations_total metric
	currentMetrics.AssertMetricNotExists("kgateway_translator_translations_total")

	// Check the translation_duration_seconds metric
	currentMetrics.AssertMetricNotExists("kgateway_translator_translation_duration_seconds")
}
