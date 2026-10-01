package proxy_syncer

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// Contract and concurrency coverage for NewPerClientEnvoyClusters (the per-client
// CDS collection). Base clusters are shared across clients, and each connected
// client has exactly one row: its assembled CDS payload. These pin that a
// connected client sees all backends across any sequence of client/backend
// add/remove, that removing one client leaves the others untouched, and that a
// client which stays connected is never left permanently without its clusters
// while others churn.
//
// Added while investigating #14184 (proxies stranded with empty per-client config).
// They document the behavior the collection must preserve and pass against the
// current implementation; they are not a reproduction of that issue.

func clustersTestTranslator() *irtranslator.BackendTranslator {
	return &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "", Kind: "Service"}: {
				InitEnvoyBackend: func(_ context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					// A translated field tests can vary to force a real base change.
					out.AltStatName = string(in.AppProtocol)
					return nil
				},
			},
		},
	}
}

func clustersTestBackend(name string) *ir.BackendObjectIR {
	return clustersTestBackendWithProtocol(name, "")
}

// clustersTestBackendWithProtocol is clustersTestBackend with an AppProtocol,
// which the test translator copies into the cluster's AltStatName. Alternating
// it is how churn tests make the base row genuinely change; resubmitting an
// identical backend is absorbed by the base row's Equals and exercises nothing.
func clustersTestBackendWithProtocol(name, appProtocol string) *ir.BackendObjectIR {
	b := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "",
		Kind:      "Service",
		Namespace: "default",
		Name:      name,
	}, 443, "", "")
	b.AppProtocol = ir.AppProtocol(appProtocol)
	return &b
}

func clustersTestClient(role string) ir.UniquelyConnectedClient {
	return ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
}

// clusterNamesForClient reads the client's stored CDS payload, so it observes what
// KRT has propagated rather than what FetchClustersForClient would compute now.
func clusterNamesForClient(c PerClientEnvoyClusters, ucc ir.UniquelyConnectedClient) []string {
	return storedClusterNamesForClient(c, ucc)
}

func newClustersTestFixture(
	t *testing.T,
	clients []ir.UniquelyConnectedClient,
	backends []*ir.BackendObjectIR,
) (krt.StaticCollection[ir.UniquelyConnectedClient], krt.StaticCollection[*ir.BackendObjectIR], PerClientEnvoyClusters) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	uccs := krt.NewStaticCollection(nil, clients, krtopts.ToOptions("UniqueClients")...)
	finalBackends := krt.NewStaticCollection(nil, backends, krtopts.ToOptions("FinalBackends")...)
	clusters := NewPerClientEnvoyClusters(ctx, krtopts, clustersTestTranslator(), finalBackends, uccs)
	return uccs, finalBackends, clusters
}

func eventuallyClusterCount(t *testing.T, c PerClientEnvoyClusters, ucc ir.UniquelyConnectedClient, want int) {
	t.Helper()
	require.Eventuallyf(t, func() bool {
		return len(clusterNamesForClient(c, ucc)) == want
	}, 5*time.Second, 10*time.Millisecond,
		"client %q never reached %d clusters (last: %v)", ucc.ResourceName(), want, clusterNamesForClient(c, ucc))
}

// A single connected client receives a cluster for every backend.
func TestPerClientClusters_ClientGetsAllBackends(t *testing.T) {
	ucc := clustersTestClient("role-a")
	_, _, clusters := newClustersTestFixture(t,
		[]ir.UniquelyConnectedClient{ucc},
		[]*ir.BackendObjectIR{clustersTestBackend("b1"), clustersTestBackend("b2")},
	)
	eventuallyClusterCount(t, clusters, ucc, 2)
}

// A client that connects AFTER the collection is built must still get clusters for
// every backend.
func TestPerClientClusters_NewClientGetsClustersAfterConnect(t *testing.T) {
	first := clustersTestClient("role-first")
	uccs, _, clusters := newClustersTestFixture(t,
		[]ir.UniquelyConnectedClient{first},
		[]*ir.BackendObjectIR{clustersTestBackend("b1"), clustersTestBackend("b2"), clustersTestBackend("b3")},
	)
	eventuallyClusterCount(t, clusters, first, 3)

	late := clustersTestClient("role-late")
	uccs.UpdateObject(late)
	eventuallyClusterCount(t, clusters, late, 3)
	// existing client unaffected
	eventuallyClusterCount(t, clusters, first, 3)
}

// Adding a backend propagates to every connected client.
func TestPerClientClusters_BackendAddedPropagatesToAllClients(t *testing.T) {
	a, b := clustersTestClient("role-a"), clustersTestClient("role-b")
	_, finalBackends, clusters := newClustersTestFixture(t,
		[]ir.UniquelyConnectedClient{a, b},
		[]*ir.BackendObjectIR{clustersTestBackend("b1")},
	)
	eventuallyClusterCount(t, clusters, a, 1)
	eventuallyClusterCount(t, clusters, b, 1)

	finalBackends.UpdateObject(clustersTestBackend("b2"))
	eventuallyClusterCount(t, clusters, a, 2)
	eventuallyClusterCount(t, clusters, b, 2)
}

// Removing a client leaves other clients untouched and deletes the removed
// client's row.
func TestPerClientClusters_ClientRemovedLeavesOthersUnaffected(t *testing.T) {
	a, b := clustersTestClient("role-a"), clustersTestClient("role-b")
	uccs, _, clusters := newClustersTestFixture(t,
		[]ir.UniquelyConnectedClient{a, b},
		[]*ir.BackendObjectIR{clustersTestBackend("b1"), clustersTestBackend("b2")},
	)
	eventuallyClusterCount(t, clusters, a, 2)
	eventuallyClusterCount(t, clusters, b, 2)

	uccs.DeleteObject(b.ResourceName())
	// The surviving client keeps its full set...
	eventuallyClusterCount(t, clusters, a, 2)
	// ...while the disconnected client no longer has a resolved generation.
	eventuallyClusterCount(t, clusters, b, 0)
}

// Each client has its own stored row, keyed by the client, and the rows share
// the base protos rather than copying them.
func TestPerClientClusters_RowIsolation(t *testing.T) {
	a, b := clustersTestClient("role-a"), clustersTestClient("role-b")
	_, _, clusters := newClustersTestFixture(t,
		[]ir.UniquelyConnectedClient{a, b},
		[]*ir.BackendObjectIR{clustersTestBackend("b1"), clustersTestBackend("b2")},
	)
	eventuallyClusterCount(t, clusters, a, 2)
	eventuallyClusterCount(t, clusters, b, 2)
	rowA := clusters.perClient.GetKey(a.ResourceName())
	rowB := clusters.perClient.GetKey(b.ResourceName())
	require.NotNil(t, rowA)
	require.NotNil(t, rowB)
	require.Equal(t, a.ResourceName(), rowA.ResourceName())
	require.Equal(t, b.ResourceName(), rowB.ResourceName())
	for name, item := range rowA.clusters.Items {
		require.Same(t, item.Resource, rowB.clusters.Items[name].Resource,
			"clients with no overlay must share the base proto, not copies of it")
	}
}

// Churning a client through disconnect/reconnect keeps its full cluster set and
// does not disturb a co-connected client.
func TestPerClientClusters_ReAddClientKeepsRows(t *testing.T) {
	a, b := clustersTestClient("role-a"), clustersTestClient("role-b")
	uccs, _, clusters := newClustersTestFixture(t,
		[]ir.UniquelyConnectedClient{a, b},
		[]*ir.BackendObjectIR{clustersTestBackend("b1"), clustersTestBackend("b2")},
	)
	eventuallyClusterCount(t, clusters, b, 2)
	uccs.DeleteObject(b.ResourceName())
	// Observe the disconnect before reconnecting, so the delete and the re-add
	// cannot coalesce into a no-op and skip the reconnect path under test.
	eventuallyClusterCount(t, clusters, b, 0)
	uccs.UpdateObject(b)
	eventuallyClusterCount(t, clusters, b, 2)
	eventuallyClusterCount(t, clusters, a, 2)
}

// A client that stays connected must never be left permanently without its clusters
// while other clients and backends churn. Run with -race.
func TestPerClientClusters_ConcurrentChurnNeverStrandsStableClient(t *testing.T) {
	stable := clustersTestClient("role-stable")
	backendNames := []string{"b1", "b2", "b3", "b4"}
	backends := make([]*ir.BackendObjectIR, 0, len(backendNames))
	for _, n := range backendNames {
		backends = append(backends, clustersTestBackend(n))
	}
	uccs, finalBackends, clusters := newClustersTestFixture(t,
		[]ir.UniquelyConnectedClient{stable}, backends)
	eventuallyClusterCount(t, clusters, stable, len(backendNames))

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Churn transient clients: rapid connect/disconnect.
	for g := range 6 {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			c := clustersTestClient(fmt.Sprintf("role-churn-%d", g))
			for {
				select {
				case <-stop:
					return
				default:
				}
				uccs.UpdateObject(c)
				uccs.DeleteObject(c.ResourceName())
			}
		}(g)
	}
	// Churn a backend in parallel so per-client rows recompute under client
	// churn. Alternate a translated field: an identical resubmission is absorbed
	// by the base row's Equals and would exercise nothing.
	var lastProtocol string
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			lastProtocol = fmt.Sprintf("v%d", i%2)
			finalBackends.UpdateObject(clustersTestBackendWithProtocol("b4", lastProtocol))
		}
	})

	time.Sleep(750 * time.Millisecond)
	close(stop)
	wg.Wait()

	// The stable client must have all its backends once churn settles, and its
	// stored payload must reflect the last backend update.
	eventuallyClusterCount(t, clusters, stable, len(backendNames))
	require.Eventually(t, func() bool {
		c := storedClustersForClient(clusters, stable)[clustersTestBackend("b4").ClusterName()]
		return c != nil && c.GetAltStatName() == lastProtocol
	}, 5*time.Second, 10*time.Millisecond, "the stable client's payload must carry the last backend update")
}
