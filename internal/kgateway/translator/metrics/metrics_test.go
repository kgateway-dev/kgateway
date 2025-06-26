package metrics_test

import (
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
