package setup

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
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

// newPolicyHarness is the ADS harness with the watch policy in front of the
// cache and its callbacks on the server, as NewControlPlane wires them.
func newPolicyHarness(t *testing.T, ordered, suppressNackResend, respondOnReconnect bool) *adsHarness {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockAdsStream{
		ctx:  ctx,
		sent: make(chan *discoveryv3.DiscoveryResponse, 16),
		recv: make(chan *discoveryv3.DiscoveryRequest, 16),
	}
	hasher := kgwxds.NewNodeRoleHasher()
	policy := newWatchPolicy(envoycache.NewSnapshotCache(true, hasher, nil), hasher, suppressNackResend, respondOnReconnect)

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
				require.ErrorIs(t, err, context.Canceled)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("ADS stream did not stop after test cancellation")
		}
	})
	return h
}

// nack rejects resp: the request carries the version the client still holds
// (accepted, the empty string here), the rejected response's nonce, and an
// error detail, which is what distinguishes a NACK from an ACK on the wire.
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

// ackNamed acknowledges a named-type response. An ACK without names after a
// named subscription is an unsubscribe-all on the pinned cache and parks no
// watch, so named types must repeat their names.
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

// Without the suppressor the cache answers every NACK with the rejected
// response again: the client is on its old version, the snapshot is the
// rejected one, the versions differ, so the cache responds. This pins the loop
// the suppressor exists for, using the plain harness.
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

			// The NACK parks: nothing is re-sent while the snapshot is unchanged.
			h.nack(first, "")
			h.expectSilence("NACK of the current version")
			h.waitForOpenWatches(1)

			// A second NACK (Envoy retrying on its own) parks the same way.
			h.nack(first, "")
			h.expectSilence("repeated NACK of the current version")

			// The correction is delivered as soon as the snapshot changes.
			h.setSnapshot(nackSnapshot("2"))
			second := h.receiveType(envoyresource.ClusterType, "2")

			// And an ACK of the correction leaves the stream in its normal state.
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

// A rejection after an accepted version: the client holds version 1, rejects
// version 2, and must not be flooded with version 2 again; version 3 reaches it.
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

// nackSnapshot is the harness snapshot (one cluster, its endpoints, one route)
// with every type at version.
func nackSnapshot(version string) *envoycache.Snapshot {
	return snapshotFor(allADSVersions(version),
		[]*envoyclusterv3.Cluster{testCluster(adsTestCluster)},
		[]*envoyroutev3.RouteConfiguration{testRouteConfig(adsTestRoute, adsTestCluster)})
}

// reconnectSnapshot is the harness snapshot with a real endpoint assignment for
// the cluster, so named EDS requests have something to be answered with.
func reconnectSnapshot(version string) *envoycache.Snapshot {
	snap := nackSnapshot(version)
	snap.Resources[cachetypes.Endpoint] = envoycache.NewResources(version, []cachetypes.Resource{
		&envoyendpointv3.ClusterLoadAssignment{ClusterName: adsTestCluster},
	})
	return snap
}

// A proxy reconnecting after a control-plane restart: its first endpoint
// request names the version it accepted on the old stream, the cache is still
// empty, and the control plane then republishes the same content under the
// same version. Without the policy the parked request is never answered
// (versions are equal), so a cluster that was warming stays warming. This pins
// that on the plain harness.
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

			// From here the stream behaves normally: the ACK parks, the next
			// version is delivered, and a NACK of it is not re-sent.
			h.ackNamed(resp, adsTestCluster)
			h.waitForOpenWatches(1)
			h.setSnapshot(reconnectSnapshot("2"))
			second := h.receiveType(envoyresource.EndpointType, "2")
			h.nack(second, "1")
			h.expectSilence("NACK of version 2 after a reconnect")
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

// Against a warm cache the pinned cache answers a fresh stream's first request
// of any type at the held version by itself, because nothing has been
// returned on that stream yet. The reconnect rule is therefore only needed for
// the empty-cache path pinned above; this documents the warm-cache behavior on
// the plain harness so a change in the pin is noticed.
func TestPlainCacheAnswersFreshStreamAtHeldVersionFromWarmCache(t *testing.T) {
	h := newADSHarness(t, true)
	h.setSnapshot(reconnectSnapshot("1"))
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.ClusterType, VersionInfo: "1"})
	h.receiveType(envoyresource.ClusterType, "1")
	h.send(&discoveryv3.DiscoveryRequest{Node: h.node, TypeUrl: envoyresource.EndpointType, ResourceNames: []string{adsTestCluster}, VersionInfo: "1"})
	h.receiveType(envoyresource.EndpointType, "1")
}

// Other types keep the cache's behavior on the empty-cache path: a
// reconnecting proxy's cluster request parks, an equal-version republish does
// not answer it, which is harmless since the proxy holds that version, and the
// next version is delivered.
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
