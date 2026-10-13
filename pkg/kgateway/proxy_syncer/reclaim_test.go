package proxy_syncer

import (
	"context"
	"sync"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoydiscoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	envoystreamv3 "github.com/envoyproxy/go-control-plane/pkg/server/stream/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shortenReclaimGrace shrinks the grace period for the duration of a test.
// Tests using it must not run in parallel.
func shortenReclaimGrace(t *testing.T, d time.Duration) {
	t.Helper()
	orig := snapshotReclaimGracePeriod
	snapshotReclaimGracePeriod = d
	t.Cleanup(func() { snapshotReclaimGracePeriod = orig })
}

// fakeReclaim stands in for publishGate.reclaimDeparted: it reclaims only
// clients absent from the connected set, and records what it reclaimed.
type fakeReclaim struct {
	mu        sync.Mutex
	connected map[string]bool
	reclaimed []string
}

func newFakeReclaim() *fakeReclaim { return &fakeReclaim{connected: map[string]bool{}} }

func (f *fakeReclaim) setConnected(name string, connected bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected[name] = connected
}

func (f *fakeReclaim) reclaim(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.connected[name] {
		return false
	}
	f.reclaimed = append(f.reclaimed, name)
	return true
}

func (f *fakeReclaim) reclaimedNodes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.reclaimed...)
}

func TestReclaim_DepartedEntryClearedAfterGrace(t *testing.T) {
	shortenReclaimGrace(t, 30*time.Millisecond)
	f := newFakeReclaim()
	r := newSnapshotReclaimer(f.reclaim)

	r.clientDeparted("c1")
	assert.Empty(t, f.reclaimedNodes(), "must not reclaim before the grace period")

	require.Eventually(t, func() bool {
		return len(f.reclaimedNodes()) == 1
	}, 2*time.Second, 5*time.Millisecond, "departed client's entry must be reclaimed after the grace period")
	assert.Equal(t, []string{"c1"}, f.reclaimedNodes())
}

func TestReclaim_ReconnectWithinGraceCancels(t *testing.T) {
	shortenReclaimGrace(t, 40*time.Millisecond)
	f := newFakeReclaim()
	r := newSnapshotReclaimer(f.reclaim)

	r.clientDeparted("c1")
	r.clientConnected("c1")

	time.Sleep(120 * time.Millisecond)
	assert.Empty(t, f.reclaimedNodes(), "a client that reconnected within the grace period must not be reclaimed")
}

func TestReclaim_FireTimeDecisionWins(t *testing.T) {
	// The collection event announcing a reconnect can lag the stream; the
	// decision at fire time must win over the timer.
	shortenReclaimGrace(t, 30*time.Millisecond)
	f := newFakeReclaim()
	r := newSnapshotReclaimer(f.reclaim)

	r.clientDeparted("c1")
	f.setConnected("c1", true) // reconnected, but clientConnected never called

	time.Sleep(120 * time.Millisecond)
	assert.Empty(t, f.reclaimedNodes(), "a client connected at fire time must not be reclaimed")
}

func TestReclaim_DuplicateDepartsArmOneTimer(t *testing.T) {
	shortenReclaimGrace(t, 30*time.Millisecond)
	f := newFakeReclaim()
	r := newSnapshotReclaimer(f.reclaim)

	r.clientDeparted("c1")
	r.clientDeparted("c1")
	r.clientDeparted("c1")

	require.Eventually(t, func() bool {
		return len(f.reclaimedNodes()) >= 1
	}, 2*time.Second, 5*time.Millisecond)
	time.Sleep(80 * time.Millisecond)
	assert.Len(t, f.reclaimedNodes(), 1, "duplicate departure events must not produce duplicate reclaims")
}

func TestReclaim_IndependentClients(t *testing.T) {
	shortenReclaimGrace(t, 30*time.Millisecond)
	f := newFakeReclaim()
	r := newSnapshotReclaimer(f.reclaim)

	r.clientDeparted("gone")
	r.clientDeparted("back")
	r.clientConnected("back")

	require.Eventually(t, func() bool {
		return len(f.reclaimedNodes()) == 1
	}, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{"gone"}, f.reclaimedNodes(),
		"only the still-departed client is reclaimed")
}

// stubConnections models the synchronous connection state behind
// publishGate.reclaimDeparted.
type stubConnections struct{ connected bool }

func (s stubConnections) RunIfDisconnected(_ string, fn func()) bool {
	if s.connected {
		return false
	}
	fn()
	return true
}

// openClusterWatch publishes version v1 for node and opens the watch a
// connected Envoy holding v1 would have: parked until the next version.
func openClusterWatch(t *testing.T, cache envoycache.SnapshotCache, node string) chan envoycache.Response {
	t.Helper()
	snap, err := envoycache.NewSnapshot("v1", map[envoyresourcev3.Type][]envoycachetypes.Resource{envoyresourcev3.ClusterType: {}})
	require.NoError(t, err)
	require.NoError(t, cache.SetSnapshot(context.Background(), node, snap))
	req := &envoydiscoveryv3.DiscoveryRequest{
		Node:        &envoycorev3.Node{Id: node},
		TypeUrl:     envoyresourcev3.ClusterType,
		VersionInfo: "v1",
	}
	responses := make(chan envoycache.Response, 1)
	_, err = cache.CreateWatch(req, envoystreamv3.NewSotwSubscription(nil, true), responses)
	require.NoError(t, err)
	require.Equal(t, 1, cache.GetStatusInfo(node).GetNumWatches(), "the watch must be parked at v1")
	return responses
}

func TestReclaimDeparted(t *testing.T) {
	const node = "c1"
	v2, err := envoycache.NewSnapshot("v2", map[envoyresourcev3.Type][]envoycachetypes.Resource{envoyresourcev3.ClusterType: {}})
	require.NoError(t, err)

	t.Run("connected client keeps its entry and its watches", func(t *testing.T) {
		cache := envoycache.NewSnapshotCache(false, envoycache.IDHash{}, nil)
		responses := openClusterWatch(t, cache, node)

		gate := newPublishGateTestTranslator(t, false, 0).gate
		assert.False(t, gate.reclaimDeparted(cache, node, stubConnections{connected: true}))
		_, err := cache.GetSnapshot(node)
		require.NoError(t, err, "a connected client's snapshot must survive")

		require.NoError(t, gate.publish(context.Background(), cache, node, v2))
		select {
		case resp := <-responses:
			assert.Equal(t, "v2", resp.GetResponseVersion())
		case <-time.After(time.Second):
			t.Fatal("the connected client's watch must still be answered")
		}
	})

	t.Run("departed client's entry is cleared", func(t *testing.T) {
		cache := envoycache.NewSnapshotCache(false, envoycache.IDHash{}, nil)
		snap, err := envoycache.NewSnapshot("v1", map[envoyresourcev3.Type][]envoycachetypes.Resource{envoyresourcev3.ClusterType: {}})
		require.NoError(t, err)
		require.NoError(t, cache.SetSnapshot(context.Background(), node, snap))

		assert.True(t, newPublishGateTestTranslator(t, false, 0).gate.reclaimDeparted(cache, node, stubConnections{}))
		_, err = cache.GetSnapshot(node)
		require.Error(t, err, "a departed client's snapshot must be cleared")
		assert.Empty(t, cache.GetStatusKeys(), "a departed client's status must be cleared")
	})

	// Why the clear must be excluded from stream admission: ClearSnapshot also
	// drops open watches, so a clear that misses a connected stream strands it.
	t.Run("clearing under a connected stream strands its watch", func(t *testing.T) {
		cache := envoycache.NewSnapshotCache(false, envoycache.IDHash{}, nil)
		responses := openClusterWatch(t, cache, node)

		cache.ClearSnapshot(node)
		require.NoError(t, cache.SetSnapshot(context.Background(), node, v2))
		select {
		case resp := <-responses:
			t.Fatalf("unexpected response %q: ClearSnapshot no longer drops open watches; revisit reclaimDeparted", resp.GetResponseVersion())
		case <-time.After(100 * time.Millisecond):
		}
	})
}
