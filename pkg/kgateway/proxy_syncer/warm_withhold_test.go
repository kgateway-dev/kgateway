package proxy_syncer

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

// newWarmWithholdTestTranslator builds a gate for a warm client with the given
// budget and bound.
func newWarmWithholdTestTranslator(t *testing.T, budget, bound time.Duration) *ProxyTranslator {
	pt := newPublishGateTestTranslator(t, true, budget)
	pt.gate.warmWithholdBound = bound
	return pt
}

// warmWithheldCount totals warm holds across gateways.
func warmWithheldCount(pt *ProxyTranslator) int {
	pt.gate.mu.Lock()
	defer pt.gate.mu.Unlock()
	n := 0
	for _, c := range pt.gate.warmWithheld {
		n += c
	}
	return n
}

// The warm bound fires with no further builds: a permanent gap is exactly the
// static-input case.
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
	assert.Equal(t, float64(1), withheldClients(t, withheldWarm), "past the budget the client moves to warm_withheld")
	assert.Equal(t, 1, metricstest.MustGatherMetrics(t).MetricLength(withheldClientsMetric), "and leaves within_budget")

	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 2*time.Second, 5*time.Millisecond, "the warm client must be published once withheld for the whole bound")
	v, _ := publishedListenerVersion(t, pt)
	assert.Equal(t, "v1", v)
	assert.Zero(t, warmWithheldCount(pt), "a released client is no longer withheld")

	g := metricstest.MustGatherMetricsContext(ctx, t,
		"kgateway_xds_snapshot_perclient_bounded_publishes_total")
	g.AssertMetricsInclude("kgateway_xds_snapshot_perclient_bounded_publishes_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetricValueTest{
			Labels: labelsWith("mode", boundedPublishWarmRelease),
			Test:   metricstest.Equal(1),
		},
	})
	g.AssertMetricNotExists(withheldClientsMetric)
}

// A zero warm bound allows indefinite holds while retaining the gauge count.
func TestWarmWithhold_BoundZeroWithholdsWithoutDeadline(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, 0)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))

	require.Never(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 250*time.Millisecond, 20*time.Millisecond, "with no bound, a warm client with missing clusters must never be released")
	assert.Equal(t, 1, warmWithheldCount(pt))
}

// With CDS complete, the next budget check publishes despite underived CLAs.
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

// mutablePriorXDS models the warm mark clearing once the key's streams close.
type mutablePriorXDS struct {
	mu  sync.Mutex
	has bool
}

func (m *mutablePriorXDS) HasPriorXDSVersion(string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.has
}

func (m *mutablePriorXDS) set(has bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.has = has
}

// Changing the missing-cluster set must not restart the warm episode.
func TestWarmWithhold_ChangingMissingSetDoesNotRestartBound(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, 250*time.Millisecond)
	start := time.Now()

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	require.Eventually(t, func() bool { return warmWithheldCount(pt) == 1 },
		time.Second, 5*time.Millisecond)
	time.Sleep(150 * time.Millisecond)
	v2 := deferredWrapperV("v2")
	v2.missingReferenced = []string{"cluster-other"}
	pt.syncXds(context.Background(), v2)

	require.Eventually(t, func() bool {
		v, ok := publishedListenerVersion(t, pt)
		return ok && v == "v2"
	}, 2*time.Second, 5*time.Millisecond, "the latest deferred snapshot must be released at the bound")
	assert.Less(t, time.Since(start), 400*time.Millisecond,
		"the bound must run from the first withhold, not from the latest change in missing clusters")
}

// Cold clients awaiting first publication do not count as warm holds.
func TestWarmWithhold_ColdClientNeverCounted(t *testing.T) {
	pt := newPublishGateTestTranslator(t, false, 40*time.Millisecond)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	assert.Zero(t, warmWithheldCount(pt))
	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, time.Second, 5*time.Millisecond)
	assert.Zero(t, warmWithheldCount(pt))
}

// Warmth is re-read at each budget check; once every warm stream closes, the
// next check publishes as a first publish.
func TestWarmWithhold_MarkClearedPublishesAtNextCheck(t *testing.T) {
	prior := &mutablePriorXDS{has: true}
	pt := newPublishGateTestTranslator(t, true, 20*time.Millisecond)
	pt.gate.clientState = prior
	pt.gate.warmWithholdBound = time.Minute

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	require.Eventually(t, func() bool { return warmWithheldCount(pt) == 1 },
		time.Second, 5*time.Millisecond)
	_, ok := publishedListenerVersion(t, pt)
	require.False(t, ok)

	prior.set(false)
	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, time.Second, 5*time.Millisecond, "a key that is no longer warm must publish at the next budget check")
	assert.Zero(t, warmWithheldCount(pt))
}

// After release, resolve later builds against this connection's published
// snapshot instead of restarting first publication.
func TestWarmWithhold_AfterReleaseLaterBuildsResolvePerCluster(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, 60*time.Millisecond)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, time.Second, 5*time.Millisecond)

	pt.syncXds(context.Background(), deferredWrapperV("v2"))
	pt.gate.mu.Lock()
	st := pt.gate.pending[publishGateTestClient]
	pt.gate.mu.Unlock()
	assert.True(t, st == nil || st.wrap.snap == nil,
		"a client with a published snapshot must not be offered to the cold-path withhold again")
	assert.Zero(t, warmWithheldCount(pt))
}

// Reconnects resume the warm episode; otherwise frequent resets never release.
func TestWarmWithhold_ReconnectDuringWithholdDoesNotRestartBound(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, 250*time.Millisecond)
	start := time.Now()

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	for time.Since(start) < 600*time.Millisecond {
		if _, ok := publishedListenerVersion(t, pt); ok {
			break
		}
		time.Sleep(100 * time.Millisecond)
		// Close the last stream and reconnect with its held config.
		pt.gate.clientDeparted(publishGateTestClient)
		pt.syncXds(context.Background(), deferredWrapperV("v1"))
	}
	_, ok := publishedListenerVersion(t, pt)
	assert.True(t, ok, "a client that keeps reconnecting while withheld must still be released at the bound")
}

// sharedKeyClients models warm proxies and an optional cold proxy that has
// waited a budget under the same key.
type sharedKeyClients struct{ coldWaited bool }

func (sharedKeyClients) HasPriorXDSVersion(string) bool { return true }

func (s sharedKeyClients) HasColdStreamSince(string, time.Duration) bool { return s.coldWaited }

func newSharedKeyTestTranslator(t *testing.T, coldWaited bool, budget, bound time.Duration) *ProxyTranslator {
	pt := newPublishGateTestTranslator(t, true, budget)
	pt.gate.clientState = sharedKeyClients{coldWaited: coldWaited}
	pt.gate.warmWithholdBound = bound
	return pt
}

// A cold proxy sharing a warm key is published after one budget; otherwise it
// has no listeners and crash-loops.
func TestWarmWithhold_ColdProxySharingKeyIsPublishedTo(t *testing.T) {
	for _, bound := range []time.Duration{time.Minute, 0} {
		t.Run(fmt.Sprintf("bound=%s", bound), func(t *testing.T) {
			pt := newSharedKeyTestTranslator(t, true, 30*time.Millisecond, bound)

			pt.syncXds(context.Background(), deferredWrapperV("v1"))

			require.Eventually(t, func() bool {
				_, ok := publishedListenerVersion(t, pt)
				return ok
			}, time.Second, 5*time.Millisecond, "a key with a cold proxy that waited a budget must publish")
			assert.Zero(t, warmWithheldCount(pt))
		})
	}
}

// Warm proxies alone on a key stay withheld while clusters are missing.
func TestWarmWithhold_WarmOnlyKeyStaysWithheld(t *testing.T) {
	pt := newSharedKeyTestTranslator(t, false, 30*time.Millisecond, time.Minute)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))

	require.Never(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 200*time.Millisecond, 20*time.Millisecond)
	assert.Equal(t, 1, warmWithheldCount(pt))
}

// Departures within the bound resume the warm episode; longer ones restart it.
func TestWarmWithhold_LongAbsenceStartsFreshEpisode(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, 200*time.Millisecond)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	require.Eventually(t, func() bool { return warmWithheldCount(pt) == 1 },
		time.Second, 5*time.Millisecond)
	pt.gate.clientDeparted(publishGateTestClient)
	time.Sleep(250 * time.Millisecond) // longer than the bound

	returned := time.Now()
	pt.syncXds(context.Background(), deferredWrapperV("v2"))
	require.Eventually(t, func() bool {
		_, ok := publishedListenerVersion(t, pt)
		return ok
	}, 2*time.Second, 5*time.Millisecond)
	assert.GreaterOrEqual(t, time.Since(returned), 180*time.Millisecond,
		"after a long absence the bound must run again from the new episode")
}

// A publish ends the episode: a later withhold for the same key starts fresh.
func TestWarmWithhold_PublishForgetsEpisode(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, time.Minute)

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	require.Eventually(t, func() bool { return warmWithheldCount(pt) == 1 },
		time.Second, 5*time.Millisecond)
	pt.gate.clientDeparted(publishGateTestClient)
	pt.syncXds(context.Background(), coherentWrapperV("coherent"))

	pt.gate.mu.Lock()
	_, remembered := pt.gate.warmEpisodes[publishGateTestClient]
	pt.gate.mu.Unlock()
	assert.False(t, remembered, "a publish must drop the remembered episode")
}

// The syncer's client state must expose the cold-stream check so a cold proxy
// can lift a shared warm withhold.
func TestWarmWithhold_ClientStateReportsColdProxies(t *testing.T) {
	_, _, state := krtcollections.NewUniquelyConnectedClients(false)
	_, ok := state.(coldStreamReader)
	assert.True(t, ok)
}
