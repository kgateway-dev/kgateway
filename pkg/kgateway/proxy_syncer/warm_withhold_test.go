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

// mutablePriorXDS models a client key whose warm mark can change between
// budget checks: the mark is cleared when every stream under the key closes.
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

// The bound measures the whole withhold episode: a change in WHICH clusters
// are missing must not restart it.
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

// A cold client is never counted as a withheld warm client, even while it
// waits out the first-publish budget.
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

// The warm mark is read at every budget check, not latched at the first one:
// once the key stops being warm (every stream that reported a prior version
// has closed), the next check publishes as a first publish.
func TestWarmWithhold_MarkClearedPublishesAtNextCheck(t *testing.T) {
	prior := &mutablePriorXDS{has: true}
	pt := newPublishGateTestTranslator(t, true, 20*time.Millisecond)
	pt.xdsClientState = prior
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

// Once a warm client has been released, later builds resolve per cluster
// against what was published; the cold-path withhold never applies again.
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
	assert.True(t, st == nil || st.snap == nil,
		"a client with a published snapshot must not be offered to the cold-path withhold again")
	assert.Zero(t, warmWithheldCount(pt))
}

// A stream that flaps (closes and reconnects under the same key) while the
// client is withheld must not restart the bound: a client whose connection
// resets more often than the bound would otherwise never be released.
func TestWarmWithhold_ReconnectDuringWithholdDoesNotRestartBound(t *testing.T) {
	pt := newWarmWithholdTestTranslator(t, 20*time.Millisecond, 250*time.Millisecond)
	start := time.Now()

	pt.syncXds(context.Background(), deferredWrapperV("v1"))
	for time.Since(start) < 600*time.Millisecond {
		if _, ok := publishedListenerVersion(t, pt); ok {
			break
		}
		time.Sleep(100 * time.Millisecond)
		// The last stream closes (its snapshot row is deleted) and the
		// proxy reconnects, still holding the config it had.
		pt.gate.clientDeparted(publishGateTestClient)
		pt.syncXds(context.Background(), deferredWrapperV("v1"))
	}
	_, ok := publishedListenerVersion(t, pt)
	assert.True(t, ok, "a client that keeps reconnecting while withheld must still be released at the bound")
}

// sharedKeyClients models a key with warm proxies and, optionally, a proxy
// holding no config that has waited a full budget under the same key.
type sharedKeyClients struct{ coldWaited bool }

func (sharedKeyClients) HasPriorXDSVersion(string) bool { return true }

func (s sharedKeyClients) HasColdStreamSince(string, time.Duration) bool { return s.coldWaited }

func newSharedKeyTestTranslator(t *testing.T, coldWaited bool, budget, bound time.Duration) *ProxyTranslator {
	pt := newPublishGateTestTranslator(t, true, budget)
	pt.xdsClientState = sharedKeyClients{coldWaited: coldWaited}
	pt.gate.warmWithholdBound = bound
	return pt
}

// A proxy holding no config that shares the key with warm proxies is published
// to once it has waited a budget: otherwise it has no listeners and
// crash-loops on behalf of its warm siblings.
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

// A proxy that stays away longer than the bound starts a fresh episode when it
// returns: only reconnects within the bound resume the old one.
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

// The client state the syncer is given must expose the cold-proxy check;
// without it, a cold proxy sharing a warm key is never published to.
func TestWarmWithhold_ClientStateReportsColdProxies(t *testing.T) {
	_, _, state := krtcollections.NewUniquelyConnectedClients(nil, false)
	_, ok := state.(coldStreamReader)
	assert.True(t, ok)
}
