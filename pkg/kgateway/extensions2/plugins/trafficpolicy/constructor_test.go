package trafficpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	translatorMetrics "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

func TestCollectTranslationMetrics_TrafficPolicy(t *testing.T) {
	translatorMetrics.ResetMetrics()

	policy := &kgateway.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-traffic-policy",
			Namespace: "test-namespace",
		},
		Spec: kgateway.TrafficPolicySpec{},
	}

	constructor := &TrafficPolicyConstructor{
		commoncol: &collections.CommonCollections{},
	}

	policyIR, errs := constructor.ConstructIR(nil, policy)
	require.Empty(t, errs, "translation should not produce errors")
	assert.NotNil(t, policyIR, "translation should return a non-nil IR")

	currentMetrics := metricstest.MustGatherMetrics(t)

	currentMetrics.AssertMetricsInclude("kgateway_translator_translations_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: "test-traffic-policy"},
				{Name: "namespace", Value: "test-namespace"},
				{Name: "result", Value: "success"},
				{Name: "translator", Value: "TrafficPolicy"},
			},
			Value: 1,
		},
	})
}
