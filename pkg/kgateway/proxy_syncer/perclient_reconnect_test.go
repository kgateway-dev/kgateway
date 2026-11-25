package proxy_syncer

// A reconnect rebuilds all client rows. The endpoints fan-out (every backend for
// every client) usually lands last, leaving CLAs briefly missing; these tests
// stall it to hold that window open.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

type reconnectBackend struct{ name string }

func (b reconnectBackend) ResourceName() string { return b.name }

// recordingCache wraps the consistency oracle and records one node's snapshots.
type recordingCache struct {
	envoycache.SnapshotCache
	node      string
	mu        sync.Mutex
	published []*envoycache.Snapshot
}

func (c *recordingCache) SetSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot) error {
	if node == c.node {
		c.mu.Lock()
		c.published = append(c.published, snapshot.(*envoycache.Snapshot))
		c.mu.Unlock()
	}
	return c.SnapshotCache.SetSnapshot(ctx, node, snapshot)
}

func (c *recordingCache) publishCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.published)
}

func (c *recordingCache) publishedSince(n int) []*envoycache.Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*envoycache.Snapshot(nil), c.published[n:]...)
}

type reconnectFixture struct {
	ucc       ir.UniquelyConnectedClient
	uccs      krt.StaticCollection[ir.UniquelyConnectedClient]
	gateways  krt.StaticCollection[GatewayXdsResources]
	backends  krt.StaticCollection[reconnectBackend]
	snapshots krt.Collection[XdsSnapWrapper]
	endpoints krt.Collection[UccWithEndpoints]
	cache     *recordingCache
	listeners envoycache.Resources
	stall     atomic.Bool
	release   chan struct{}
}

func newReconnectFixture(t *testing.T, routeTargets []string, backendNames []string) *reconnectFixture {
	t.Helper()
	f := &reconnectFixture{release: make(chan struct{})}
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	f.ucc = ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	f.uccs = krt.NewMutableCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{f.ucc})
	f.listeners = sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	f.gateways = krt.NewMutableCollection[GatewayXdsResources](nil, nil)
	f.setRoutes(routeTargets...)
	var initial []reconnectBackend
	for _, name := range backendNames {
		initial = append(initial, reconnectBackend{name: name})
	}
	f.backends = krt.NewMutableCollection[reconnectBackend](nil, initial)

	// Mirror production CDS: a complete backend row for each client.
	perClient := krt.NewCollection(f.uccs.AsCollection(), func(kctx krt.HandlerContext, c ir.UniquelyConnectedClient) *clustersWithErrors {
		var rows []uccWithCluster
		for _, b := range krt.Fetch(kctx, f.backends.AsCollection()) {
			rows = append(rows, edsClusterForClient(c, b.name, uint64(len(b.name))))
		}
		return assemblePerClientClusters(c, rows)
	})
	// Mirror NewPerClientEnvoyEndpoints: each backend builds rows for all clients.
	endpointRows := krt.NewManyCollection(f.backends.AsCollection(), func(kctx krt.HandlerContext, b reconnectBackend) []UccWithEndpoints {
		clients := krt.Fetch(kctx, f.uccs.AsCollection())
		if f.stall.Load() && len(clients) > 0 {
			<-f.release
		}
		out := make([]UccWithEndpoints, 0, len(clients))
		for _, c := range clients {
			out = append(out, endpointsForClient(c, b.name, 7+uint64(len(b.name))))
		}
		return out
	})
	endpoints := PerClientEnvoyEndpoints{
		endpoints: endpointRows,
		index:     krtpkg.UnnamedIndex(endpointRows, func(ep UccWithEndpoints) []string { return []string{ep.Client.ResourceName()} }),
	}
	f.endpoints = endpointRows

	f.snapshots = snapshotPerClient(krtutil.KrtOptions{}, f.uccs.AsCollection(), f.gateways.AsCollection(), endpoints, PerClientEnvoyClusters{perClient: perClient}, clusterScoping{})
	f.cache = &recordingCache{SnapshotCache: newTestSnapshotCache(t), node: f.ucc.ResourceName()}
	translator := NewProxyTranslator(f.cache, stubPriorXDS{has: true}, 15*time.Second, true, clusterScoping{})
	// Mirror ProxySyncer.Start's current-row check and clientDeparted on Delete.
	translator.gate.currentBuild = f.snapshots.GetKey
	f.snapshots.RegisterBatch(func(events []krt.Event[XdsSnapWrapper]) {
		for _, event := range events {
			if event.Event == controllers.EventDelete {
				translator.gate.clientDeparted(event.Latest().ResourceName())
				continue
			}
			translator.syncXds(context.Background(), event.Latest())
		}
	}, true)
	t.Cleanup(func() {
		f.unstall()
		translator.gate.clientDeparted(f.ucc.ResourceName())
	})
	return f
}

func (f *reconnectFixture) setRoutes(clusters ...string) {
	routes := routeResourcesForClusters(clusters...)
	f.gateways.UpdateObject(GatewayXdsResources{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          f.listeners,
		ReferencedClusters: collectReferencedClusters(routes, f.listeners),
	})
}

// reconnect drops the client, runs whileAway, and reconnects with endpoints
// stalled. It returns the publish count at reconnect.
func (f *reconnectFixture) reconnect(t *testing.T, whileAway func()) int {
	t.Helper()
	f.uccs.DeleteObject(f.ucc.ResourceName())
	require.Eventually(t, func() bool { return len(f.snapshots.List()) == 0 }, 2*time.Second, 5*time.Millisecond)
	// Wait for the endpoint rows to go before stalling, or a quick reconnect can
	// reuse them and skip the window under test.
	require.Eventually(t, func() bool { return len(f.endpoints.List()) == 0 }, 2*time.Second, 5*time.Millisecond)
	if whileAway != nil {
		whileAway()
	}
	mark := f.cache.publishCount()
	f.stall.Store(true)
	f.uccs.UpdateObject(f.ucc)
	// Wait for a rebuilt snapshot that still lacks CLA rows.
	require.Eventually(t, func() bool {
		wraps := f.snapshots.List()
		return len(wraps) == 1 && len(wraps[0].synthesizedEndpoints) > 0
	}, 2*time.Second, 5*time.Millisecond)
	return mark
}

func (f *reconnectFixture) unstall() {
	if f.stall.CompareAndSwap(true, false) {
		close(f.release)
	}
}

func endpointCount(snap *envoycache.Snapshot, cluster string) (int, bool) {
	item, ok := snap.Resources[envoycachetypes.Endpoint].Items[cluster]
	if !ok {
		return 0, false
	}
	n := 0
	for _, locality := range item.Resource.(*envoyendpointv3.ClusterLoadAssignment).GetEndpoints() {
		n += len(locality.GetLbEndpoints())
	}
	return n, true
}

func (f *reconnectFixture) served(t *testing.T) *envoycache.Snapshot {
	t.Helper()
	snap, err := f.cache.GetSnapshot(f.ucc.ResourceName())
	require.NoError(t, err)
	return snap.(*envoycache.Snapshot)
}

// While CLA rows rebuild, no publish may replace held endpoints with an empty
// CLA, including for filter-only clusters, which never defer the snapshot.
func TestReconnectWhileEndpointRowsRebuildKeepsClientEndpoints(t *testing.T) {
	f := newReconnectFixture(t, []string{"cluster-a"}, []string{"cluster-a", "cluster-authz"})
	require.Eventually(t, func() bool {
		snap, err := f.cache.GetSnapshot(f.ucc.ResourceName())
		if err != nil {
			return false
		}
		a, _ := endpointCount(snap.(*envoycache.Snapshot), "cluster-a")
		authz, _ := endpointCount(snap.(*envoycache.Snapshot), "cluster-authz")
		return a == 1 && authz == 1
	}, 2*time.Second, 5*time.Millisecond, "steady state serves both clusters with their endpoint")

	mark := f.reconnect(t, nil)
	time.Sleep(50 * time.Millisecond)
	for _, snap := range f.cache.publishedSince(mark) {
		for _, cluster := range []string{"cluster-a", "cluster-authz"} {
			if n, ok := endpointCount(snap, cluster); ok {
				require.Equal(t, 1, n, "a publish during the rebuild replaced %s's endpoints with an empty CLA", cluster)
			}
		}
	}

	f.unstall()
	require.Eventually(t, func() bool {
		wraps := f.snapshots.List()
		return len(wraps) == 1 && len(wraps[0].synthesizedEndpoints) == 0 && f.cache.publishCount() > mark
	}, 2*time.Second, 5*time.Millisecond, "the rebuilt snapshot publishes once the rows land")
	served := f.served(t)
	for _, cluster := range []string{"cluster-a", "cluster-authz"} {
		n, ok := endpointCount(served, cluster)
		require.True(t, ok)
		require.Equal(t, 1, n)
	}
	assertSnapshotCoherent(t, served)
}

// A reconnect is not resolved against the old cache entry, which can predate
// config from another replica. Holding or carrying from it would restore the old
// route or the deleted backend.
func TestReconnectDoesNotResolveAgainstPreviousConnection(t *testing.T) {
	f := newReconnectFixture(t, []string{"cluster-a"}, []string{"cluster-a", "cluster-b"})
	require.Eventually(t, func() bool {
		snap, err := f.cache.GetSnapshot(f.ucc.ResourceName())
		return err == nil && snapshotReferencesCluster(snap.(*envoycache.Snapshot), "cluster-a")
	}, 2*time.Second, 5*time.Millisecond)

	mark := f.reconnect(t, func() {
		f.setRoutes("cluster-b")
		f.backends.DeleteObject("cluster-a")
	})
	time.Sleep(50 * time.Millisecond)
	for _, snap := range f.cache.publishedSince(mark) {
		require.False(t, snapshotReferencesCluster(snap, "cluster-a"), "a publish after reconnect restored the route from the previous connection")
		require.NotContains(t, snap.Resources[envoycachetypes.Cluster].Items, "cluster-a", "a publish after reconnect resurrected the deleted cluster")
	}

	f.unstall()
	require.Eventually(t, func() bool {
		if f.cache.publishCount() == mark {
			return false
		}
		return snapshotReferencesCluster(f.served(t), "cluster-b")
	}, 2*time.Second, 5*time.Millisecond)
	served := f.served(t)
	require.NotContains(t, served.Resources[envoycachetypes.Cluster].Items, "cluster-a")
	assertSnapshotCoherent(t, served)
}

// Preserve published endpoints when their row disappears. A derived empty CLA
// still publishes as known backend state.
func TestPublishedClientKeepsEndpointsWhenRowIsNotDerived(t *testing.T) {
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	routes := routeResourcesForClusters("cluster-a")
	gateways := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})
	clusterRows := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		edsClusterForClient(ucc, "cluster-a", 1),
		edsClusterForClient(ucc, "cluster-authz", 2),
	})
	live := endpointsForClient(ucc, "cluster-a", 3)
	liveAuthz := endpointsForClient(ucc, "cluster-authz", 4)
	endpointRows := krt.NewMutableCollection[UccWithEndpoints](nil, []UccWithEndpoints{live, liveAuthz})
	snapshots := snapshotPerClient(krtutil.KrtOptions{}, uccs, gateways,
		PerClientEnvoyEndpoints{
			endpoints: endpointRows.AsCollection(),
			index:     krtpkg.UnnamedIndex(endpointRows.AsCollection(), func(ep UccWithEndpoints) []string { return []string{ep.Client.ResourceName()} }),
		},
		newTestPerClientClustersFromCol(clusterRows, uccs),
		clusterScoping{},
	)
	cache := newTestSnapshotCache(t)
	registerSyncXds(snapshots, NewProxyTranslator(cache, nil, 0, true, clusterScoping{}))
	node := ucc.ResourceName()
	endpointsServed := func(cluster string) (int, bool) {
		snap, err := cache.GetSnapshot(node)
		if err != nil {
			return 0, false
		}
		return endpointCount(snap.(*envoycache.Snapshot), cluster)
	}
	require.Eventually(t, func() bool {
		a, _ := endpointsServed("cluster-a")
		authz, _ := endpointsServed("cluster-authz")
		return a == 1 && authz == 1
	}, time.Second, 5*time.Millisecond)

	endpointRows.DeleteObject(live.ResourceName())
	endpointRows.DeleteObject(liveAuthz.ResourceName())
	require.Eventually(t, func() bool {
		wraps := snapshots.List()
		return len(wraps) == 1 && len(wraps[0].synthesizedEndpoints) == 2
	}, time.Second, 5*time.Millisecond)
	require.Never(t, func() bool {
		a, _ := endpointsServed("cluster-a")
		authz, _ := endpointsServed("cluster-authz")
		return a != 1 || authz != 1
	}, 200*time.Millisecond, 10*time.Millisecond, "endpoints the client holds must not be replaced by an empty CLA")

	endpointRows.UpdateObject(emptyEndpointsForClient(ucc, "cluster-a", 5))
	require.Eventually(t, func() bool {
		n, ok := endpointsServed("cluster-a")
		return ok && n == 0
	}, time.Second, 5*time.Millisecond, "a derived empty CLA is the backend's truth and publishes")
}
