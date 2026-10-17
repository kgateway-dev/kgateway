package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// sourceFixture wires snapshotPerClient to static collections and the gate the
// way ProxySyncer.Start does, with an explicit set of backend endpoint sources.
type sourceFixture struct {
	ucc       ir.UniquelyConnectedClient
	uccs      krt.StaticCollection[ir.UniquelyConnectedClient]
	rows      krt.StaticCollection[UccWithEndpoints]
	snapshots krt.Collection[XdsSnapWrapper]
	cache     envoycache.SnapshotCache
}

func newSourceFixture(t *testing.T, state krtcollections.XDSClientState, routed string, clusters []string, sources []string, rows []string) *sourceFixture {
	t.Helper()
	f := &sourceFixture{}
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	f.ucc = ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	f.uccs = krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{f.ucc})
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	routes := routeResourcesForClusters(routed)
	gateways := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})
	var clusterRows []uccWithCluster
	var hash uint64
	for _, name := range clusters {
		hash++
		clusterRows = append(clusterRows, edsClusterForClient(f.ucc, name, hash))
	}
	var sourceRows []ir.EndpointsForBackend
	for _, name := range sources {
		sourceRows = append(sourceRows, ir.EndpointsForBackend{ClusterName: name, UpstreamResourceName: name})
	}
	sourceCol := krt.NewStaticCollection[ir.EndpointsForBackend](nil, sourceRows)
	var initialRows []UccWithEndpoints
	hash = 100
	for _, name := range rows {
		hash++
		initialRows = append(initialRows, endpointsForClient(f.ucc, name, hash))
	}
	f.rows = krt.NewStaticCollection[UccWithEndpoints](nil, initialRows)
	endpoints := PerClientEnvoyEndpoints{
		endpoints: f.rows,
		index:     krtpkg.UnnamedIndex(f.rows, func(ep UccWithEndpoints) []string { return []string{ep.Client.ResourceName()} }),
		sources:   sourceCol,
		sourcesByCluster: krtpkg.UnnamedIndex(sourceCol, func(ep ir.EndpointsForBackend) []string {
			return []string{ep.ClusterName}
		}),
	}
	f.snapshots = snapshotPerClient(krtutil.KrtOptions{}, f.uccs, gateways, endpoints,
		newTestPerClientClustersFromCol(krt.NewStaticCollection[uccWithCluster](nil, clusterRows), f.uccs), clusterScoping{})
	f.cache = newTestSnapshotCache(t)
	translator := NewProxyTranslator(f.cache, state, 15*time.Second, true, clusterScoping{})
	f.snapshots.RegisterBatch(func(events []krt.Event[XdsSnapWrapper]) {
		for _, e := range events {
			if e.Event == controllers.EventDelete {
				translator.gate.clientDeparted(e.Latest().ResourceName())
				continue
			}
			translator.syncXds(context.Background(), e.Latest())
		}
	}, true)
	t.Cleanup(func() { translator.gate.clientDeparted(f.ucc.ResourceName()) })
	return f
}

func (f *sourceFixture) hosts(cla string) (int, bool) {
	rs, err := f.cache.GetSnapshot(f.ucc.ResourceName())
	if err != nil {
		return 0, false
	}
	return endpointCount(rs.(*envoycache.Snapshot), cla)
}

// TestFirstPublishLeavesOutRowsStillBeingBuilt: a new proxy's first snapshot
// can publish as soon as its route targets' rows exist, while rows for other
// clusters (a filter-called ext_authz backend, say) are still being built.
// Publishing their synthesized empty CLAs would let Envoy finish initializing
// with zero hosts behind them, so ext_authz denies every request until the row
// lands; leaving them out makes Envoy wait for them, as it did before the
// placeholders existed. A cluster no backend derives endpoints for (an EC2
// Backend with discovery disabled) never gets a row, so its empty CLA is its
// steady state and still publishes, letting it activate immediately.
func TestFirstPublishLeavesOutRowsStillBeingBuilt(t *testing.T) {
	f := newSourceFixture(t, stubPriorXDS{has: false},
		"routed",
		[]string{"routed", "authz", "ec2-no-discovery"},
		[]string{"routed", "authz"}, // ec2-no-discovery has no endpoints source
		[]string{"routed"},          // the authz row has not been built yet
	)

	require.Eventually(t, func() bool { _, ok := f.hosts("routed"); return ok }, 2*time.Second, 5*time.Millisecond)
	wraps := f.snapshots.List()
	require.Len(t, wraps, 1)
	assert.Equal(t, []string{"authz"}, wraps[0].synthesizedEndpoints,
		"only the synthesized CLA whose row is on its way is settled; the sourceless one is truth")

	_, authzPublished := f.hosts("authz")
	assert.False(t, authzPublished, "a row still being built is left out, so Envoy waits for it")
	n, ec2Published := f.hosts("ec2-no-discovery")
	assert.True(t, ec2Published, "a cluster with no endpoints source publishes its empty CLA")
	assert.Zero(t, n)

	f.rows.UpdateObject(endpointsForClient(f.ucc, "authz", 200))
	require.Eventually(t, func() bool { n, ok := f.hosts("authz"); return ok && n == 1 }, 2*time.Second, 5*time.Millisecond,
		"the derived CLA publishes as soon as the row lands")
}

// TestReconnectWithoutClientStateKeepsFilterClusterEndpoints: a deployment that
// does not wire XDSClientState cannot tell a reconnecting proxy from a new one.
// Leaving out rows still being built must not depend on that, or every
// reconnect would publish explicit empty CLAs over the endpoints the proxy
// holds for clusters whose rows are rebuilt last.
func TestReconnectWithoutClientStateKeepsFilterClusterEndpoints(t *testing.T) {
	f := newSourceFixture(t, nil,
		"routed",
		[]string{"routed", "authz"},
		[]string{"routed", "authz"},
		[]string{"routed", "authz"},
	)
	require.Eventually(t, func() bool { n, ok := f.hosts("authz"); return ok && n == 1 }, 2*time.Second, 5*time.Millisecond)

	// Stream reset: the client's rows are dropped and the cache entry retained.
	f.uccs.DeleteObject(f.ucc.ResourceName())
	f.rows.DeleteObject(endpointsForClient(f.ucc, "routed", 0).ResourceName())
	f.rows.DeleteObject(endpointsForClient(f.ucc, "authz", 0).ResourceName())
	require.Eventually(t, func() bool { return len(f.snapshots.List()) == 0 }, 2*time.Second, 5*time.Millisecond)

	// Reconnect; the routed backend's row is rebuilt first.
	f.uccs.UpdateObject(f.ucc)
	f.rows.UpdateObject(endpointsForClient(f.ucc, "routed", 100))
	require.Eventually(t, func() bool {
		wraps := f.snapshots.List()
		return len(wraps) == 1 && len(wraps[0].missingEndpointsReferenced) == 0 && len(wraps[0].synthesizedEndpoints) == 1
	}, 2*time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	n, ok := f.hosts("authz")
	assert.False(t, ok && n == 0, "the reconnected proxy must not be sent an empty CLA for a cluster whose row is being rebuilt")
}
