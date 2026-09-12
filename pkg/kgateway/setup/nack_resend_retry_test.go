package setup

import (
	"context"
	"sync"
	"testing"
	"time"

	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	serverconfig "github.com/envoyproxy/go-control-plane/pkg/server/config"
	sotwv3 "github.com/envoyproxy/go-control-plane/pkg/server/sotw/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/stream/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"

	kgwxds "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
)

// parkNamedEndpointNack has the stream receive EDS version 1 for adsTestCluster
// and NACK it, which parks the watch.
func parkNamedEndpointNack(t *testing.T, h *adsHarness) {
	t.Helper()

	h.setSnapshot(edsOnlySnapshot(t, "1", adsTestCluster))
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}})
	first := h.receiveType(envoyresource.EndpointType, "1")
	h.send(&discoveryv3.DiscoveryRequest{
		Node:          h.node,
		TypeUrl:       envoyresource.EndpointType,
		ResourceNames: []string{adsTestCluster},
		ResponseNonce: first.GetNonce(),
		ErrorDetail:   &status.Status{Message: "rejected by test"},
	})
	h.waitForOpenWatches(1)
}

// adsOnlyCache is a single ADS snapshot cache. newSnapshotCache serves EDS per
// request (#14865) and answers a named request whatever else the snapshot
// holds; the tests below need a cache that cannot answer one, as the ADS cache
// still cannot for the types it serves.
func adsOnlyCache() envoycache.SnapshotCache {
	return envoycache.NewSnapshotCache(true, kgwxds.NewNodeRoleHasher(), nil)
}

// The ADS cache keeps a named watch it cannot answer, but drops a new one. A
// retry that replaced the kept watch while the snapshot held an endpoint set
// the proxy does not request would leave the stream with no EDS watch, so a
// later answerable snapshot would never be sent.
func TestParkedNackRetryKeepsAWatchTheCacheCannotAnswer(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "ordered"}[ordered], func(t *testing.T) {
			shortenNackRetry(t, 100*time.Millisecond, time.Second)
			h := newNackHarnessOver(t, ordered, adsOnlyCache())
			parkNamedEndpointNack(t, h)

			// The cache cannot answer the parked watch while the snapshot holds
			// an unrequested endpoint set; the retry deadline passes meanwhile.
			h.setSnapshot(edsOnlySnapshot(t, "2", adsTestCluster, "unrequested"))
			h.expectSilence("the snapshot holds an endpoint set outside the request")
			h.waitForOpenWatches(1)

			h.setSnapshot(edsOnlySnapshot(t, "3", adsTestCluster))
			h.receiveType(envoyresource.EndpointType, "3")
		})
	}
}

// The snapshot can leave the rejected version and come back to it while the
// parked watch waits: removing the endpoint set that blocked it restores the
// same content, and so the same version. The parked watch claims that version,
// so only the retry can re-send it, and it must not have given up.
func TestParkedNackRetryResumesWhenTheRejectedVersionReturns(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "ordered"}[ordered], func(t *testing.T) {
			shortenNackRetry(t, 50*time.Millisecond, 50*time.Millisecond)
			h := newNackHarnessOver(t, ordered, adsOnlyCache())
			parkNamedEndpointNack(t, h)

			h.setSnapshot(edsOnlySnapshot(t, "2", adsTestCluster, "unrequested"))
			h.expectSilence("the snapshot holds an endpoint set outside the request")
			h.setSnapshot(edsOnlySnapshot(t, "1", adsTestCluster))
			h.receiveType(envoyresource.EndpointType, "1")
		})
	}
}

// snapshotReadGate pauses the first GetSnapshot after it is armed, once the
// snapshot has been read: the point where a retry has checked the version and
// not yet replaced the parked watch.
type snapshotReadGate struct {
	envoycache.SnapshotCache

	mu     sync.Mutex
	armed  bool
	read   chan struct{}
	resume chan struct{}
	once   sync.Once
}

// release lets a paused GetSnapshot return; later calls do nothing.
func (g *snapshotReadGate) release() {
	g.once.Do(func() { close(g.resume) })
}

func (g *snapshotReadGate) arm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.armed = true
}

func (g *snapshotReadGate) GetSnapshot(node string) (envoycache.ResourceSnapshot, error) {
	snapshot, err := g.SnapshotCache.GetSnapshot(node)
	g.mu.Lock()
	pause := g.armed
	g.armed = false
	g.mu.Unlock()
	if pause {
		close(g.read)
		<-g.resume
	}
	return snapshot, err
}

// newNackHarnessOver is newPolicyHarness with the policy in front of inner.
func newNackHarnessOver(t *testing.T, ordered bool, inner envoycache.SnapshotCache) *adsHarness {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	stream := &mockAdsStream{
		ctx:  ctx,
		sent: make(chan *discoveryv3.DiscoveryResponse, 16),
		recv: make(chan *discoveryv3.DiscoveryRequest, 16),
	}
	suppressor := newWatchPolicy(inner, kgwxds.NewNodeRoleHasher(), true, true)
	var opts []serverconfig.XDSOption
	if ordered {
		opts = append(opts, sotwv3.WithOrderedADS())
	}
	srv := xdsserver.NewServer(ctx, suppressor, suppressor.callbacks(), opts...)
	h := &adsHarness{t: t, ctx: ctx, cancel: cancel, cache: suppressor, stream: stream, done: make(chan error, 1), node: adsTestNode(t)}
	go func() {
		h.done <- srv.StreamAggregatedResources(stream)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-h.done:
			if err != nil {
				require.Truef(t, isCancellation(err), "ADS stream ended with %v, not the test's cancellation", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("ADS stream did not stop after test cancellation")
		}
	})
	return h
}

// A snapshot published between a retry's version check and its replacement of
// the parked watch could be one the cache cannot answer: the cache would keep
// the parked watch, the retry would cancel it, and the cache would drop the
// replacement. Publication waits for the retry instead.
func TestParkedNackRetryHoldsPublicationOffUntilItReplacesTheWatch(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "ordered"}[ordered], func(t *testing.T) {
			shortenNackRetry(t, 100*time.Millisecond, time.Second)
			gate := &snapshotReadGate{
				SnapshotCache: adsOnlyCache(),
				read:          make(chan struct{}),
				resume:        make(chan struct{}),
			}
			h := newNackHarnessOver(t, ordered, gate)
			t.Cleanup(gate.release)
			parkNamedEndpointNack(t, h)

			gate.arm()
			select {
			case <-gate.read:
			case <-time.After(2 * time.Second):
				t.Fatal("the retry did not check the snapshot")
			}
			blocked := edsOnlySnapshot(t, "2", adsTestCluster, "unrequested")
			published := make(chan error, 1)
			go func() {
				published <- h.cache.SetSnapshot(h.ctx, adsTestNodeID, blocked)
			}()
			select {
			case <-published:
				t.Fatal("a snapshot was published between the retry's version check and its replacement")
			case <-time.After(100 * time.Millisecond):
			}

			gate.release()
			h.receiveType(envoyresource.EndpointType, "1")
			select {
			case err := <-published:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				t.Fatal("the publication did not proceed after the retry")
			}
		})
	}
}

// Parking a NACK checks the snapshot version and then registers the parked
// watch. A snapshot published in between that the cache cannot answer the
// parked request at would have the cache drop the request, leaving the stream
// with no endpoint watch. Publication waits for the parking instead, so the
// watch is registered, kept across the blocked snapshot, and answered later.
func TestNackParkingHoldsPublicationOffUntilTheWatchIsRegistered(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "ordered"}[ordered], func(t *testing.T) {
			shortenNackRetry(t, time.Minute, time.Minute)
			gate := &snapshotReadGate{
				SnapshotCache: adsOnlyCache(),
				read:          make(chan struct{}),
				resume:        make(chan struct{}),
			}
			h := newNackHarnessOver(t, ordered, gate)
			t.Cleanup(gate.release)
			h.setSnapshot(edsOnlySnapshot(t, "1", adsTestCluster))
			h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}})
			first := h.receiveType(envoyresource.EndpointType, "1")

			gate.arm()
			h.send(&discoveryv3.DiscoveryRequest{
				Node:          h.node,
				TypeUrl:       envoyresource.EndpointType,
				ResourceNames: []string{adsTestCluster},
				ResponseNonce: first.GetNonce(),
				ErrorDetail:   &status.Status{Message: "rejected by test"},
			})
			select {
			case <-gate.read:
			case <-time.After(2 * time.Second):
				t.Fatal("parking did not check the snapshot")
			}
			blocked := edsOnlySnapshot(t, "2", adsTestCluster, "unrequested")
			published := make(chan error, 1)
			go func() {
				published <- h.cache.SetSnapshot(h.ctx, adsTestNodeID, blocked)
			}()
			select {
			case <-published:
				t.Fatal("a snapshot was published between parking's version check and the watch's registration")
			case <-time.After(100 * time.Millisecond):
			}

			gate.release()
			select {
			case err := <-published:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				t.Fatal("the publication did not proceed after parking")
			}
			h.waitForOpenWatches(1)
			h.setSnapshot(edsOnlySnapshot(t, "3", adsTestCluster))
			h.receiveType(envoyresource.EndpointType, "3")
		})
	}
}

// The stream never waits on a publication: a NACK that arrives while one holds
// the publication lock is handled as the plain cache would, and re-sent.
func TestNackDuringPublicationIsAnsweredLikeThePlainCache(t *testing.T) {
	h := newPolicyHarness(t, true, true, true)
	h.setSnapshot(nackSnapshot("1"))
	h.subscribe(envoyresource.ClusterType)
	first := h.receiveType(envoyresource.ClusterType, "1")

	policy := h.cache.(*watchPolicy)
	policy.publish.Lock()
	h.nack(first, "")
	again := h.receiveType(envoyresource.ClusterType, "1")
	policy.publish.Unlock()
	require.NotEqual(t, first.GetNonce(), again.GetNonce(), "the NACK is answered with a new response")
}

// A request that omits Node shares the stream's Node, whose metadata the
// unique-client callback rewrites on every request. The retry must read its own
// copy; under -race this fails if it reads the shared one.
func TestParkedNackRetryReadsItsOwnNode(t *testing.T) {
	shortenNackRetry(t, time.Millisecond, time.Millisecond)
	hasher := kgwxds.NewNodeRoleHasher()
	policy := newWatchPolicy(envoycache.NewSnapshotCache(true, hasher, nil), hasher, true, true)
	node := adsTestNode(t)
	require.NoError(t, policy.SetSnapshot(t.Context(), hasher.ID(node), nackSnapshot("1")))

	// CDS with no names is a wildcard subscription, as the server makes it. The
	// stream has been sent version 1, so the parked watch cannot be answered and
	// only the retry can respond.
	sub := stream.NewSotwSubscription(nil, true)
	initial := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.ClusterType}
	require.NoError(t, policy.onStreamRequest(1, initial))
	sent := make(chan envoycache.Response, 1)
	_, err := policy.CreateWatch(initial, &sub, sent)
	require.NoError(t, err)
	sub.SetReturnedResources(awaitResponse(t, sent, "the first CDS response").GetReturnedResources())
	policy.onStreamResponse(t.Context(), 1, initial, &discoveryv3.DiscoveryResponse{TypeUrl: envoyresource.ClusterType, Nonce: "1", VersionInfo: "1"})
	nack := nackRequest(node, "1")
	require.NoError(t, policy.onStreamRequest(1, nack))

	ch := make(chan envoycache.Response, 1)
	cancel, err := policy.CreateWatch(nack, &sub, ch)
	require.NoError(t, err)
	t.Cleanup(cancel)

	role := node.GetMetadata().GetFields()[kgwxds.RoleKey]
	deadline := time.After(2 * time.Second)
	for {
		select {
		case resp := <-ch:
			require.Equal(t, "1", resp.GetResponseVersion(), "the retry re-sends the rejected version")
			return
		case <-deadline:
			t.Fatal("the parked NACK was not retried")
		default:
			node.GetMetadata().GetFields()[kgwxds.RoleKey] = role
		}
	}
}
