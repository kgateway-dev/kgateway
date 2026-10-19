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

// TestClusterScopingMetricsReportWhatWasDroppedAndWhy: dropped_clusters shows
// what scoping saves, so a recorded count must reach the gateway's series.
func TestClusterScopingMetricsReportWhatWasDroppedAndWhy(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)

	recordClusterScopingDropped(publishGateTestClient, 17)

	gathered := metricstest.MustGatherMetrics(t)
	gathered.AssertMetric("kgateway_xds_cluster_scoping_dropped_clusters", &metricstest.ExpectedMetric{
		Labels: []metrics.Label{
			{Name: gatewayLabel, Value: "unknown"},
			{Name: namespaceLabel, Value: "unknown"},
		},
		Value: 17,
	})
}

// TestClusterScopingDisabledIsAZeroSeriesUntilItIsNot: the series exists at 0
// while healthy, so an alert can fire on the change to 1 instead of on a label
// appearing.
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

// TestDereferenceTransitionsAreCountedOncePerWindow: an open window is not
// re-counted on every publish, so the rate tracks removals, not publish
// frequency.
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
// never-sent clusters counts each once.
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

// Rebuilds must not recount open windows; deferred and coherent holds count
// alike.
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

// The unscoped warning fires on the transition, not on every client rebuild.
func TestClusterScopingDisabledIsNotedOnTransition(t *testing.T) {
	const gateway = "kube-gateway-api~ns-note~gw-note"
	assert.False(t, noteClusterScopingDisabled(gateway, false))
	assert.True(t, noteClusterScopingDisabled(gateway, true), "scoped to unscoped is a transition")
	assert.False(t, noteClusterScopingDisabled(gateway, true), "staying unscoped is not")
	assert.False(t, noteClusterScopingDisabled(gateway, false))
	assert.True(t, noteClusterScopingDisabled(gateway, true), "turning unscoped again is")
}

// Keep gateway gauges while clients remain; remove them after the last leaves
// so a deleted gateway cannot keep a stale disabled alert active.
func TestClusterScopingSeriesAreForgottenWithTheGatewaysLastClient(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	// series persist across tests in this package
	clusterScopingDroppedClusters.Reset()
	clusterScopingDisabledGateways.Reset()
	const (
		leaving = "kube-gateway-api~ns-forget~gw-forget"
		other   = "kube-gateway-api~ns-forget~gw-other"
	)
	for _, client := range []string{leaving, other} {
		recordClusterScopingDropped(client, 17)
		recordClusterScopingDisabled(client, true)
		noteClusterScopingDisabled(client, true)
	}
	stillThere := []ir.UniquelyConnectedClient{newTestUcc(leaving)}
	require.True(t, gatewayHasClients(leaving, stillThere), "another client of the gateway is still connected")
	require.False(t, gatewayHasClients(leaving, []ir.UniquelyConnectedClient{newTestUcc(other)}))

	forgetClusterScopingGateway(leaving)

	gathered := metricstest.MustGatherMetrics(t)
	for _, name := range []string{
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
