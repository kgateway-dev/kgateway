package setup

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	serverconfig "github.com/envoyproxy/go-control-plane/pkg/server/config"
	sotwv3 "github.com/envoyproxy/go-control-plane/pkg/server/sotw/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"

	kgwxds "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
)

// newPolicyHarness mirrors NewControlPlane's watch-policy and callback wiring,
// secrets cache included.
func newPolicyHarness(t *testing.T, ordered, suppressNackResend, respondOnReconnect bool) *adsHarness {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockAdsStream{
		ctx:  ctx,
		sent: make(chan *discoveryv3.DiscoveryResponse, 16),
		recv: make(chan *discoveryv3.DiscoveryRequest, 16),
	}
	hasher := kgwxds.NewNodeRoleHasher()
	policy := newWatchPolicy(newSnapshotCache(hasher, nil), hasher, suppressNackResend, respondOnReconnect)

	var opts []serverconfig.XDSOption
	if ordered {
		opts = append(opts, sotwv3.WithOrderedADS())
	}
	srv := xdsserver.NewServer(ctx, policy, policy.callbacks(), opts...)

	h := &adsHarness{
		t:      t,
		ctx:    ctx,
		cancel: cancel,
		cache:  policy,
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

// openWatches counts node's open watches in cache and the cache it wraps.
// newSnapshotCache's status already sums its per-type caches. ok is false
// until one of them has seen the node.
func openWatches(cache envoycache.SnapshotCache, node string) (n int, ok bool) {
	if c, isPolicy := cache.(*watchPolicy); isPolicy {
		return openWatches(c.SnapshotCache, node)
	}
	info := cache.GetStatusInfo(node)
	if info == nil {
		return 0, false
	}
	return info.GetNumWatches(), true
}

// nack sends a NACK of resp: accepted as the held version, resp's nonce, and an
// error detail, which is what marks a NACK on the wire. A named type passes
// the names it still subscribes to.
func (h *adsHarness) nack(resp *discoveryv3.DiscoveryResponse, accepted string, names ...string) {
	h.t.Helper()

	h.send(&discoveryv3.DiscoveryRequest{
		Node:          h.node,
		TypeUrl:       resp.GetTypeUrl(),
		ResourceNames: names,
		VersionInfo:   accepted,
		ResponseNonce: resp.GetNonce(),
		ErrorDetail:   &status.Status{Message: "rejected by test"},
	})
}

// ackNamed ACKs a named-type response, repeating the names: on the pinned cache
// an ACK without names unsubscribes from all.
func (h *adsHarness) ackNamed(resp *discoveryv3.DiscoveryResponse, names ...string) {
	h.t.Helper()

	h.send(&discoveryv3.DiscoveryRequest{
		Node:          h.node,
		TypeUrl:       resp.GetTypeUrl(),
		ResourceNames: names,
		VersionInfo:   resp.GetVersionInfo(),
		ResponseNonce: resp.GetNonce(),
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
			h := newPolicyHarness(t, ordered, true, true)
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
	h := newPolicyHarness(t, true, true, true)
	h.setSnapshot(nackSnapshot("1"))
	h.subscribe(envoyresource.ClusterType)
	first := h.receiveType(envoyresource.ClusterType, "1")

	// The snapshot moves before the client's rejection of version 1 arrives.
	h.setSnapshot(nackSnapshot("2"))
	h.nack(first, "")
	h.receiveType(envoyresource.ClusterType, "2")
}

func TestNackSuppressionLeavesAcksUntouched(t *testing.T) {
	h := newPolicyHarness(t, true, true, true)
	h.setSnapshot(nackSnapshot("1"))
	h.subscribeAndAck(envoyresource.ClusterType, "1", 1)
	h.setSnapshot(nackSnapshot("2"))
	h.receiveType(envoyresource.ClusterType, "2")
}

func TestNackOfNamedTypeIsNotResent(t *testing.T) {
	h := newPolicyHarness(t, true, true, true)
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
	h := newPolicyHarness(t, true, true, true)
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

// reconnectSnapshot includes a real CLA for named EDS requests.
func reconnectSnapshot(version string) *envoycache.Snapshot {
	snap := nackSnapshot(version)
	snap.Resources[cachetypes.Endpoint] = envoycache.NewResources(version, []cachetypes.Resource{
		&envoyendpointv3.ClusterLoadAssignment{ClusterName: adsTestCluster},
	})
	return snap
}

const adsTestSecret = "test-secret"

// reconnectSnapshotWithSecret adds a secret for named SDS requests.
func reconnectSnapshotWithSecret(version string) *envoycache.Snapshot {
	snap := reconnectSnapshot(version)
	snap.Resources[cachetypes.Secret] = envoycache.NewResources(version, []cachetypes.Resource{
		&envoytlsv3.Secret{Name: adsTestSecret},
	})
	return snap
}

// The named types a resource warms on, with the name the harness requests.
var reconnectWarmedTypes = []struct{ typeURL, name string }{
	{envoyresource.EndpointType, adsTestCluster},
	{envoyresource.RouteType, adsTestRoute},
	{envoyresource.SecretType, adsTestSecret},
}

// On the plain cache, a held-version EDS request parked on an empty cache is not
// answered by an equal-version republish, only by a new version. A warming
// cluster would wait for its fetch timeout.
func TestPlainCacheLeavesReconnectEndpointRequestParkedOnEqualVersionRepublish(t *testing.T) {
	h := newADSHarness(t, true)
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}, VersionInfo: "1"})
	h.waitForOpenWatches(1)
	h.setSnapshot(reconnectSnapshot("1"))
	h.expectSilence("equal-version republish to a reconnecting proxy's parked endpoint request")
	h.setSnapshot(reconnectSnapshot("2"))
	h.receiveType(envoyresource.EndpointType, "2")
}

func TestReconnectEndpointRequestIsAnsweredWhenSnapshotArrivesAtHeldVersion(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "ordered"}[ordered], func(t *testing.T) {
			h := newPolicyHarness(t, ordered, true, true)
			h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}, VersionInfo: "1"})
			h.waitForOpenWatches(1)
			h.setSnapshot(reconnectSnapshot("1"))
			resp := h.receiveType(envoyresource.EndpointType, "1")
			require.Len(t, resp.GetResources(), 1)

			// Then the stream is normal: ACKs park, updates arrive, NACKs are not re-sent.
			h.ackNamed(resp, adsTestCluster)
			h.waitForOpenWatches(1)
			h.setSnapshot(reconnectSnapshot("2"))
			second := h.receiveType(envoyresource.EndpointType, "2")
			// The NACK keeps the subscription; without its names it would
			// unsubscribe, and the silence would prove nothing.
			h.nack(second, "1", adsTestCluster)
			h.expectSilence("NACK of version 2 after a reconnect")
			h.waitForOpenWatches(1)
			h.setSnapshot(reconnectSnapshot("3"))
			h.receiveType(envoyresource.EndpointType, "3")
		})
	}
}

func TestReconnectEndpointRequestIsAnsweredFromWarmCache(t *testing.T) {
	h := newPolicyHarness(t, true, true, true)
	h.setSnapshot(reconnectSnapshot("1"))
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}, VersionInfo: "1"})
	resp := h.receiveType(envoyresource.EndpointType, "1")
	require.Len(t, resp.GetResources(), 1)
}

// A warm pinned cache answers a fresh stream's held-version request itself,
// because nothing was returned on that stream yet, so only the empty-cache path
// needs the rule. Pinned so a go-control-plane change is noticed.
func TestPlainCacheAnswersFreshStreamAtHeldVersionFromWarmCache(t *testing.T) {
	h := newADSHarness(t, true)
	h.setSnapshot(reconnectSnapshot("1"))
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.ClusterType, VersionInfo: "1"})
	h.receiveType(envoyresource.ClusterType, "1")
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}, VersionInfo: "1"})
	h.receiveType(envoyresource.EndpointType, "1")
}

// CDS keeps the cache's behavior: an equal-version republish leaves the cluster
// request parked (harmless, the proxy holds that version); the next version
// answers.
func TestReconnectRuleLeavesClusterRequestsToTheCache(t *testing.T) {
	h := newPolicyHarness(t, true, true, true)
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.ClusterType, VersionInfo: "1"})
	h.waitForOpenWatches(1)
	h.setSnapshot(reconnectSnapshot("1"))
	h.expectSilence("equal-version republish to a parked cluster request")
	h.setSnapshot(reconnectSnapshot("2"))
	h.receiveType(envoyresource.ClusterType, "2")
}

// A cold proxy (no held version) and an ordinary ACK are untouched by the rule.
func TestReconnectRuleIgnoresColdRequestsAndAcks(t *testing.T) {
	h := newPolicyHarness(t, true, true, true)
	h.setSnapshot(reconnectSnapshot("1"))
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}})
	first := h.receiveType(envoyresource.EndpointType, "1")
	h.ackNamed(first, adsTestCluster)
	h.waitForOpenWatches(1)
	h.expectSilence("ACK at the current version")
	h.setSnapshot(reconnectSnapshot("2"))
	h.receiveType(envoyresource.EndpointType, "2")
}

// With the setting off the reconnect request parks exactly as on the plain cache.
func TestReconnectRuleDisabledParksLikePlainCache(t *testing.T) {
	h := newPolicyHarness(t, true, true, false)
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}, VersionInfo: "1"})
	h.waitForOpenWatches(1)
	h.setSnapshot(reconnectSnapshot("1"))
	h.expectSilence("equal-version republish with the reconnect rule disabled")
}

// shortenNackRetry speeds up the parked-NACK retry; callers must not run in parallel.
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
			h := newPolicyHarness(t, ordered, true, true)
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

	policy := newWatchPolicy(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), envoycache.IDHash{}, true, false)
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

// Route and secret requests park the same way on the plain cache; a warming
// listener or cluster would wait for its fetch timeout.
func TestPlainCacheLeavesReconnectRouteAndSecretRequestsParkedOnEqualVersionRepublish(t *testing.T) {
	for _, tc := range reconnectWarmedTypes[1:] {
		t.Run(tc.typeURL, func(t *testing.T) {
			h := newADSHarness(t, true)
			h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: tc.typeURL, ResourceNames: []string{tc.name}, VersionInfo: "1"})
			h.waitForOpenWatches(1)
			h.setSnapshot(reconnectSnapshotWithSecret("1"))
			h.expectSilence("equal-version republish to a reconnecting proxy's parked request")
		})
	}
}

// The policy answers each warmed-on type once a snapshot arrives at the held version.
func TestReconnectWarmedTypeRequestsAreAnsweredWhenSnapshotArrivesAtHeldVersion(t *testing.T) {
	for _, tc := range reconnectWarmedTypes {
		for _, ordered := range []bool{false, true} {
			t.Run(tc.typeURL+map[bool]string{false: "/default", true: "/ordered"}[ordered], func(t *testing.T) {
				h := newPolicyHarness(t, ordered, true, true)
				h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: tc.typeURL, ResourceNames: []string{tc.name}, VersionInfo: "1"})
				h.waitForOpenWatches(1)
				h.setSnapshot(reconnectSnapshotWithSecret("1"))
				resp := h.receiveType(tc.typeURL, "1")
				require.Len(t, resp.GetResources(), 1)
			})
		}
	}
}
