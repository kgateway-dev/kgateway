package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

const (
	testTranslatorName string = "test-translator"
	testGatewayName    string = "test-gateway"
	testNamespace      string = "test-namespace"
)

func setupTest() {
	ResetMetrics()
}

func assertTranslationsRunning(currentMetrics metricstest.GatheredMetrics, translatorName string, count int) {
	currentMetrics.AssertMetric("kgateway_translator_translations_running", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "name", Value: testGatewayName},
			{Name: "namespace", Value: testNamespace},
			{Name: "translator", Value: translatorName},
		},
		Value: float64(count),
	})
}

func TestCollectTranslationMetrics_Success(t *testing.T) {
	setupTest()

	// Start translation
	finishFunc := CollectTranslationMetrics(TranslatorMetricLabels{
		Name:       testGatewayName,
		Namespace:  testNamespace,
		Translator: testTranslatorName,
	})

	// Check that the translations_running metric is 1
	currentMetrics := metricstest.MustGatherMetrics(t)
	assertTranslationsRunning(currentMetrics, testTranslatorName, 1)

	// Finish translation
	finishFunc(nil)
	currentMetrics = metricstest.MustGatherMetrics(t)

	// Check the translations_running metric
	assertTranslationsRunning(currentMetrics, testTranslatorName, 0)

	currentMetrics.AssertMetric("kgateway_translator_translations_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "name", Value: testGatewayName},
			{Name: "namespace", Value: testNamespace},
			{Name: "result", Value: "success"},
			{Name: "translator", Value: testTranslatorName},
		},
		Value: 1,
	})

	// Check the translation_duration_seconds metric
	currentMetrics.AssertMetricLabels("kgateway_translator_translation_duration_seconds", []metrics.Label{
		{Name: "name", Value: testGatewayName},
		{Name: "namespace", Value: testNamespace},
		{Name: "translator", Value: testTranslatorName},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_translator_translation_duration_seconds")
}

func TestCollectTranslationMetrics_Error(t *testing.T) {
	setupTest()

	finishFunc := CollectTranslationMetrics(TranslatorMetricLabels{
		Name:       testGatewayName,
		Namespace:  testNamespace,
		Translator: testTranslatorName,
	})

	currentMetrics := metricstest.MustGatherMetrics(t)
	assertTranslationsRunning(currentMetrics, testTranslatorName, 1)

	finishFunc(assert.AnError)
	currentMetrics = metricstest.MustGatherMetrics(t)
	assertTranslationsRunning(currentMetrics, testTranslatorName, 0)

	currentMetrics.AssertMetric(
		"kgateway_translator_translations_total",
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: testGatewayName},
				{Name: "namespace", Value: testNamespace},
				{Name: "result", Value: "error"},
				{Name: "translator", Value: testTranslatorName},
			},
			Value: 1,
		},
	)

	currentMetrics.AssertMetricLabels("kgateway_translator_translation_duration_seconds", []metrics.Label{
		{Name: "name", Value: testGatewayName},
		{Name: "namespace", Value: testNamespace},
		{Name: "translator", Value: testTranslatorName},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_translator_translation_duration_seconds")
}

func TestTranslationMetricsNotActive(t *testing.T) {
	metrics.SetActive(false)
	defer metrics.SetActive(true)

	setupTest()

	assert.False(t, metrics.Active())

	finishFunc := CollectTranslationMetrics(TranslatorMetricLabels{
		Name:       testGatewayName,
		Namespace:  testNamespace,
		Translator: testTranslatorName,
	})

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricNotExists("kgateway_translator_translations_running")

	finishFunc(nil)

	currentMetrics = metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricNotExists("kgateway_translator_translations_running")
	currentMetrics.AssertMetricNotExists("kgateway_translator_translations_total")
	currentMetrics.AssertMetricNotExists("kgateway_translator_translation_duration_seconds")
}
