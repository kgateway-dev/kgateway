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

func assertTranslationsRunning(currentMetrics metricstest.GatheredMetrics, count int) {
	currentMetrics.AssertMetric("kgateway_translator_translations_running", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "name", Value: testGatewayName},
			{Name: "namespace", Value: testNamespace},
			{Name: "translator", Value: testTranslatorName},
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
	assertTranslationsRunning(currentMetrics, 1)

	// Finish translation
	finishFunc(nil)
	currentMetrics = metricstest.MustGatherMetrics(t)

	// Check the translations_running metric
	assertTranslationsRunning(currentMetrics, 0)

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

	currentMetrics := metricstest.MustGatherMetrics(t)
	assertTranslationsRunning(currentMetrics, 1)

	finishFunc(assert.AnError)
	currentMetrics = metricstest.MustGatherMetrics(t)
	assertTranslationsRunning(currentMetrics, 0)

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

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricNotExists("kgateway_translator_translations_running")

	finishFunc(nil)

	currentMetrics = metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricNotExists("kgateway_translator_translations_running")
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

func TestCollectPolicyTranslationMetrics_Success(t *testing.T) {
	setupTest()

	// Translate policy
	finishFunc := CollectPolicyTranslationMetrics("test-policy", "test-namespace", "BackendConfigPolicy")
	finishFunc(nil)

	currentMetrics := metricstest.MustGatherMetrics(t)

	// Check the translations_total metric for a policy translation, including name and namespace labels
	currentMetrics.AssertMetricsInclude("kgateway_translator_translations_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: "test-policy"},
				{Name: "namespace", Value: "test-namespace"},
				{Name: "result", Value: "success"},
				{Name: "translator", Value: "BackendConfigPolicy"},
			},
			Value: 1,
		},
	})

	// Check the translation_duration_seconds metric
	currentMetrics.AssertMetricLabels("kgateway_translator_translation_duration_seconds", []metrics.Label{
		{Name: "name", Value: "test-policy"},
		{Name: "namespace", Value: "test-namespace"},
		{Name: "translator", Value: "BackendConfigPolicy"},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_translator_translation_duration_seconds")
}

func TestCollectPolicyTranslationMetrics_Error(t *testing.T) {
	setupTest()

	// Translate policy with error
	finishFunc := CollectPolicyTranslationMetrics("test-policy", "test-namespace", "TrafficPolicy")
	finishFunc(assert.AnError)

	currentMetrics := metricstest.MustGatherMetrics(t)

	// Check the translations_total metric with error result
	currentMetrics.AssertMetricsInclude("kgateway_translator_translations_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: "test-policy"},
				{Name: "namespace", Value: "test-namespace"},
				{Name: "result", Value: "error"},
				{Name: "translator", Value: "TrafficPolicy"},
			},
			Value: 1,
		},
	})

	currentMetrics.AssertHistogramPopulated("kgateway_translator_translation_duration_seconds")
}

func TestCollectPolicyTranslationMetrics_Multiple(t *testing.T) {
	setupTest()

	// Translate multiple policies
	policies := []string{"BackendConfigPolicy", "TrafficPolicy", "ListenerPolicy"}
	for _, policy := range policies {
		finishFunc := CollectPolicyTranslationMetrics("test-policy", "test-namespace", policy)
		finishFunc(nil)
	}

	currentMetrics := metricstest.MustGatherMetrics(t)

	// Check that each policy has metrics
	for _, policy := range policies {
		currentMetrics.AssertMetricsInclude("kgateway_translator_translations_total", []metricstest.ExpectMetric{
			&metricstest.ExpectedMetric{
				Labels: []metrics.Label{
					{Name: "name", Value: "test-policy"},
					{Name: "namespace", Value: "test-namespace"},
					{Name: "result", Value: "success"},
					{Name: "translator", Value: policy},
				},
				Value: 1,
			},
		})
	}
}

func TestCollectPolicyTranslationMetrics_NotActive(t *testing.T) {
	metrics.SetActive(false)
	defer metrics.SetActive(true)

	setupTest()

	assert.False(t, metrics.Active())

	finishFunc := CollectPolicyTranslationMetrics("test-policy", "test-namespace", "BackendConfigPolicy")
	finishFunc(nil)

	currentMetrics := metricstest.MustGatherMetrics(t)

	// Counter exists after Reset() but should have value 0 since metrics are not active
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
