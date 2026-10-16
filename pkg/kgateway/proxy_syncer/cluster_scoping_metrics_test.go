package proxy_syncer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
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

// TestReferenceAheadTransitionsAreCountedOncePerCluster: a retarget onto
// clusters the proxy has never been sent counts each such cluster once, the
// way the de-reference transitions count clusters, and does not count the
// publish that releases the hold.
func TestReferenceAheadTransitionsAreCountedOncePerCluster(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	clusterScopingTransitionsTotal.Reset() // series persist across tests in this package

	pt := newReferenceAheadTranslator(t, time.Hour)
	ctx := t.Context()

	pt.syncXds(ctx, routedClusterWrapperV("v1", "old"))
	pt.syncXds(ctx, routedClusterWrapperV("v2", "old", "new-a", "new-b"))

	held := metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels("kgateway_xds_cluster_scoping_transitions_total", []metrics.Label{
		{Name: gatewayLabel, Value: "unknown"},
		{Name: namespaceLabel, Value: "unknown"},
		{Name: transitionLabel, Value: transitionReferenceAheadHeld},
	})
	assert.Equal(t, float64(2), held, "two never-sent clusters, one held update: count the clusters, once each")
}

// A cluster counts once per window, not once per build that the window spans,
// and a hold the deferred path applies counts like a coherent one.
func TestReferenceAheadTransitionsCountWindowsNotBuilds(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	clusterScopingTransitionsTotal.Reset() // series persist across tests in this package
	held := func() float64 {
		t.Helper()
		return metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels("kgateway_xds_cluster_scoping_transitions_total", []metrics.Label{
			{Name: gatewayLabel, Value: "unknown"},
			{Name: namespaceLabel, Value: "unknown"},
			{Name: transitionLabel, Value: transitionReferenceAheadHeld},
		})
	}

	pt := newReferenceAheadTranslator(t, time.Hour)
	ctx := t.Context()
	pt.syncXds(ctx, routedBuild(t, "v1", "old", "old"))
	pt.syncXds(ctx, routedBuild(t, "v2", "new", "old", "new"))
	pt.syncXds(ctx, routedBuild(t, "v3", "new", "old", "new")) // a rebuild inside the window
	assert.Equal(t, float64(1), held(), "one cluster, one window")

	deferred := routedBuild(t, "v4", "new", "old", "new", "authz")
	deferred.missingEndpointsReferenced = []string{"old"}
	deferred.synthesizedEndpoints = []string{"old"}
	pt.syncXds(ctx, deferred)
	assert.Equal(t, float64(2), held(), "the deferred path's hold counts its new cluster")
}

// The unscoped warning fires when a gateway turns unscoped, not on every
// rebuild of each of its clients while it stays that way.
func TestClusterScopingDisabledIsNotedOnTransition(t *testing.T) {
	const gateway = "kube-gateway-api~ns-note~gw-note"
	assert.False(t, noteClusterScopingDisabled(gateway, false))
	assert.True(t, noteClusterScopingDisabled(gateway, true), "scoped to unscoped is a transition")
	assert.False(t, noteClusterScopingDisabled(gateway, true), "staying unscoped is not")
	assert.False(t, noteClusterScopingDisabled(gateway, false))
	assert.True(t, noteClusterScopingDisabled(gateway, true), "turning unscoped again is")
}

// A gateway's scoping gauges are dropped when its last client departs, so a
// deleted gateway does not keep exporting its last values (a disabled gauge of
// 1 would keep an alert firing), and are kept while any client remains.
func TestClusterScopingSeriesAreForgottenWithTheGatewaysLastClient(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	// series persist across tests in this package
	clusterScopingEmittedClusters.Reset()
	clusterScopingDroppedClusters.Reset()
	clusterScopingDisabledGateways.Reset()
	const (
		leaving = "kube-gateway-api~ns-forget~gw-forget"
		other   = "kube-gateway-api~ns-forget~gw-other"
	)
	for _, client := range []string{leaving, other} {
		recordClusterScopingEmission(client, 3, 17)
		recordClusterScopingDisabled(client, true)
		noteClusterScopingDisabled(client, true)
	}
	stillThere := []ir.UniquelyConnectedClient{newTestUcc(leaving)}
	require.True(t, gatewayHasClients(leaving, stillThere), "another client of the gateway is still connected")
	require.False(t, gatewayHasClients(leaving, []ir.UniquelyConnectedClient{newTestUcc(other)}))

	forgetClusterScopingGateway(leaving)

	gathered := metricstest.MustGatherMetrics(t)
	for _, name := range []string{
		"kgateway_xds_cluster_scoping_emitted_clusters",
		"kgateway_xds_cluster_scoping_dropped_clusters",
		"kgateway_xds_cluster_scoping_disabled_gateways",
	} {
		gathered.AssertMetricLabels(name, []metrics.Label{
			{Name: gatewayLabel, Value: "gw-other"},
			{Name: namespaceLabel, Value: "ns-forget"},
		})
	}
	assert.True(t, noteClusterScopingDisabled(leaving, true), "a returning gateway that is unscoped warns again")
}
