package setup

import (
	"context"
	"strconv"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	serverconfig "github.com/envoyproxy/go-control-plane/pkg/server/config"
	sotwv3 "github.com/envoyproxy/go-control-plane/pkg/server/sotw/v3"
	streamv3 "github.com/envoyproxy/go-control-plane/pkg/server/stream/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
)

// These tests deliberately use the pinned upstream server/cache plus the
// decorator from #14701. No local cache or SotW implementation is involved.
func TestNackSharedKeyKeepsStreamVersionsSeparate(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(strconv.FormatBool(ordered), func(t *testing.T) {
			ctx := t.Context()
			hasher := xds.NewNodeRoleHasher()
			policy := newWatchPolicy(envoycache.NewSnapshotCache(true, hasher, nil), hasher, true, true)
			var opts []serverconfig.XDSOption
			if ordered {
				opts = append(opts, sotwv3.WithOrderedADS())
			}
			originals := make(chan string, 8)
			observer := xdsserver.CallbackFuncs{StreamRequestFunc: func(_ int64, req *discoveryv3.DiscoveryRequest) error {
				if req.ErrorDetail != nil {
					originals <- req.VersionInfo
				}
				return nil
			}}
			srv := xdsserver.NewServer(ctx, policy, chainCallbacks(policy.callbacks(), observer), opts...)
			attach := func() *adsHarness {
				streamCtx, cancel := context.WithCancel(ctx)
				h := &adsHarness{t: t, ctx: streamCtx, cancel: cancel, cache: policy, node: adsTestNode(t), done: make(chan error, 1), stream: &mockAdsStream{ctx: streamCtx, sent: make(chan *discoveryv3.DiscoveryResponse, 16), recv: make(chan *discoveryv3.DiscoveryRequest, 16)}}
				go func() { h.done <- srv.StreamAggregatedResources(h.stream) }()
				t.Cleanup(func() {
					cancel()
					select {
					case <-h.done:
					case <-time.After(2 * time.Second):
						t.Error("stream did not stop")
					}
				})
				return h
			}
			a, b := attach(), attach()
			a.setSnapshot(nackSnapshot("1"))
			a.subscribe(envoyresource.ClusterType)
			a1 := a.receiveType(envoyresource.ClusterType, "1")
			b.setSnapshot(nackSnapshot("2"))
			b.subscribe(envoyresource.ClusterType)
			b2 := b.receiveType(envoyresource.ClusterType, "2")
			// Both servers' streams start their nonce counter at 1, so node/type/
			// nonce alone cannot distinguish which version was actually rejected.
			require.Equal(t, a1.Nonce, b2.Nonce)
			b.nack(b2, "")
			b.expectSilence("B rejected v2")
			a.nack(a1, "")
			a2 := a.receiveType(envoyresource.ClusterType, "2")
			a.nack(a2, "")
			a.expectSilence("A has now rejected v2 too")
			for range 3 {
				select {
				case original := <-originals:
					require.Empty(t, original)
				case <-time.After(time.Second):
					t.Fatal("missing NACK callback")
				}
			}
			a.setSnapshot(nackSnapshot("3"))
			a.receiveType(envoyresource.ClusterType, "3")
			b.receiveType(envoyresource.ClusterType, "3")
		})
	}
}

func nackRequest(node *envoycorev3.Node, nonce string) *discoveryv3.DiscoveryRequest {
	return &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.ClusterType, ResponseNonce: nonce, VersionInfo: "accepted", ErrorDetail: &status.Status{Message: "rejected"}}
}

func TestNackCorrelationRejectsUnrelatedRequests(t *testing.T) {
	for _, scenario := range []string{"matching", "stale nonce", "missing nonce", "different type", "different node", "ACK", "unassociated clone"} {
		t.Run(scenario, func(t *testing.T) {
			policy := newWatchPolicy(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), envoycache.IDHash{}, true, true)
			node := &envoycorev3.Node{Id: "node"}
			initial := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.ClusterType}
			require.NoError(t, policy.onStreamRequest(1, initial))
			policy.onStreamResponse(t.Context(), 1, initial, &discoveryv3.DiscoveryResponse{TypeUrl: envoyresource.ClusterType, Nonce: "2", VersionInfo: "rejected"})
			req := nackRequest(node, "2")
			switch scenario {
			case "stale nonce":
				req.ResponseNonce = "1"
			case "missing nonce":
				req.ResponseNonce = ""
			case "different type":
				req.TypeUrl = envoyresource.EndpointType
			case "different node":
				req.Node = &envoycorev3.Node{Id: "other"}
			case "ACK":
				req.ErrorDetail = nil
			}
			original := proto.Clone(req)
			require.NoError(t, policy.onStreamRequest(1, req))
			if scenario == "unassociated clone" {
				req = proto.Clone(req).(*discoveryv3.DiscoveryRequest)
			}
			version, ok := policy.rejectedVersion(req)
			require.Equal(t, scenario == "matching", ok)
			if ok {
				require.Equal(t, "rejected", version)
			}
			require.True(t, proto.Equal(original, req), "callbacks must not rewrite the request")
			policy.onStreamClosed(1, node)
			require.Empty(t, policy.streams)
			require.Empty(t, policy.pending)
		})
	}
}

func TestNackRequestAssociationsAreBoundedAndReleased(t *testing.T) {
	policy := newWatchPolicy(envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil), envoycache.IDHash{}, true, true)
	node := &envoycorev3.Node{Id: "shared"}
	for _, id := range []int64{1, 2} {
		initial := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.ClusterType}
		require.NoError(t, policy.onStreamRequest(id, initial))
		policy.onStreamResponse(t.Context(), id, initial, &discoveryv3.DiscoveryResponse{TypeUrl: envoyresource.ClusterType, Nonce: "1", VersionInfo: "v"})
	}
	var first, last *discoveryv3.DiscoveryRequest
	for range 100 {
		last = nackRequest(node, "1")
		if first == nil {
			first = last
		}
		require.NoError(t, policy.onStreamRequest(1, last))
		require.Len(t, policy.pending, 1, "requests skipped before CreateWatch must not accumulate")
	}
	_, ok := policy.rejectedVersion(first)
	require.False(t, ok)
	second := nackRequest(node, "1")
	require.NoError(t, policy.onStreamRequest(2, second))
	policy.onStreamClosed(1, node)
	require.Len(t, policy.streams, 1)
	require.Len(t, policy.pending, 1)
	version, ok := policy.rejectedVersion(second)
	require.True(t, ok)
	require.Equal(t, "v", version)
	require.Empty(t, policy.pending)
	require.Nil(t, policy.streams[2].pending)
	policy.onStreamClosed(2, node)
	require.Empty(t, policy.streams)
}

type nackObservingCache struct {
	envoycache.SnapshotCache
	request *envoycache.Request
}

func (c *nackObservingCache) CreateWatch(req *envoycache.Request, _ envoycache.Subscription, _ chan envoycache.Response) (func(), error) {
	c.request = req
	return func() {}, nil
}

func TestNackCacheReceivesCopyAndConsumesAssociation(t *testing.T) {
	inner := &nackObservingCache{SnapshotCache: envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil)}
	policy := newWatchPolicy(inner, envoycache.IDHash{}, true, true)
	node := &envoycorev3.Node{Id: "node"}
	require.NoError(t, inner.SetSnapshot(t.Context(), node.Id, nackSnapshot("rejected")))
	initial := &discoveryv3.DiscoveryRequest{Node: node, TypeUrl: envoyresource.ClusterType}
	require.NoError(t, policy.onStreamRequest(1, initial))
	policy.onStreamResponse(t.Context(), 1, initial, &discoveryv3.DiscoveryResponse{TypeUrl: envoyresource.ClusterType, Nonce: "1", VersionInfo: "rejected"})
	req := nackRequest(node, "1")
	original := proto.Clone(req)
	require.NoError(t, policy.onStreamRequest(1, req))
	cancel, err := policy.CreateWatch(req, streamv3.NewSotwSubscription(nil, true), make(chan envoycache.Response, 1))
	require.NoError(t, err)
	cancel()
	require.NotSame(t, req, inner.request)
	require.Equal(t, "rejected", inner.request.VersionInfo)
	require.True(t, proto.Equal(original, req))
	require.Empty(t, policy.pending)
	require.Nil(t, policy.streams[1].pending)
}
