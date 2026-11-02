package proxy_syncer

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"slices"
	"testing"
	"time"
	"weak"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// withScannedRouting gives a fixture built from a bare snapshot the routing
// targets snapshotPerClient would compose for it.
func (p XdsSnapWrapper) withScannedRouting() XdsSnapWrapper {
	return p.WithSnapshot(p.snap)
}

func targetSet(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

// partIdentities identifies the shared sets routing is built from.
func partIdentities(routing routingTargets) []uintptr {
	out := make([]uintptr, 0, len(routing.parts))
	for _, part := range routing.parts {
		out = append(out, reflect.ValueOf(part).Pointer())
	}
	slices.Sort(out)
	return out
}

func identities(parts ...map[string]struct{}) []uintptr {
	return partIdentities(routingTargets{parts: parts})
}

// Values derived from one routingTargets never see each other's parts, even
// when the shared slice has spare capacity.
func TestRoutingTargetsDerivedValuesDoNotAlias(t *testing.T) {
	base := newRoutingTargets(targetSet("a"), targetSet("b"), targetSet("c"))
	require.Greater(t, cap(base.parts), len(base.parts), "the fixture needs spare capacity to catch aliasing")

	left := base.with(targetSet("left"))
	right := base.with(targetSet("right"))

	assert.Equal(t, targetSet("a", "b", "c", "left"), left.names())
	assert.Equal(t, targetSet("a", "b", "c", "right"), right.names())
	assert.Equal(t, targetSet("a", "b", "c"), base.names())
	assert.False(t, newRoutingTargets(nil, targetSet()).has(""), "empty parts add nothing")
}

// A build's listeners are the gateway's plus the kept backends' own, with the
// gateway's taking precedence on a name and a later backend listener replacing
// an earlier one. The composed targets follow exactly those listeners.
func TestBackendListenerRoutingTargetsFollowMergedListeners(t *testing.T) {
	gatewayRoutes := routeResourcesForClusters("routed")
	gatewayListeners := sliceToResources([]*envoylistenerv3.Listener{
		httpListenerWithRDS(t, "http", "route-config"),
		tunnelListener("shadowed", "gateway-target"),
	})
	byCluster := map[string]baseResources[*envoylistenerv3.Listener]{
		"a-kept": backendListenersOf(
			tunnelListener("shadowed", "shadowed-target"),
			tunnelListener("tunnel_replaced", "replaced-target"),
			tunnelListener("tunnel_cleared", "cleared-target"),
			tunnelListener("tunnel_kept", "kept-target"),
		),
		"b-kept": backendListenersOf(
			tunnelListener("tunnel_replaced", "replacing-target"),
			&envoylistenerv3.Listener{Name: "tunnel_cleared"},
		),
		"dropped": backendListenersOf(tunnelListener("tunnel_dropped", "dropped-target")),
	}
	kept := keptBackendResources(map[string]envoycachetypes.ResourceWithTTL{"a-kept": {}, "b-kept": {}}, byCluster)

	listeners, backendRouting := mergeBackendResources(gatewayListeners, kept)
	routing := newRoutingTargets(collectReferencedClusters(gatewayRoutes, gatewayListeners)).union(backendRouting)

	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Route] = gatewayRoutes
	snap.Resources[envoycachetypes.Listener] = listeners
	require.NoError(t, routingTargetsError(snap, routing), "the composed targets must describe the merged listeners")
	assert.Equal(t, targetSet("routed", "gateway-target", "replacing-target", "kept-target"), routing.names())
}

// sharedRoutingFixture builds two clients of one gateway whose route reaches a
// tunneled backend, with a mutable endpoint collection.
type sharedRoutingFixture struct {
	clients     []ir.UniquelyConnectedClient
	gateway     GatewayXdsResources
	tunnel      baseResources[*envoylistenerv3.Listener]
	endpointCol krt.StaticCollection[UccWithEndpoints]
	snapshots   krt.Collection[XdsSnapWrapper]
}

func newSharedRoutingFixture(t *testing.T) *sharedRoutingFixture {
	t.Helper()
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	f := &sharedRoutingFixture{
		clients: []ir.UniquelyConnectedClient{
			ir.NewUniquelyConnectedClient(role, "ns", map[string]string{"pod": "a"}, ir.PodLocality{}),
			ir.NewUniquelyConnectedClient(role, "ns", map[string]string{"pod": "b"}, ir.PodLocality{}),
		},
		tunnel: backendListenersOf(tunnelListener("tunnel_tunneled", "proxy")),
	}
	routes := routeResourcesForClusters("tunneled")
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	f.gateway = GatewayXdsResources{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}
	var clusterRows []uccWithCluster
	var endpointRows []UccWithEndpoints
	for _, ucc := range f.clients {
		tunneled := edsClusterForClient(ucc, "tunneled", 1)
		tunneled.Listeners = f.tunnel
		clusterRows = append(clusterRows, tunneled, edsClusterForClient(ucc, "proxy", 2))
		endpointRows = append(endpointRows, endpointsForClient(ucc, "tunneled", 3), endpointsForClient(ucc, "proxy", 4))
	}
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, f.clients)
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, clusterRows)
	f.endpointCol = krt.NewMutableCollection[UccWithEndpoints](nil, endpointRows)
	f.snapshots = snapshotPerClient(
		krtutil.KrtOptions{},
		uccs,
		krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{f.gateway}),
		PerClientEnvoyEndpoints{
			endpoints: f.endpointCol.AsCollection(),
			index: krtpkg.UnnamedIndex(f.endpointCol.AsCollection(), func(ep UccWithEndpoints) []string {
				return []string{ep.Client.ResourceName()}
			}),
		},
		newTestPerClientClustersFromCol(clusterCol, uccs),
		clusterScoping{},
	)
	return f
}

// wrapperFor waits for ucc's row to satisfy ready.
func (f *sharedRoutingFixture) wrapperFor(t *testing.T, ucc ir.UniquelyConnectedClient, ready func(XdsSnapWrapper) bool) XdsSnapWrapper {
	t.Helper()
	var wrap XdsSnapWrapper
	require.Eventually(t, func() bool {
		row := f.snapshots.GetKey(ucc.ResourceName())
		if row == nil || !ready(*row) {
			return false
		}
		wrap = *row
		return true
	}, 2*time.Second, 10*time.Millisecond)
	return wrap
}

// Clients of one gateway share its routing targets and the tunnel listener's,
// both computed once where the routes and the listener were produced, and an
// endpoint-only rebuild reuses them instead of recomputing anything.
func TestSnapshotPerClientSharesRoutingTargets(t *testing.T) {
	f := newSharedRoutingFixture(t)
	shared := identities(f.gateway.ReferencedClusters, f.tunnel.items[0].routing)

	var first XdsSnapWrapper
	for i, ucc := range f.clients {
		wrap := f.wrapperFor(t, ucc, func(XdsSnapWrapper) bool { return true })
		require.False(t, wrap.deferred())
		require.NoError(t, routingTargetsError(wrap.snap, wrap.routing))
		assert.Equal(t, targetSet("tunneled", "proxy"), wrap.routing.names())
		assert.Equal(t, shared, partIdentities(wrap.routing), "client %s must share the computed targets", ucc.ResourceName())
		if i == 0 {
			first = wrap
		}
	}

	ucc := f.clients[0]
	f.endpointCol.UpdateObject(endpointsForClient(ucc, "tunneled", 30))
	endpointsVersion := first.snap.Resources[envoycachetypes.Endpoint].Version
	rebuilt := f.wrapperFor(t, ucc, func(w XdsSnapWrapper) bool {
		return w.snap.Resources[envoycachetypes.Endpoint].Version != endpointsVersion
	})
	assert.Equal(t, first.snap.Resources[envoycachetypes.Listener].Version, rebuilt.snap.Resources[envoycachetypes.Listener].Version)
	assert.Equal(t, shared, partIdentities(rebuilt.routing), "an endpoint-only rebuild reuses the computed targets")
}

// gateRetentionBuilds publishes v1, then a deferred v2 the gate resolves
// against v1, then a coherent v3. It returns weak pointers to each build's
// route configuration, so the caller holds none of them.
//
//go:noinline
func gateRetentionBuilds(t *testing.T, pt *ProxyTranslator, held bool) (v1, v2, v3 weak.Pointer[envoyroutev3.RouteConfiguration]) {
	ctx := context.Background()
	routeConfig := func(snap *envoycache.Snapshot) weak.Pointer[envoyroutev3.RouteConfiguration] {
		rc := snap.Resources[envoycachetypes.Route].Items["route-config"].Resource.(*envoyroutev3.RouteConfiguration)
		return weak.Make(rc)
	}

	first := carryTestBuild(t, "v1", []string{"cluster-a", "cluster-b"}, "cluster-a", "cluster-b")
	pt.syncXds(ctx, XdsSnapWrapper{snap: first, proxyKey: publishGateTestClient}.withScannedRouting())

	var second XdsSnapWrapper
	if held {
		// cluster-new is newly referenced and its CLA is underived: the flip holds.
		snap := carryTestBuild(t, "v2", []string{"cluster-a", "cluster-new"}, "cluster-a", "cluster-b", "cluster-new")
		second = XdsSnapWrapper{snap: snap, proxyKey: publishGateTestClient, missingEndpointsReferenced: []string{"cluster-new"}}.withScannedRouting()
	} else {
		// cluster-b lags the build and is carried; the new routes publish.
		snap := carryTestBuild(t, "v2", []string{"cluster-a", "cluster-b"}, "cluster-a")
		second = XdsSnapWrapper{snap: snap, proxyKey: publishGateTestClient, missingReferenced: []string{"cluster-b"}}.withScannedRouting()
	}
	pt.syncXds(ctx, second)
	if held {
		require.Equal(t, "rds-v1", servedSnapshot(t, pt).GetVersion(envoyresourcev3.RouteType), "the flip is held")
	} else {
		require.Equal(t, "rds-v2", servedSnapshot(t, pt).GetVersion(envoyresourcev3.RouteType), "the carry publishes the new routes")
	}

	third := carryTestBuild(t, "v3", []string{"cluster-a", "cluster-new"}, "cluster-a", "cluster-new")
	pt.syncXds(ctx, XdsSnapWrapper{snap: third, proxyKey: publishGateTestClient}.withScannedRouting())
	require.Equal(t, "rds-v3", servedSnapshot(t, pt).Resources[envoycachetypes.Route].Version)
	return routeConfig(first), routeConfig(second.snap), routeConfig(third)
}

// Once a coherent build supersedes them, the routes the gate resolved deferred
// builds against become unreachable: recording what was published retains no
// routes or listeners. The installed snapshot still holds its own.
func TestPublishedRoutingTargetsRetainNoSupersededConfig(t *testing.T) {
	for _, held := range []bool{false, true} {
		name := "carried"
		if held {
			name = "held"
		}
		t.Run(name, func(t *testing.T) {
			pt := newPublishGateTestTranslator(t, false, time.Hour)
			v1, v2, v3 := gateRetentionBuilds(t, pt, held)
			require.Eventually(t, func() bool {
				runtime.GC()
				return v1.Value() == nil && v2.Value() == nil
			}, 2*time.Second, 10*time.Millisecond, "superseded route configurations must be collectable")
			assert.NotNil(t, v3.Value(), "the installed snapshot keeps its routes")
			runtime.KeepAlive(pt)
		})
	}
}

// failNextInstallCache rejects installs without installing while fail is set.
type failNextInstallCache struct {
	envoycache.SnapshotCache
	fail bool
}

func (c *failNextInstallCache) SetSnapshot(ctx context.Context, node string, snapshot envoycache.ResourceSnapshot) error {
	if c.fail {
		return errors.New("install rejected")
	}
	return c.SnapshotCache.SetSnapshot(ctx, node, snapshot)
}

// recordedRouting returns the routing targets the gate recorded for the test
// client, and whether it counts the client as published on its connection.
func recordedRouting(pt *ProxyTranslator) (map[string]struct{}, bool) {
	pt.gate.mu.Lock()
	defer pt.gate.mu.Unlock()
	routing, ok := pt.gate.published[publishGateTestClient]
	return routing.names(), ok
}

// The recorded routing targets follow installation: a superseded connection's
// or a rejected install leaves them describing the snapshot still installed,
// an install whose watches could not be answered advances them, a reconnect
// forgets them, and timers record what they publish.
func TestPublishedRoutingTargetsFollowInstallation(t *testing.T) {
	ctx := context.Background()
	build := func(version string, routeTo ...string) XdsSnapWrapper {
		return XdsSnapWrapper{snap: carryTestBuild(t, version, routeTo, routeTo...), proxyKey: publishGateTestClient}.withScannedRouting()
	}

	t.Run("superseded connection", func(t *testing.T) {
		pt := newPublishGateTestTranslator(t, false, time.Hour)
		pt.syncXds(ctx, build("v1", "cluster-a"))
		pt.gate.beforeInstall = func(key string) { pt.gate.fence.start(pt.xdsCache, key) }
		pt.syncXds(ctx, build("v2", "cluster-b"))
		pt.gate.beforeInstall = nil
		recorded, published := recordedRouting(pt)
		require.True(t, published)
		assert.Equal(t, targetSet("cluster-a"), recorded, "a publish for the previous connection records nothing")

		pt.gate.connectionStarted(publishGateTestClient)
		_, published = recordedRouting(pt)
		assert.False(t, published, "a new connection forgets the previous connection's targets")
		pt.syncXds(ctx, build("v2", "cluster-b"))
		recorded, _ = recordedRouting(pt)
		assert.Equal(t, targetSet("cluster-b"), recorded)
		servedSnapshot(t, pt)
	})

	t.Run("rejected install", func(t *testing.T) {
		cache := &failNextInstallCache{SnapshotCache: newTestSnapshotCache(t)}
		pt := NewProxyTranslator(cache, stubPriorXDS{}, time.Hour, true, clusterScoping{})
		pt.syncXds(ctx, build("v1", "cluster-a"))
		cache.fail = true
		pt.syncXds(ctx, build("v2", "cluster-b"))
		cache.fail = false
		recorded, _ := recordedRouting(&pt)
		assert.Equal(t, targetSet("cluster-a"), recorded, "a rejected install records nothing")
		assert.Equal(t, "v1", servedSnapshot(t, &pt).GetVersion(envoyresourcev3.ListenerType))
	})

	t.Run("installed but unanswered", func(t *testing.T) {
		pt := NewProxyTranslator(installThenFailCache{newTestSnapshotCache(t)}, stubPriorXDS{}, time.Hour, true, clusterScoping{})
		pt.syncXds(ctx, build("v1", "cluster-a"))
		pt.syncXds(ctx, build("v2", "cluster-b"))
		recorded, _ := recordedRouting(&pt)
		assert.Equal(t, targetSet("cluster-b"), recorded, "an installed snapshot is recorded even if answering failed")
		servedSnapshot(t, &pt)
	})

	t.Run("first-publish budget", func(t *testing.T) {
		pt := newPublishGateTestTranslator(t, false, 20*time.Millisecond)
		wrap := build("v1", "cluster-a")
		wrap.missingEndpointsReferenced = []string{"cluster-a"}
		pt.syncXds(ctx, wrap)
		require.Eventually(t, func() bool {
			_, published := recordedRouting(pt)
			return published
		}, 2*time.Second, 5*time.Millisecond)
		recorded, _ := recordedRouting(pt)
		assert.Equal(t, targetSet("cluster-a"), recorded)
		servedSnapshot(t, pt)
	})

	t.Run("flip release", func(t *testing.T) {
		pt := newPublishGateTestTranslator(t, false, 20*time.Millisecond)
		_, flipWrap := flipHoldFixture(t, pt)
		pt.syncXds(ctx, flipWrap)
		recorded, _ := recordedRouting(pt)
		assert.Equal(t, targetSet("cluster-old"), recorded, "a hold records the held routes")
		require.Eventually(t, func() bool {
			recorded, _ := recordedRouting(pt)
			_, released := recorded["cluster-new"]
			return released && len(recorded) == 1
		}, 2*time.Second, 5*time.Millisecond, "the release records the released routes")
		servedSnapshot(t, pt)
	})
}
