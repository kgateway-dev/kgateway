package proxy_syncer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	kmetrics "github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

const (
	testSyncerName  = "test-syncer"
	testGatewayName = "test-gateway"
	testNamespace   = "test-namespace"
)

func setupTest() {
	ResetMetrics()
	kmetrics.ResetMetrics()
}

func TestCollectStatusSyncMetrics_Success(t *testing.T) {
	setupTest()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	finishFunc := collectStatusSyncMetrics(statusSyncMetricLabels{
		Name:      testGatewayName,
		Namespace: testNamespace,
		Syncer:    testSyncerName,
	})
	finishFunc(nil)

	currentMetrics := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_status_syncer_status_syncs_total",
		"kgateway_status_syncer_status_sync_duration_seconds",
	)

	currentMetrics.AssertMetric("kgateway_status_syncer_status_syncs_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "name", Value: testGatewayName},
			{Name: "namespace", Value: testNamespace},
			{Name: "result", Value: "success"},
			{Name: "syncer", Value: "test-syncer"},
		},
		Value: 1,
	})

	currentMetrics.AssertMetricLabels("kgateway_status_syncer_status_sync_duration_seconds", []metrics.Label{
		{Name: "name", Value: testGatewayName},
		{Name: "namespace", Value: testNamespace},
		{Name: "syncer", Value: "test-syncer"},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_status_syncer_status_sync_duration_seconds")
}

func TestCollectStatusSyncMetrics_Error(t *testing.T) {
	setupTest()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	finishFunc := collectStatusSyncMetrics(statusSyncMetricLabels{
		Name:      testGatewayName,
		Namespace: testNamespace,
		Syncer:    testSyncerName,
	})
	finishFunc(assert.AnError)

	currentMetrics := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_status_syncer_status_syncs_total",
		"kgateway_status_syncer_status_sync_duration_seconds",
	)

	currentMetrics.AssertMetric("kgateway_status_syncer_status_syncs_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "name", Value: testGatewayName},
			{Name: "namespace", Value: testNamespace},
			{Name: "result", Value: "error"},
			{Name: "syncer", Value: "test-syncer"},
		},
		Value: 1,
	})

	currentMetrics.AssertMetricLabels("kgateway_status_syncer_status_sync_duration_seconds", []metrics.Label{
		{Name: "name", Value: testGatewayName},
		{Name: "namespace", Value: testNamespace},
		{Name: "syncer", Value: "test-syncer"},
	})
	currentMetrics.AssertHistogramPopulated("kgateway_status_syncer_status_sync_duration_seconds")
}

func TestResourceSyncMetrics(t *testing.T) {
	setupTest()

	testNS := "test-namespace"
	testName := "test-name"
	testResource := "test-resource"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kmetrics.StartResourceSyncMetricsProcessing(ctx)

	kmetrics.StartResourceStatusSync(kmetrics.ResourceSyncDetails{
		Gateway:      testName,
		Namespace:    testNS,
		ResourceType: testResource,
		ResourceName: testName,
	})

	kmetrics.EndResourceStatusSync(kmetrics.ResourceSyncDetails{
		Gateway:      testName,
		Namespace:    testNS,
		ResourceType: testResource,
		ResourceName: testName,
	})

	gathered := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_resources_status_syncs_started_total",
		"kgateway_resources_status_syncs_completed_total",
		"kgateway_resources_status_sync_duration_seconds",
	)

	gathered.AssertMetric("kgateway_resources_status_syncs_started_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: testName},
			{Name: "namespace", Value: testNS},
			{Name: "resource", Value: testResource},
		},
		Value: 1,
	})

	gathered.AssertMetric("kgateway_resources_status_syncs_completed_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: "gateway", Value: testName},
			{Name: "namespace", Value: testNS},
			{Name: "resource", Value: testResource},
		},
		Value: 1,
	})

	gathered.AssertMetricsLabels("kgateway_resources_status_sync_duration_seconds", [][]metrics.Label{{
		{Name: "gateway", Value: testName},
		{Name: "namespace", Value: testNS},
		{Name: "resource", Value: testResource},
	}})
	gathered.AssertHistogramPopulated("kgateway_resources_status_sync_duration_seconds")
}

func TestGetDetailsFromXDSClientResourceName(t *testing.T) {
	testCases := []struct {
		name     string
		resource string
		expected struct {
			role      string
			gateway   string
			namespace string
		}
	}{
		{
			name:     "Valid resource name",
			resource: "kgateway-kube-gateway-api~ns~test",
			expected: struct {
				role      string
				gateway   string
				namespace string
			}{
				role:      "kgateway-kube-gateway-api",
				gateway:   "test",
				namespace: "ns",
			},
		},
		{
			name:     "Invalid resource name",
			resource: "invalid-resource-name",
			expected: struct {
				role      string
				gateway   string
				namespace string
			}{
				role:      "unknown",
				gateway:   "unknown",
				namespace: "unknown",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cd := getDetailsFromXDSClientResourceName(tc.resource)
			assert.Equal(t, tc.expected.gateway, cd.Gateway)
			assert.Equal(t, tc.expected.namespace, cd.Namespace)
		})
	}
}
