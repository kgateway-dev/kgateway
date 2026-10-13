package proxy_syncer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

// newWarmWithholdTestTranslator builds a gate for a warm client (it reported a
// prior xDS version) with the given budget and warm withhold bound.
func newWarmWithholdTestTranslator(t *testing.T, budget, bound time.Duration) *ProxyTranslator {
	pt := newPublishGateTestTranslator(t, true, budget)
	pt.gate.warmWithholdBound = bound
	return pt
}

// warmWithheldCount reads the gate's count of currently withheld warm clients
// across all gateways.
func warmWithheldCount(pt *ProxyTranslator) int {
	pt.gate.mu.Lock()
	defer pt.gate.mu.Unlock()
	n := 0
	for _, c := range pt.gate.warmWithheld {
		n += c
	}
	return n
}

// A warm client withheld for missing clusters is published once the bound is
// reached, even when no further deferred build arrives to re-arm the timer:
// the permanent case is exactly the one with static inputs.
func TestWarmWithhold_ReleasedAtBoundWithStaticInputs(t *testing.T) {
	ResetMetrics()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, 200*time.Millisecond)

	pt.syncXds(ctx, deferredWrapperV("v1")) // the only event

	require.Eventually(t, func() bool { return warmWithheldCount(pt) == 1 },
		time.Second, 5*time.Millisecond, "the client must be counted as withheld after the first budget expiry")
	_, ok := publishedListenerVersion(t, pt)
	assert.False(t, ok, "a warm client with missing clusters must stay withheld before the bound")

	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 2*time.Second, 5*time.Millisecond, "the warm client must be published once withheld for the whole bound")
	v, _ := publishedListenerVersion(t, pt)
	assert.Equal(t, "v1", v)
	assert.Zero(t, warmWithheldCount(pt), "a released client is no longer withheld")

	g := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_xds_snapshot_perclient_bounded_publishes_total",
		"kgateway_xds_snapshot_perclient_warm_withheld_clients")
	g.AssertMetricsInclude("kgateway_xds_snapshot_perclient_bounded_publishes_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetricValueTest{
			Labels: labelsWith("mode", boundedPublishWarmRelease),
			Test:   metricstest.Equal(1),
		},
	})
	g.AssertMetricsInclude("kgateway_xds_snapshot_perclient_warm_withheld_clients", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetricValueTest{Labels: unknownLabels, Test: metricstest.Equal(0)},
	})
}

// With the bound at 0 the warm withhold has no deadline, and the client stays
// counted as withheld.
func TestWarmWithhold_BoundZeroWithholdsWithoutDeadline(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, 0)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))

	require.Never(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 250*time.Millisecond, 20*time.Millisecond, "with no bound, a warm client with missing clusters must never be released")
	assert.Equal(t, 1, warmWithheldCount(pt))
}

// If the missing clusters appear while endpoints are still underived, the
// warm-truth publish happens at the next budget check, not at the bound.
func TestWarmWithhold_MissingClustersResolvedPublishesTruthPromptly(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, time.Minute)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	require.Eventually(t, func() bool { return warmWithheldCount(pt) == 1 },
		time.Second, 5*time.Millisecond)

	pt.syncXds(context.Background(), deferredMissingEndpointsWrapperV("v2"))

	require.Eventually(t, func() bool {
		v, ok := publishedListenerVersion(t, pt)
		return ok && v == "v2"
	}, time.Second, 5*time.Millisecond, "once nothing is missing from CDS, the warm-truth publish must not wait for the bound")
	assert.Zero(t, warmWithheldCount(pt))
}

// A coherent publish ends the withhold and clears the count.
func TestWarmWithhold_CoherentPublishClearsCount(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, time.Minute)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	require.Eventually(t, func() bool { return warmWithheldCount(pt) == 1 },
		time.Second, 5*time.Millisecond)

	pt.syncXds(context.Background(), coherentWrapperV("coherent"))

	v, ok := publishedListenerVersion(t, pt)
	require.True(t, ok)
	assert.Equal(t, "coherent", v)
	assert.Zero(t, warmWithheldCount(pt))
	time.Sleep(100 * time.Millisecond) // past several budget re-checks
	v, _ = publishedListenerVersion(t, pt)
	assert.Equal(t, "coherent", v, "a canceled re-check must not overwrite the coherent snapshot")
}

// A departed client is no longer counted, and its re-check does not fire.
func TestWarmWithhold_DepartureClearsCount(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, 100*time.Millisecond)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	require.Eventually(t, func() bool { return warmWithheldCount(pt) == 1 },
		time.Second, 5*time.Millisecond)

	pt.gate.clientDeparted(publishGateTestClient)

	assert.Zero(t, warmWithheldCount(pt))
	time.Sleep(250 * time.Millisecond) // past the bound
	_, ok := publishedListenerVersion(t, pt)
	assert.False(t, ok, "a departed client must not be released at the bound")
}
