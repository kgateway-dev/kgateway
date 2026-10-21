package setup

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	serverconfig "github.com/envoyproxy/go-control-plane/pkg/server/config"
	sotwv3 "github.com/envoyproxy/go-control-plane/pkg/server/sotw/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"

	kgwxds "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
)

// newNackHarness is the ADS harness with the NACK resend suppressor in front
// of the cache and its callbacks on the server, as NewControlPlane wires them.
func newNackHarness(t *testing.T, ordered bool) *adsHarness {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockAdsStream{
		ctx:  ctx,
		sent: make(chan *discoveryv3.DiscoveryResponse, 16),
		recv: make(chan *discoveryv3.DiscoveryRequest, 16),
	}
	hasher := kgwxds.NewNodeRoleHasher()
	suppressor := newNackResendSuppressor(envoycache.NewSnapshotCache(true, hasher, nil), hasher)

	var opts []serverconfig.XDSOption
	if ordered {
		opts = append(opts, sotwv3.WithOrderedADS())
	}
	srv := xdsserver.NewServer(ctx, suppressor, suppressor.callbacks(), opts...)

	h := &adsHarness{
		t:      t,
		ctx:    ctx,
		cancel: cancel,
		cache:  suppressor,
		stream: stream,
		done:   make(chan error, 1),
		node:   adsTestNode(t),
	}
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

// nack sends a NACK of resp: accepted as the held version, resp's nonce, and an
// error detail, which is what marks a NACK on the wire.
func (h *adsHarness) nack(resp *discoveryv3.DiscoveryResponse, accepted string) {
	h.t.Helper()

	h.send(&discoveryv3.DiscoveryRequest{
		Node:          h.node,
		TypeUrl:       resp.GetTypeUrl(),
		VersionInfo:   accepted,
		ResponseNonce: resp.GetNonce(),
		ErrorDetail:   &status.Status{Message: "rejected by test"},
	})
}

func (h *adsHarness) expectSilence(reason string) {
	h.t.Helper()

	select {
	case resp := <-h.stream.sent:
		h.t.Fatalf("%s: unexpected %s response at version %q", reason, resp.GetTypeUrl(), resp.GetVersionInfo())
	case err := <-h.done:
		require.NoError(h.t, err)
	case <-time.After(300 * time.Millisecond):
	}
}

// The plain cache re-sends the rejected response on every NACK, because the
// NACK's version differs from the snapshot's.
func TestPlainCacheResendsRejectedResponseOnEveryNack(t *testing.T) {
	h := newADSHarness(t, true)
	h.setSnapshot(nackSnapshot("1"))
	h.subscribe(envoyresource.ClusterType)
	first := h.receiveType(envoyresource.ClusterType, "1")

	for range 3 {
		h.nack(first, "")
		first = h.receiveType(envoyresource.ClusterType, "1")
	}
}

func TestNackOfCurrentSnapshotIsNotResent(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "ordered"}[ordered], func(t *testing.T) {
			h := newNackHarness(t, ordered)
			h.setSnapshot(nackSnapshot("1"))
			h.subscribe(envoyresource.ClusterType)
			first := h.receiveType(envoyresource.ClusterType, "1")

			// The NACK parks; nothing is re-sent.
			h.nack(first, "")
			h.expectSilence("NACK of the current version")
			h.waitForOpenWatches(1)

			// A second NACK (Envoy retrying on its own) parks the same way.
			h.nack(first, "")
			h.expectSilence("repeated NACK of the current version")

			// A snapshot change delivers the correction.
			h.setSnapshot(nackSnapshot("2"))
			second := h.receiveType(envoyresource.ClusterType, "2")

			// After an ACK the stream behaves normally.
			h.ack(second)
			h.waitForOpenWatches(1)
			h.setSnapshot(nackSnapshot("3"))
			h.receiveType(envoyresource.ClusterType, "3")
		})
	}
}

func TestNackAfterSnapshotChangedIsAnsweredImmediately(t *testing.T) {
	h := newNackHarness(t, true)
	h.setSnapshot(nackSnapshot("1"))
	h.subscribe(envoyresource.ClusterType)
	first := h.receiveType(envoyresource.ClusterType, "1")

	// The snapshot moves before the client's rejection of version 1 arrives.
	h.setSnapshot(nackSnapshot("2"))
	h.nack(first, "")
	h.receiveType(envoyresource.ClusterType, "2")
}

func TestNackSuppressionLeavesAcksUntouched(t *testing.T) {
	h := newNackHarness(t, true)
	h.setSnapshot(nackSnapshot("1"))
	h.subscribeAndAck(envoyresource.ClusterType, "1", 1)
	h.setSnapshot(nackSnapshot("2"))
	h.receiveType(envoyresource.ClusterType, "2")
}

func TestNackOfNamedTypeIsNotResent(t *testing.T) {
	h := newNackHarness(t, true)
	h.setSnapshot(nackSnapshot("1"))
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}})
	first := h.receiveType(envoyresource.EndpointType, "1")

	h.send(&discoveryv3.DiscoveryRequest{
		Node:          h.node,
		TypeUrl:       envoyresource.EndpointType,
		ResourceNames: []string{adsTestCluster},
		ResponseNonce: first.GetNonce(),
		ErrorDetail:   &status.Status{Message: "rejected by test"},
	})
	h.expectSilence("NACK of the current EDS version")
	h.waitForOpenWatches(1)

	h.setSnapshot(nackSnapshot("2"))
	h.receiveType(envoyresource.EndpointType, "2")
}

// The client holds v1 and rejects v2: no flood of v2, and v3 still arrives.
func TestNackOfSecondVersionParksUntilThird(t *testing.T) {
	h := newNackHarness(t, true)
	h.setSnapshot(nackSnapshot("1"))
	h.subscribeAndAck(envoyresource.ClusterType, "1", 1)

	h.setSnapshot(nackSnapshot("2"))
	second := h.receiveType(envoyresource.ClusterType, "2")
	h.nack(second, "1")
	h.expectSilence("NACK of version 2 while the snapshot is still 2")
	h.waitForOpenWatches(1)

	h.setSnapshot(nackSnapshot("3"))
	h.receiveType(envoyresource.ClusterType, "3")
}

// nackSnapshot is one cluster, its endpoints and one route, all at version.
func nackSnapshot(version string) *envoycache.Snapshot {
	return snapshotFor(allADSVersions(version),
		[]*envoyclusterv3.Cluster{testCluster(adsTestCluster)},
		[]*envoyroutev3.RouteConfiguration{testRouteConfig(adsTestRoute, adsTestCluster)})
}

// shortenNackRetry makes the parked-NACK retry fire quickly. Tests using it
// must not run in parallel.
func shortenNackRetry(t *testing.T, base, maximum time.Duration) {
	t.Helper()
	origBase, origMax := nackRetryBase, nackRetryMax
	nackRetryBase, nackRetryMax = base, maximum
	t.Cleanup(func() { nackRetryBase, nackRetryMax = origBase, origMax })
}

// With the snapshot unchanged, a parked NACK is re-sent after the retry delay so
// a transient rejection recovers. A snapshot change still answers at once.
func TestParkedNackIsRetried(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "ordered"}[ordered], func(t *testing.T) {
			shortenNackRetry(t, 100*time.Millisecond, time.Second)
			h := newNackHarness(t, ordered)
			h.setSnapshot(nackSnapshot("1"))
			h.subscribe(envoyresource.ClusterType)
			first := h.receiveType(envoyresource.ClusterType, "1")

			h.nack(first, "")
			again := h.receiveType(envoyresource.ClusterType, "1")
			require.NotEqual(t, first.GetNonce(), again.GetNonce(), "the retry is a new response")

			// Rejected again: parked again, and a correction still goes out at once.
			h.nack(again, "")
			h.waitForOpenWatches(1)
			h.setSnapshot(nackSnapshot("2"))
			h.receiveType(envoyresource.ClusterType, "2")
		})
	}
}

// Rejections of one version double the delay up to the maximum, bounding the
// resend rate. A different version or an ACK resets it.
func TestNackRetryDelayBacksOff(t *testing.T) {
	shortenNackRetry(t, time.Second, 10*time.Second)
	require.Equal(t, time.Second, nackRetryDelay(1))
	require.Equal(t, 2*time.Second, nackRetryDelay(2))
	require.Equal(t, 8*time.Second, nackRetryDelay(4))
	require.Equal(t, 10*time.Second, nackRetryDelay(5))
	require.Equal(t, 10*time.Second, nackRetryDelay(1000))

	policy := newNackResendSuppressor(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), envoycache.IDHash{})
	node := &envoycorev3.Node{Id: "node"}
	respond := func(nonce, version string) {
		policy.onStreamResponse(t.Context(), 1, &discoveryv3.DiscoveryRequest{Node: node}, &discoveryv3.DiscoveryResponse{TypeUrl: envoyresource.ClusterType, Nonce: nonce, VersionInfo: version})
	}
	reject := func(nonce string) time.Duration {
		t.Helper()
		req := nackRequest(node, nonce)
		require.NoError(t, policy.onStreamRequest(1, req))
		_, retryIn, ok := policy.rejectedVersion(req)
		require.True(t, ok)
		return retryIn
	}
	require.NoError(t, policy.onStreamRequest(1, &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.ClusterType}))
	respond("1", "bad")
	require.Equal(t, time.Second, reject("1"))
	respond("2", "bad")
	require.Equal(t, 2*time.Second, reject("2"), "the same version rejected again backs off")
	respond("3", "also-bad")
	require.Equal(t, time.Second, reject("3"), "a different version starts over")
	require.NoError(t, policy.onStreamRequest(1, &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.ClusterType, VersionInfo: "also-bad", ResponseNonce: "3"}))
	respond("4", "also-bad")
	require.Equal(t, time.Second, reject("4"), "an ACK starts over")
}
