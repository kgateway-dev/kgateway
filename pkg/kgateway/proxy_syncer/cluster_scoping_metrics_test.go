package proxy_syncer

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestClusterScopingMetricsReportWhatWasDroppedAndWhy: dropped_clusters shows
// what scoping saves, so a recorded count must reach the gateway's series.
func TestClusterScopingMetricsReportWhatWasDroppedAndWhy(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	t.Cleanup(func() { forgetClusterScopingClient(publishGateTestClient, false) })

	recordClusterScopingDropped(publishGateTestClient, 17)

	// Other tests' series persist in this package; read only this gateway's.
	assert.Equal(t, float64(17), metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels("kgateway_xds_cluster_scoping_dropped_clusters", []metrics.Label{
		{Name: gatewayLabel, Value: "unknown"},
		{Name: namespaceLabel, Value: "unknown"},
	}))
}

// TestClusterScopingDroppedIsTheMostAnyConnectedGroupDrops: one group falling
// back to every cluster, or leaving, must not hide what another group saves.
func TestClusterScopingDroppedIsTheMostAnyConnectedGroupDrops(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	const (
		zoneA = "kube-gateway-api~ns-dropped~gw-dropped~zone-a"
		zoneB = "kube-gateway-api~ns-dropped~gw-dropped~zone-b"
	)
	t.Cleanup(func() { forgetClusterScopingClient(zoneA, false) })
	dropped := func() float64 {
		t.Helper()
		return metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels("kgateway_xds_cluster_scoping_dropped_clusters", []metrics.Label{
			{Name: gatewayLabel, Value: "gw-dropped"},
			{Name: namespaceLabel, Value: "ns-dropped"},
		})
	}

	recordClusterScopingDropped(zoneA, 17)
	recordClusterScopingDropped(zoneB, 0) // zone b reverted to every cluster
	assert.Equal(t, float64(17), dropped(), "zone b's build must not overwrite what zone a saves")
	recordClusterScopingDropped(zoneA, 5)
	assert.Equal(t, float64(5), dropped(), "the gauge follows the groups' current builds")
	recordClusterScopingDropped(zoneB, 9)
	forgetClusterScopingClient(zoneB, true)
	assert.Equal(t, float64(5), dropped(), "a departing group takes its count with it")
}

// TestClusterScopingDisabledIsAZeroSeriesUntilItIsNot: the series exists at 0
// while healthy, so an alert can fire on the change to 1 instead of on a label
// appearing.
func TestClusterScopingDisabledIsAZeroSeriesUntilItIsNot(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	const client = "kube-gateway-api~ns-zero~gw-zero"
	t.Cleanup(func() { forgetClusterScopingClient(client, false) })
	disabled := func() float64 {
		t.Helper()
		return metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels("kgateway_xds_cluster_scoping_disabled_gateways", []metrics.Label{
			{Name: gatewayLabel, Value: "gw-zero"},
			{Name: namespaceLabel, Value: "ns-zero"},
		})
	}

	reportClusterScopingDisabled(discardLogger(), client, nil)
	assert.Equal(t, float64(0), disabled())

	reportClusterScopingDisabled(discardLogger(), client, []string{`cluster_header "x-target"`})
	assert.Equal(t, float64(1), disabled())
}

// TestDereferenceTransitionsAreCountedOncePerWindow: an open window is not
// re-counted on every publish, so the rate tracks removals, not publish
// frequency.
func TestDereferenceTransitionsAreCountedOncePerWindow(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	clusterScopingTransitionsTotal.Reset() // series persist across tests in this package

	pt := newGraceTestTranslator(t, time.Hour)
	ctx := t.Context()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going-away"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))
	pt.syncXds(ctx, clusterWrapperV("v3", "kept"))
	pt.syncXds(ctx, clusterWrapperV("v4", "kept"))

	assert.Equal(t, float64(1), metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels("kgateway_xds_cluster_scoping_transitions_total", []metrics.Label{
		{Name: gatewayLabel, Value: "unknown"},
		{Name: namespaceLabel, Value: "unknown"},
		{Name: transitionLabel, Value: transitionDereferenceGraced},
	}))
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

// TestClusterScopingDisabledWarnsOnTransitions: the warning fires when the
// gateway turns unscoped, names why, and stays quiet while it stays unscoped
// or recovers.
func TestClusterScopingDisabledWarnsOnTransitions(t *testing.T) {
	const client = "kube-gateway-api~ns-note~gw-note"
	t.Cleanup(func() { forgetClusterScopingClient(client, false) })
	log, logged := bufferLogger()
	reason := []string{`backend "tunneled" listener "tunnel_tunneled": sni_cluster`}
	warnings := func() int {
		return strings.Count(logged.String(), "cluster scoping disabled for this gateway")
	}

	reportClusterScopingDisabled(log, client, nil)
	assert.Equal(t, 0, warnings(), "scoped is not a warning")
	reportClusterScopingDisabled(log, client, reason)
	assert.Equal(t, 1, warnings(), "scoped to unscoped is a transition")
	assert.Contains(t, logged.String(), "tunnel_tunneled", "the warning names why")
	assert.Contains(t, logged.String(), "sni_cluster")
	reportClusterScopingDisabled(log, client, reason)
	reportClusterScopingDisabled(log, client, reason)
	assert.Equal(t, 1, warnings(), "staying unscoped is not")
	reportClusterScopingDisabled(log, client, nil)
	assert.Equal(t, 1, warnings(), "recovering is not")
	reportClusterScopingDisabled(log, client, reason)
	assert.Equal(t, 2, warnings(), "turning unscoped again is")
}

// TestClusterScopingDisabledIsAnyOfTheGatewaysClientGroups: a gateway reads as
// unscoped while any of its connected proxy groups is, through builds,
// departures and a reconnect, and warns only when that changes to unscoped.
func TestClusterScopingDisabledIsAnyOfTheGatewaysClientGroups(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	const (
		zoneA = "kube-gateway-api~ns-groups~gw-groups~zone-a"
		zoneB = "kube-gateway-api~ns-groups~gw-groups~zone-b"
	)
	t.Cleanup(func() { forgetClusterScopingClient(zoneA, false) })
	log, logged := bufferLogger()
	reason := []string{"sni_cluster"}
	disabled := func() float64 {
		t.Helper()
		return metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels("kgateway_xds_cluster_scoping_disabled_gateways", []metrics.Label{
			{Name: gatewayLabel, Value: "gw-groups"},
			{Name: namespaceLabel, Value: "ns-groups"},
		})
	}
	warnings := func() int {
		return strings.Count(logged.String(), "cluster scoping disabled for this gateway")
	}

	reportClusterScopingDisabled(log, zoneA, reason)
	reportClusterScopingDisabled(log, zoneB, nil)
	assert.Equal(t, float64(1), disabled(), "zone b's healthy build must not clear zone a's fallback")
	reportClusterScopingDisabled(log, zoneB, nil)
	assert.Equal(t, float64(1), disabled())
	reportClusterScopingDisabled(log, zoneB, reason)
	assert.Equal(t, 1, warnings(), "a second unscoped group does not turn the gateway unscoped again")

	reportClusterScopingDisabled(log, zoneA, nil)
	assert.Equal(t, float64(1), disabled(), "zone b is still unscoped")
	forgetClusterScopingClient(zoneB, true)
	assert.Equal(t, float64(0), disabled(), "zone b's departure takes its fallback with it")

	reportClusterScopingDisabled(log, zoneB, reason)
	assert.Equal(t, float64(1), disabled(), "a reconnecting group counts again")
	assert.Equal(t, 2, warnings(), "and the gateway turning unscoped again warns again")
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
	t.Cleanup(func() {
		forgetClusterScopingClient(leaving, false)
		forgetClusterScopingClient(other, false)
	})
	log, logged := bufferLogger()
	for _, client := range []string{leaving, other} {
		recordClusterScopingDropped(client, 17)
		reportClusterScopingDisabled(log, client, []string{"sni_cluster"})
	}
	stillThere := []ir.UniquelyConnectedClient{newTestUcc(leaving)}
	require.True(t, gatewayHasClients(leaving, stillThere), "another client of the gateway is still connected")
	require.False(t, gatewayHasClients(leaving, []ir.UniquelyConnectedClient{newTestUcc(other)}))

	forgetClusterScopingClient(leaving, false)

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
	reportClusterScopingDisabled(log, leaving, []string{"sni_cluster"})
	assert.Equal(t, 3, strings.Count(logged.String(), "cluster scoping disabled for this gateway"),
		"a returning gateway that is unscoped warns again")
}

// bufferLogger returns a logger that writes warnings to the returned buffer.
func bufferLogger() (*slog.Logger, *bytes.Buffer) {
	var logged bytes.Buffer
	warn := slog.LevelWarn
	return logging.NewWithOptions("proxy_syncer_test", logging.Options{Writer: &logged, Level: &warn, Format: logging.TextFormat}), &logged
}

func discardLogger() *slog.Logger {
	log, _ := bufferLogger()
	return log
}
