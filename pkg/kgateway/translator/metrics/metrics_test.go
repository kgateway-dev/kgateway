package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/metrics"
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

func TestCollectTranslationMetrics_Success(t *testing.T) {
	setupTest()

	// Start translation
	finishFunc := CollectTranslationMetrics(TranslatorMetricLabels{
		Name:       testGatewayName,
		Namespace:  testNamespace,
		Translator: testTranslatorName,
	})

	// Finish translation
	finishFunc(nil)
	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricsInclude("kgateway_translator_translations_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: testGatewayName},
				{Name: "namespace", Value: testNamespace},
				{Name: "result", Value: "success"},
				{Name: "translator", Value: testTranslatorName},
			},
			Value: 1,
		},
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

	finishFunc(assert.AnError)
	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricsInclude("kgateway_translator_translations_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: testGatewayName},
				{Name: "namespace", Value: testNamespace},
				{Name: "result", Value: "error"},
				{Name: "translator", Value: testTranslatorName},
			},
			Value: 1,
		},
	})

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

	finishFunc(nil)

	currentMetrics := metricstest.MustGatherMetrics(t)

	// Counter exists after Reset() but should have value 0 since no translations were recorded
	currentMetrics.AssertMetric("kgateway_translator_translations_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "name", Value: ""},
			{Name: "namespace", Value: ""},
			{Name: "result", Value: ""},
			{Name: "translator", Value: ""},
		},
		Value: 0.0,
	})
	currentMetrics.AssertMetricNotExists("kgateway_translator_translation_duration_seconds")
}
