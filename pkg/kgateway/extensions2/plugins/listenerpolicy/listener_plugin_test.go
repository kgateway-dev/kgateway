package listenerpolicy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	translatorMetrics "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestCollectTranslationMetrics_ListenerPolicy(t *testing.T) {
	translatorMetrics.ResetMetrics()
	t.Cleanup(translatorMetrics.ResetMetrics)

	spec := &kgateway.ListenerPolicySpec{}

	objSrc := ir.ObjectSource{
		Group:     "gateway.kgateway.dev",
		Kind:      "ListenerPolicy",
		Namespace: "test-namespace",
		Name:      "test-listener-policy",
	}

	policyIR, errs := NewListenerPolicyIR(nil, nil, time.Now(), spec, objSrc)
	require.Empty(t, errs, "translation should not produce errors")
	assert.NotNil(t, policyIR, "translation should return a non-nil IR")

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricsInclude("kgateway_translator_translations_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: "test-listener-policy"},
				{Name: "namespace", Value: "test-namespace"},
				{Name: "result", Value: "success"},
				{Name: "translator", Value: "ListenerPolicy"},
			},
			Value: 1,
		},
	})
}

func TestCollectTranslationMetrics_HTTPListenerPolicy(t *testing.T) {
	translatorMetrics.ResetMetrics()
	t.Cleanup(translatorMetrics.ResetMetrics)

	spec := &kgateway.ListenerPolicySpec{}

	objSrc := ir.ObjectSource{
		Group:     "gateway.kgateway.dev",
		Kind:      "HTTPListenerPolicy",
		Namespace: "test-namespace",
		Name:      "test-httplistener-policy",
	}

	policyIR, errs := NewListenerPolicyIR(nil, nil, time.Now(), spec, objSrc)
	require.Empty(t, errs, "translation should not produce errors")
	assert.NotNil(t, policyIR, "translation should return a non-nil IR")

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricsInclude("kgateway_translator_translations_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: "test-httplistener-policy"},
				{Name: "namespace", Value: "test-namespace"},
				{Name: "result", Value: "success"},
				{Name: "translator", Value: "HTTPListenerPolicy"},
			},
			Value: 1,
		},
	})
}
