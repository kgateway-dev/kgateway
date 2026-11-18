package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

// This key has no gateway or namespace, so labels are "unknown".
var unknownLabels = []metrics.Label{
	{Name: "gateway", Value: "unknown"},
	{Name: "namespace", Value: "unknown"},
}

func labelsWith(name, value string) []metrics.Label {
	return append(append([]metrics.Label{}, unknownLabels...), metrics.Label{Name: name, Value: value})
}

// A deferred build followed by a bounded first publish increments
// defers_total{missing_clusters} and bounded_publishes_total{first_publish}.
func TestPublicationMetrics_DeferAndBoundedPublish(t *testing.T) {
	ResetMetrics()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pt := newPublishGateTestTranslator(t, false, 30*time.Millisecond)
	pt.syncXds(ctx, deferredWrapperV("v1"))
	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 2*time.Second, 5*time.Millisecond)

	g := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_xds_snapshot_perclient_defers_total",
		"kgateway_xds_snapshot_perclient_bounded_publishes_total")
	g.AssertMetricsInclude("kgateway_xds_snapshot_perclient_defers_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetricValueTest{
			Labels: labelsWith("reason", deferReasonMissingClusters),
			Test:   metricstest.GreaterOrEqual(1),
		},
	})
	g.AssertMetricsInclude("kgateway_xds_snapshot_perclient_bounded_publishes_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetricValueTest{
			Labels: labelsWith("mode", boundedPublishFirstPublish),
			Test:   metricstest.Equal(1),
		},
	})
}

// A held route flip released at budget expiry increments
// bounded_publishes_total{flip_release}.
func TestPublicationMetrics_FlipRelease(t *testing.T) {
	ResetMetrics()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pt := newPublishGateTestTranslator(t, false, 30*time.Millisecond)
	_, flipWrap := flipHoldFixture(t, pt)
	pt.syncXds(ctx, flipWrap)
	require.Eventually(t, func() bool {
		return snapshotReferencesCluster(servedSnapshot(t, pt), "cluster-new")
	}, 2*time.Second, 5*time.Millisecond)

	g := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_xds_snapshot_perclient_bounded_publishes_total")
	g.AssertMetricsInclude("kgateway_xds_snapshot_perclient_bounded_publishes_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetricValueTest{
			Labels: labelsWith("mode", boundedPublishFlipRelease),
			Test:   metricstest.Equal(1),
		},
	})
}

// A warm bounded publish with only underived CLAs increments
// bounded_publishes_total{warm_truth}.
func TestPublicationMetrics_WarmTruthBoundedPublish(t *testing.T) {
	ResetMetrics()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pt := newPublishGateTestTranslator(t, true, 30*time.Millisecond)
	pt.syncXds(ctx, deferredMissingEndpointsWrapperV("v1"))
	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 2*time.Second, 5*time.Millisecond)

	g := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_xds_snapshot_perclient_bounded_publishes_total")
	g.AssertMetricsInclude("kgateway_xds_snapshot_perclient_bounded_publishes_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetricValueTest{
			Labels: labelsWith("mode", boundedPublishWarmTruth),
			Test:   metricstest.Equal(1),
		},
	})
}

// With KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK on, an orphan CLA is counted but still
// published. A plain cache is used because the test oracle rejects it.
func TestPublicationMetrics_InconsistentSnapshotRecorded(t *testing.T) {
	ResetMetrics()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pt := NewProxyTranslator(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), nil, 0, true, clusterScoping{})
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("e1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "orphan"}},
	})
	pt.syncXds(ctx, XdsSnapWrapper{snap: snap, proxyKey: publishGateTestClient})

	_, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	require.NoError(t, err, "the inconsistent snapshot must still be published")

	g := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_xds_snapshot_perclient_inconsistent_snapshots_total")
	g.AssertMetric("kgateway_xds_snapshot_perclient_inconsistent_snapshots_total",
		&metricstest.ExpectedMetricValueTest{Test: metricstest.Equal(1)})
}

const withheldClientsMetric = "kgateway_xds_snapshot_perclient_withheld_clients"

// withheldClients reads the withheld_clients gauge for state. No series reads as
// 0; a series only for another state fails the read.
func withheldClients(t *testing.T, state string) float64 {
	t.Helper()
	g := metricstest.MustGatherMetrics(t)
	if g.MetricLength(withheldClientsMetric) == 0 {
		return 0
	}
	return g.MustGetMetricValueByLabels(withheldClientsMetric, labelsWith("state", state))
}

// withheld_clients counts a client as within_budget while its first publish is
// withheld, and drops the series once it publishes or leaves.
func TestPublicationMetrics_WithheldClients(t *testing.T) {
	ctx := context.Background()
	gone := func(msg string) {
		t.Helper()
		require.Zero(t, metricstest.MustGatherMetrics(t).MetricLength(withheldClientsMetric), msg)
	}

	ResetMetrics()
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	pt.syncXds(ctx, deferredWrapperV("v1"))
	require.Equal(t, float64(1), withheldClients(t, withheldWithinBudget), "withheld from its first publish")
	pt.syncXds(ctx, deferredWrapperV("v2"))
	require.Equal(t, float64(1), withheldClients(t, withheldWithinBudget), "one client, however many deferred builds")
	pt.syncXds(ctx, coherentWrapperV("v3"))
	gone("published: no series is left at zero")

	bounded := newPublishGateTestTranslator(t, false, 30*time.Millisecond)
	bounded.syncXds(ctx, deferredWrapperV("v1"))
	require.Equal(t, float64(1), withheldClients(t, withheldWithinBudget))
	require.Eventually(t, func() bool {
		return metricstest.MustGatherMetrics(t).MetricLength(withheldClientsMetric) == 0
	}, 2*time.Second, 5*time.Millisecond, "published at the budget")

	departed := newPublishGateTestTranslator(t, false, time.Hour)
	departed.syncXds(ctx, deferredWrapperV("v1"))
	require.Equal(t, float64(1), withheldClients(t, withheldWithinBudget))
	departed.gate.clientDeparted(publishGateTestClient)
	gone("left before its first publish")
}
