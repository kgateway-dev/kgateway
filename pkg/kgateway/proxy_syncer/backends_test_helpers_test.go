package proxy_syncer

import (
	"context"
	"slices"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// testClusterCols keeps the static collections backing a test-built
// PerClientEnvoyClusters alive and available to tests that need direct access.
type testClusterCols struct {
	bases krt.StaticCollection[baseEnvoyCluster]
}

// baseFromCluster projects a flat cluster entry onto a shared base row. Backend
// and Base are left nil: such a row has nothing to overlay, so every client
// publishes it as-is.
func baseFromCluster(cluster uccWithCluster) baseEnvoyCluster {
	return baseEnvoyCluster{
		Name:              cluster.Name,
		Cluster:           cluster.Cluster,
		ClusterVersion:    cluster.ClusterVersion,
		Error:             cluster.Error,
		BackendSource:     cluster.BackendSource,
		BackendGeneration: cluster.BackendGeneration,
	}
}

// newTestPerClientClusters builds a PerClientEnvoyClusters from flat cluster
// entries. These snapshot tests do not exercise overlays, so each entry is a
// shared base, and each distinct client named by an entry is a connected client.
func newTestPerClientClusters(initial []uccWithCluster) (PerClientEnvoyClusters, *testClusterCols) {
	basesByName := make(map[string]baseEnvoyCluster)
	clientsByName := make(map[string]ir.UniquelyConnectedClient)
	for _, cluster := range initial {
		basesByName[cluster.Name] = baseFromCluster(cluster)
		clientsByName[cluster.Client.ResourceName()] = cluster.Client
	}

	bases := make([]baseEnvoyCluster, 0, len(basesByName))
	for _, base := range basesByName {
		bases = append(bases, base)
	}
	clients := make([]ir.UniquelyConnectedClient, 0, len(clientsByName))
	for _, client := range clientsByName {
		clients = append(clients, client)
	}

	baseCol := krt.NewStaticCollection[baseEnvoyCluster](nil, bases)
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, clients)
	pcc := newPerClientEnvoyClusters(context.Background(), krtutil.KrtOptions{}, nil, baseCol, uccs)
	return pcc, &testClusterCols{bases: baseCol}
}

// storedClustersForClient returns the clusters in the client's stored CDS
// payload, keyed by name, or nil when the client has no row yet. Unlike
// FetchClustersForClient, which computes the view on demand, this observes what
// KRT has actually propagated, so tests of event ordering and churn use it.
func storedClustersForClient(c PerClientEnvoyClusters, ucc ir.UniquelyConnectedClient) map[string]*envoyclusterv3.Cluster {
	if c.perClient == nil {
		return nil
	}
	row := c.perClient.GetKey(ucc.ResourceName())
	if row == nil {
		return nil
	}
	out := make(map[string]*envoyclusterv3.Cluster, len(row.clusters.Items))
	for name, item := range row.clusters.Items {
		if cluster, ok := item.Resource.(*envoyclusterv3.Cluster); ok {
			out[name] = cluster
		}
	}
	return out
}

// storedClusterNamesForClient is storedClustersForClient reduced to sorted
// names, for tests that only care which clusters a client holds.
func storedClusterNamesForClient(c PerClientEnvoyClusters, ucc ir.UniquelyConnectedClient) []string {
	stored := storedClustersForClient(c, ucc)
	names := make([]string, 0, len(stored))
	for name := range stored {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
