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

// These tests pin the contract of NewPerClientEnvoyClusters: the set of clusters
// returned for a connected client must track (uccCol membership) x (finalBackends),
// across any sequence of client/backend add/remove. They are the regression net for
// the #14184 re-keying (per-client clusters are primary-keyed on the connected-client
// collection). The concurrent-churn test additionally targets the stranding race: a
// client that remains connected must never be left permanently without its clusters
// while other clients churn.

func rekeyStubTranslator() *irtranslator.BackendTranslator {
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

func rekeyBackend(name string) *ir.BackendObjectIR {
	b := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "",
		Kind:      "Service",
		Namespace: "default",
		Name:      name,
	}, 443, "")
	return &b
}

func rekeyClient(role string) ir.UniqlyConnectedClient {
	return ir.NewUniqlyConnectedClient(role, "", nil, ir.PodLocality{})
}

func clusterNamesForClient(c PerClientEnvoyClusters, ucc ir.UniqlyConnectedClient) []string {
	fetched := c.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
	names := make([]string, 0, len(fetched))
	for _, f := range fetched {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

func newRekeyFixture(
	t *testing.T,
	clients []ir.UniqlyConnectedClient,
	backends []*ir.BackendObjectIR,
) (krt.StaticCollection[ir.UniqlyConnectedClient], krt.StaticCollection[*ir.BackendObjectIR], PerClientEnvoyClusters) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	uccs := krt.NewStaticCollection(nil, clients, krtopts.ToOptions("UniqueClients")...)
	finalBackends := krt.NewStaticCollection(nil, backends, krtopts.ToOptions("FinalBackends")...)
	clusters := NewPerClientEnvoyClusters(ctx, krtopts, rekeyStubTranslator(), finalBackends, uccs)
	return uccs, finalBackends, clusters
}

func eventuallyClusterCount(t *testing.T, c PerClientEnvoyClusters, ucc ir.UniqlyConnectedClient, want int) {
	t.Helper()
	require.Eventuallyf(t, func() bool {
		return len(clusterNamesForClient(c, ucc)) == want
	}, 5*time.Second, 10*time.Millisecond,
		"client %q never reached %d clusters (last: %v)", ucc.ResourceName(), want, clusterNamesForClient(c, ucc))
}

// A single connected client receives a cluster for every backend.
func TestRekeyClusters_ClientGetsAllBackends(t *testing.T) {
	ucc := rekeyClient("role-a")
	_, _, clusters := newRekeyFixture(t,
		[]ir.UniqlyConnectedClient{ucc},
		[]*ir.BackendObjectIR{rekeyBackend("b1"), rekeyBackend("b2")},
	)
	eventuallyClusterCount(t, clusters, ucc, 2)
}

// The core #14184 property: a client that connects AFTER the collection is built
// must still get clusters for every backend.
func TestRekeyClusters_NewClientGetsClustersAfterConnect(t *testing.T) {
	first := rekeyClient("role-first")
	uccs, _, clusters := newRekeyFixture(t,
		[]ir.UniqlyConnectedClient{first},
		[]*ir.BackendObjectIR{rekeyBackend("b1"), rekeyBackend("b2"), rekeyBackend("b3")},
	)
	eventuallyClusterCount(t, clusters, first, 3)

	late := rekeyClient("role-late")
	uccs.UpdateObject(late)
	eventuallyClusterCount(t, clusters, late, 3)
	// existing client unaffected
	eventuallyClusterCount(t, clusters, first, 3)
}

// Adding a backend propagates to every connected client.
func TestRekeyClusters_BackendAddedPropagatesToAllClients(t *testing.T) {
	a, b := rekeyClient("role-a"), rekeyClient("role-b")
	_, finalBackends, clusters := newRekeyFixture(t,
		[]ir.UniqlyConnectedClient{a, b},
		[]*ir.BackendObjectIR{rekeyBackend("b1")},
	)
	eventuallyClusterCount(t, clusters, a, 1)
	eventuallyClusterCount(t, clusters, b, 1)

	finalBackends.UpdateObject(rekeyBackend("b2"))
	eventuallyClusterCount(t, clusters, a, 2)
	eventuallyClusterCount(t, clusters, b, 2)
}

// Removing a client clears its rows and leaves other clients untouched.
func TestRekeyClusters_ClientRemovedClearsRowsOthersUnaffected(t *testing.T) {
	a, b := rekeyClient("role-a"), rekeyClient("role-b")
	uccs, _, clusters := newRekeyFixture(t,
		[]ir.UniqlyConnectedClient{a, b},
		[]*ir.BackendObjectIR{rekeyBackend("b1"), rekeyBackend("b2")},
	)
	eventuallyClusterCount(t, clusters, a, 2)
	eventuallyClusterCount(t, clusters, b, 2)

	uccs.DeleteObject(b.ResourceName())
	eventuallyClusterCount(t, clusters, b, 0)
	eventuallyClusterCount(t, clusters, a, 2)
}

// Each client's index entry returns only that client's clusters.
func TestRekeyClusters_IndexIsolation(t *testing.T) {
	a, b := rekeyClient("role-a"), rekeyClient("role-b")
	_, _, clusters := newRekeyFixture(t,
		[]ir.UniqlyConnectedClient{a, b},
		[]*ir.BackendObjectIR{rekeyBackend("b1"), rekeyBackend("b2")},
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
func TestRekeyClusters_ReAddClientRestoresRows(t *testing.T) {
	a, b := rekeyClient("role-a"), rekeyClient("role-b")
	uccs, _, clusters := newRekeyFixture(t,
		[]ir.UniqlyConnectedClient{a, b},
		[]*ir.BackendObjectIR{rekeyBackend("b1"), rekeyBackend("b2")},
	)
	eventuallyClusterCount(t, clusters, b, 2)
	uccs.DeleteObject(b.ResourceName())
	eventuallyClusterCount(t, clusters, b, 0)
	uccs.UpdateObject(b)
	eventuallyClusterCount(t, clusters, b, 2)
}

// Stranding race: a client that stays connected must never be left permanently
// without its clusters while other clients and backends churn. Run with -race.
func TestRekeyClusters_ConcurrentChurnNeverStrandsStableClient(t *testing.T) {
	stable := rekeyClient("role-stable")
	backendNames := []string{"b1", "b2", "b3", "b4"}
	backends := make([]*ir.BackendObjectIR, 0, len(backendNames))
	for _, n := range backendNames {
		backends = append(backends, rekeyBackend(n))
	}
	uccs, finalBackends, clusters := newRekeyFixture(t,
		[]ir.UniqlyConnectedClient{stable}, backends)
	eventuallyClusterCount(t, clusters, stable, len(backendNames))

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Churn transient clients: rapid connect/disconnect of identical-then-gone clients.
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			c := rekeyClient(fmt.Sprintf("role-churn-%d", g))
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
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			finalBackends.UpdateObject(rekeyBackend("b4"))
		}
	}()

	time.Sleep(750 * time.Millisecond)
	close(stop)
	wg.Wait()

	// The stable client must have all its backends once churn settles. Permanent
	// stranding (the #14184 failure) shows up here as a count that never recovers.
	eventuallyClusterCount(t, clusters, stable, len(backendNames))
}
