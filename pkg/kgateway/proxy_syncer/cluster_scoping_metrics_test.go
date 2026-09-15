package proxy_syncer

import (
	"testing"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

// TestClusterScopingMetricsReportWhatWasDroppedAndWhy: the emitted/dropped pair
// is how an operator sees what the feature bought them, so both series must
// exist for a scoped client even when nothing was dropped.
func TestClusterScopingMetricsReportWhatWasDroppedAndWhy(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)

	recordClusterScopingEmission(publishGateTestClient, 3, 17)

	gathered := metricstest.MustGatherMetrics(t)
	gathered.AssertMetric("kgateway_xds_cluster_scoping_emitted_clusters", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: gatewayLabel, Value: "unknown"},
			{Name: namespaceLabel, Value: "unknown"},
		},
		Value: 3,
	})
	gathered.AssertMetric("kgateway_xds_cluster_scoping_dropped_clusters", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: gatewayLabel, Value: "unknown"},
			{Name: namespaceLabel, Value: "unknown"},
		},
		Value: 17,
	})
}

// TestClusterScopingDisabledIsAZeroSeriesUntilItIsNot: an alert should fire on
// the value going to 1, which requires the series to exist while the gateway is
// healthy. Recording only the disabled case would make the alert depend on a
// label appearing, which is far harder to write correctly.
func TestClusterScopingDisabledIsAZeroSeriesUntilItIsNot(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)

	recordClusterScopingDisabled(publishGateTestClient, false)
	metricstest.MustGatherMetrics(t).AssertMetric(
		"kgateway_xds_cluster_scoping_disabled_gateways", &metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: gatewayLabel, Value: "unknown"},
				{Name: namespaceLabel, Value: "unknown"},
			},
			Value: 0,
		})

	recordClusterScopingDisabled(publishGateTestClient, true)
	metricstest.MustGatherMetrics(t).AssertMetric(
		"kgateway_xds_cluster_scoping_disabled_gateways", &metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: gatewayLabel, Value: "unknown"},
				{Name: namespaceLabel, Value: "unknown"},
			},
			Value: 1,
		})
}

// TestDereferenceTransitionsAreCountedOncePerWindow: the counter measures
// de-reference events, not publishes. A cluster sitting in an open window must
// not be re-counted every time the client publishes, or the rate would track
// churn rather than removals.
func TestDereferenceTransitionsAreCountedOncePerWindow(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)

	pt := newGraceTestTranslator(t, time.Hour)
	ctx := t.Context()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going-away"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))
	pt.syncXds(ctx, clusterWrapperV("v3", "kept"))
	pt.syncXds(ctx, clusterWrapperV("v4", "kept"))

	gathered := metricstest.MustGatherMetrics(t)
	gathered.AssertMetric("kgateway_xds_cluster_scoping_transitions_total", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: gatewayLabel, Value: "unknown"},
			{Name: namespaceLabel, Value: "unknown"},
			{Name: transitionLabel, Value: transitionDereferenceGraced},
		},
		Value: 1,
	})
}
