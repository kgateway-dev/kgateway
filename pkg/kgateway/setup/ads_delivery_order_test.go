package setup

import (
	"context"
	"io"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	serverconfig "github.com/envoyproxy/go-control-plane/pkg/server/config"
	sotwv3 "github.com/envoyproxy/go-control-plane/pkg/server/sotw/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	kgwxds "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
)

const (
	adsTestNodeID     = "kgateway-kube-gateway-api~default~ordered-ads-test"
	adsTestCluster    = "backend"
	adsTestNewCluster = "backend-v2"
	adsTestRoute      = "local_route"
)

type mockAdsStream struct {
	grpc.ServerStream

	ctx  context.Context
	sent chan *discoveryv3.DiscoveryResponse
	recv chan *discoveryv3.DiscoveryRequest
}

func (s *mockAdsStream) Context() context.Context {
	return s.ctx
}

func (s *mockAdsStream) Send(resp *discoveryv3.DiscoveryResponse) error {
	select {
	case s.sent <- resp:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *mockAdsStream) Recv() (*discoveryv3.DiscoveryRequest, error) {
	select {
	case req, ok := <-s.recv:
		if !ok {
			return nil, io.EOF
		}
		return req, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

type adsHarness struct {
	t      *testing.T
	ctx    context.Context
	cancel context.CancelFunc
	cache  envoycache.SnapshotCache
	stream *mockAdsStream
	done   chan error
	node   *envoycorev3.Node
}

func newADSHarness(t *testing.T, ordered bool) *adsHarness {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockAdsStream{
		ctx:  ctx,
		sent: make(chan *discoveryv3.DiscoveryResponse, 16),
		recv: make(chan *discoveryv3.DiscoveryRequest, 16),
	}
	cache := envoycache.NewSnapshotCache(true, kgwxds.NewNodeRoleHasher(), nil)

	var opts []serverconfig.XDSOption
	if ordered {
		opts = append(opts, sotwv3.WithOrderedADS())
	}
	srv := xdsserver.NewServer(ctx, cache, nil, opts...)

	h := &adsHarness{
		t:      t,
		ctx:    ctx,
		cancel: cancel,
		cache:  cache,
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

func adsTestNode(t *testing.T) *envoycorev3.Node {
	t.Helper()

	metadata, err := structpb.NewStruct(map[string]any{
		kgwxds.RoleKey: adsTestNodeID,
	})
	require.NoError(t, err)

	return &envoycorev3.Node{Id: adsTestNodeID, Metadata: metadata}
}

func (h *adsHarness) setSnapshot(snapshot envoycache.ResourceSnapshot) {
	h.t.Helper()

	require.NoError(h.t, h.cache.SetSnapshot(h.ctx, adsTestNodeID, snapshot))
}

func (h *adsHarness) subscribe(typeURL string) {
	h.t.Helper()

	h.send(&discoveryv3.DiscoveryRequest{
		Node:    h.node,
		TypeUrl: typeURL,
	})
}

func (h *adsHarness) ack(resp *discoveryv3.DiscoveryResponse) {
	h.t.Helper()

	h.send(&discoveryv3.DiscoveryRequest{
		Node:          h.node,
		TypeUrl:       resp.GetTypeUrl(),
		VersionInfo:   resp.GetVersionInfo(),
		ResponseNonce: resp.GetNonce(),
	})
}

func (h *adsHarness) send(req *discoveryv3.DiscoveryRequest) {
	h.t.Helper()

	select {
	case h.stream.recv <- req:
	case err := <-h.done:
		require.NoError(h.t, err)
	case <-time.After(2 * time.Second):
		h.t.Fatalf("timed out sending %s request", req.GetTypeUrl())
	}
}

func (h *adsHarness) receive() *discoveryv3.DiscoveryResponse {
	h.t.Helper()

	select {
	case resp := <-h.stream.sent:
		return resp
	case err := <-h.done:
		require.NoError(h.t, err)
		return nil
	case <-time.After(2 * time.Second):
		h.t.Fatal("timed out waiting for ADS response")
		return nil
	}
}

func (h *adsHarness) receiveType(typeURL, version string) *discoveryv3.DiscoveryResponse {
	h.t.Helper()

	resp := h.receive()
	require.Equal(h.t, typeURL, resp.GetTypeUrl())
	require.Equal(h.t, version, resp.GetVersionInfo())
	return resp
}

func (h *adsHarness) subscribeAndAck(typeURL, version string, wantOpenWatches int) *discoveryv3.DiscoveryResponse {
	h.t.Helper()

	h.subscribe(typeURL)
	resp := h.receiveType(typeURL, version)
	h.ack(resp)
	h.waitForOpenWatches(wantOpenWatches)
	return resp
}

func (h *adsHarness) waitForOpenWatches(count int) {
	h.t.Helper()

	require.Eventually(h.t, func() bool {
		info := h.cache.GetStatusInfo(adsTestNodeID)
		return info != nil && info.GetNumWatches() == count
	}, 2*time.Second, 10*time.Millisecond)
}

func TestADSQuietStreamAdditionDeliversClusterBeforeRoute(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newADSHarness(t, ordered)
			h.setSnapshot(snapshotFor(allADSVersions("1"), nil, nil))

			h.subscribeAndAck(resource.ClusterType, "1", 1)
			h.subscribeAndAck(resource.EndpointType, "1", 2)
			h.subscribeAndAck(resource.ListenerType, "1", 3)

			// Hold the initial RDS ACK so the route watch is closed when the
			// addition snapshot lands. That keeps the stream quiet: only CDS can
			// answer first, and RDS is requested after that response is observed.
			h.subscribe(resource.RouteType)
			rdsV1 := h.receiveType(resource.RouteType, "1")
			h.waitForOpenWatches(3)

			h.setSnapshot(snapshotFor(
				adsVersions{cluster: "2", endpoint: "1", listener: "1", route: "2"},
				[]*envoyclusterv3.Cluster{testCluster(adsTestCluster)},
				[]*envoyroutev3.RouteConfiguration{testRouteConfig(adsTestRoute, adsTestCluster)},
			))

			cdsV2 := h.receiveType(resource.ClusterType, "2")
			h.ack(cdsV2)
			h.waitForOpenWatches(3)

			h.ack(rdsV1)
			h.receiveType(resource.RouteType, "2")
		})
	}
}

func TestADSAckSkewDeliversRouteBeforeClusterEvenWhenOrdered(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newADSHarness(t, ordered)
			h.setSnapshot(snapshotFor(
				allADSVersions("1"),
				[]*envoyclusterv3.Cluster{testCluster(adsTestCluster)},
				[]*envoyroutev3.RouteConfiguration{testRouteConfig(adsTestRoute, adsTestCluster)},
			))

			h.subscribeAndAck(resource.ClusterType, "1", 1)
			h.subscribeAndAck(resource.EndpointType, "1", 2)
			h.subscribeAndAck(resource.ListenerType, "1", 3)
			h.subscribeAndAck(resource.RouteType, "1", 4)

			h.setSnapshot(snapshotFor(
				adsVersions{cluster: "2", endpoint: "1", listener: "1", route: "1"},
				[]*envoyclusterv3.Cluster{
					testCluster(adsTestCluster),
					testCluster(adsTestNewCluster),
				},
				[]*envoyroutev3.RouteConfiguration{testRouteConfig(adsTestRoute, adsTestCluster)},
			))
			cdsV2 := h.receiveType(resource.ClusterType, "2")

			h.setSnapshot(snapshotFor(
				adsVersions{cluster: "3", endpoint: "1", listener: "1", route: "3"},
				[]*envoyclusterv3.Cluster{
					testCluster(adsTestCluster),
					testCluster(adsTestNewCluster),
				},
				[]*envoyroutev3.RouteConfiguration{testRouteConfig(adsTestRoute, adsTestNewCluster)},
			))
			h.receiveType(resource.RouteType, "3")

			h.ack(cdsV2)
			h.receiveType(resource.ClusterType, "3")
		})
	}
}

func TestADSCombinedRemovalDeliversClusterRemovalBeforeRouteDereferenceWhenOrdered(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(adsModeName(ordered), func(t *testing.T) {
			h := newADSHarness(t, ordered)
			h.setSnapshot(snapshotFor(
				allADSVersions("1"),
				[]*envoyclusterv3.Cluster{testCluster(adsTestCluster)},
				[]*envoyroutev3.RouteConfiguration{testRouteConfig(adsTestRoute, adsTestCluster)},
			))

			h.subscribeAndAck(resource.ClusterType, "1", 1)
			h.subscribeAndAck(resource.EndpointType, "1", 2)
			h.subscribeAndAck(resource.ListenerType, "1", 3)
			h.subscribeAndAck(resource.RouteType, "1", 4)

			h.setSnapshot(snapshotFor(
				adsVersions{cluster: "2", endpoint: "1", listener: "1", route: "2"},
				nil,
				nil,
			))

			got := []string{
				h.receive().GetTypeUrl(),
				h.receive().GetTypeUrl(),
			}
			if ordered {
				require.Equal(t, []string{resource.ClusterType, resource.RouteType}, got)
			} else {
				require.ElementsMatch(t, []string{resource.ClusterType, resource.RouteType}, got)
			}
		})
	}
}

type adsVersions struct {
	cluster  string
	endpoint string
	listener string
	route    string
}

func allADSVersions(version string) adsVersions {
	return adsVersions{
		cluster:  version,
		endpoint: version,
		listener: version,
		route:    version,
	}
}

func snapshotFor(versions adsVersions, clusters []*envoyclusterv3.Cluster, routes []*envoyroutev3.RouteConfiguration) *envoycache.Snapshot {
	snap := &envoycache.Snapshot{}
	snap.Resources[cachetypes.Cluster] = envoycache.NewResources(versions.cluster, clusterResources(clusters))
	snap.Resources[cachetypes.Endpoint] = envoycache.NewResources(versions.endpoint, nil)
	snap.Resources[cachetypes.Listener] = envoycache.NewResources(versions.listener, nil)
	snap.Resources[cachetypes.Route] = envoycache.NewResources(versions.route, routeResources(routes))
	return snap
}

func clusterResources(clusters []*envoyclusterv3.Cluster) []cachetypes.Resource {
	out := make([]cachetypes.Resource, 0, len(clusters))
	for _, cluster := range clusters {
		out = append(out, cluster)
	}
	return out
}

func routeResources(routes []*envoyroutev3.RouteConfiguration) []cachetypes.Resource {
	out := make([]cachetypes.Resource, 0, len(routes))
	for _, route := range routes {
		out = append(out, route)
	}
	return out
}

func testCluster(name string) *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{Name: name}
}

func testRouteConfig(name, cluster string) *envoyroutev3.RouteConfiguration {
	return &envoyroutev3.RouteConfiguration{
		Name: name,
		VirtualHosts: []*envoyroutev3.VirtualHost{
			{
				Name:    "local_service",
				Domains: []string{"*"},
				Routes: []*envoyroutev3.Route{
					{
						Match: &envoyroutev3.RouteMatch{
							PathSpecifier: &envoyroutev3.RouteMatch_Prefix{Prefix: "/"},
						},
						Action: &envoyroutev3.Route_Route{
							Route: &envoyroutev3.RouteAction{
								ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: cluster},
							},
						},
					},
				},
			},
		},
	}
}

func adsModeName(ordered bool) string {
	if ordered {
		return "ordered"
	}
	return "default"
}
