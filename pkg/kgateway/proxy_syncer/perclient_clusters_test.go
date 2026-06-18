package proxy_syncer

import (
	"context"
	"fmt"
	"sort"
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
// CDS collection). These pin the invariant that the clusters returned for a
// connected client track (connected-client set) x (finalBackends) across any
// sequence of client/backend add/remove, plus that a client which stays connected
// is never left permanently without its clusters while other clients churn.
//
// Added while investigating #14184 (proxies stranded with empty per-client config).
// They document the behavior the collection must preserve and pass against the
// current implementation; they are not a reproduction of that issue.

func clustersTestTranslator() *irtranslator.BackendTranslator {
	return &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			{Group: "", Kind: "Service"}: {
				InitEnvoyBackend: func(_ context.Context, _ ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
	}
}

func clustersTestBackend(name string) *ir.BackendObjectIR {
	b := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "",
		Kind:      "Service",
		Namespace: "default",
		Name:      name,
	}, 443, "", "")
	return &b
}

func clustersTestClient(role string) ir.UniquelyConnectedClient {
	return ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
}

func clusterNamesForClient(c PerClientEnvoyClusters, ucc ir.UniquelyConnectedClient) []string {
	fetched := c.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
	names := make([]string, 0, len(fetched))
	for _, f := range fetched {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
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

// Removing a client clears its rows and leaves other clients untouched.
func TestPerClientClusters_ClientRemovedClearsRowsOthersUnaffected(t *testing.T) {
	a, b := clustersTestClient("role-a"), clustersTestClient("role-b")
	uccs, _, clusters := newClustersTestFixture(t,
		[]ir.UniquelyConnectedClient{a, b},
		[]*ir.BackendObjectIR{clustersTestBackend("b1"), clustersTestBackend("b2")},
	)
	eventuallyClusterCount(t, clusters, a, 2)
	eventuallyClusterCount(t, clusters, b, 2)

	uccs.DeleteObject(b.ResourceName())
	eventuallyClusterCount(t, clusters, b, 0)
	eventuallyClusterCount(t, clusters, a, 2)
}

// Each client's index entry returns only that client's clusters.
func TestPerClientClusters_IndexIsolation(t *testing.T) {
	a, b := clustersTestClient("role-a"), clustersTestClient("role-b")
	_, _, clusters := newClustersTestFixture(t,
		[]ir.UniquelyConnectedClient{a, b},
		[]*ir.BackendObjectIR{clustersTestBackend("b1"), clustersTestBackend("b2")},
	)
	eventuallyClusterCount(t, clusters, a, 2)
	for _, fc := range clusters.FetchClustersForClient(krt.TestingDummyContext{}, a) {
		require.Equal(t, a.ResourceName(), fc.Client.ResourceName(), "index leaked another client's row into client a")
	}
	for _, fc := range clusters.FetchClustersForClient(krt.TestingDummyContext{}, b) {
		require.Equal(t, b.ResourceName(), fc.Client.ResourceName(), "index leaked another client's row into client b")
	}
}

// Removing then re-adding the same client restores its full cluster set.
func TestPerClientClusters_ReAddClientRestoresRows(t *testing.T) {
	a, b := clustersTestClient("role-a"), clustersTestClient("role-b")
	uccs, _, clusters := newClustersTestFixture(t,
		[]ir.UniquelyConnectedClient{a, b},
		[]*ir.BackendObjectIR{clustersTestBackend("b1"), clustersTestBackend("b2")},
	)
	eventuallyClusterCount(t, clusters, b, 2)
	uccs.DeleteObject(b.ResourceName())
	eventuallyClusterCount(t, clusters, b, 0)
	uccs.UpdateObject(b)
	eventuallyClusterCount(t, clusters, b, 2)
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
	// Churn a backend in parallel so per-client rows recompute under client churn.
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			finalBackends.UpdateObject(clustersTestBackend("b4"))
		}
	})

	time.Sleep(750 * time.Millisecond)
	close(stop)
	wg.Wait()

	// The stable client must have all its backends once churn settles.
	eventuallyClusterCount(t, clusters, stable, len(backendNames))
}
