package proxy_syncer

import (
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// testClusterCols keeps the static collections backing a test-built
// PerClientEnvoyClusters alive and available to tests that need direct access.
type testClusterCols struct {
	bases   krt.StaticCollection[baseEnvoyCluster]
	deltas  krt.StaticCollection[backendClusterDeltaSet]
	clients krt.StaticCollection[ir.UniquelyConnectedClient]
}

// newTestPerClientClustersRaw builds a generation-consistent
// PerClientEnvoyClusters directly from base and sparse delta entries. clients
// must include every UCC that the caller will query; passing them explicitly
// models the production resolved-client fence.
func newTestPerClientClustersRaw(
	bases []baseEnvoyCluster,
	deltas []uccClusterDelta,
	clients ...ir.UniquelyConnectedClient,
) PerClientEnvoyClusters {
	clientSnapshot := newClientInputSnapshot(clients)
	deltaSets := make([]backendClusterDeltaSet, 0, len(bases))
	for _, base := range bases {
		set := backendClusterDeltaSet{
			Name:               base.Name,
			BaseFingerprint:    base.Fingerprint,
			ClientsFingerprint: clientSnapshot.Fingerprint,
			ResolvedClients:    clientSnapshot,
		}
		for _, delta := range deltas {
			if delta.Name != base.Name {
				continue
			}
			if set.Deltas == nil {
				set.Deltas = make(map[string]uccClusterDelta)
			}
			set.Deltas[delta.Client.ResourceName()] = delta
		}
		deltaSets = append(deltaSets, set)
	}

	baseCol := krt.NewStaticCollection[baseEnvoyCluster](nil, bases)
	deltaCol := krt.NewStaticCollection[backendClusterDeltaSet](nil, deltaSets)
	clientCol := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, clients)
	return PerClientEnvoyClusters{
		base:    baseCol,
		deltas:  deltaCol,
		clients: clientCol,
	}
}

// baseFromCluster projects a flat cluster entry onto a resolved shared base.
// The zero Fingerprint is deliberate and load-bearing: the delta sets built
// alongside it carry the same zero fingerprint, so a base row swapped in later
// (see testClusterCols.bases) still reads as the current generation.
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
// resolved shared base and each distinct client is included in the client set.
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

	clientSnapshot := newClientInputSnapshot(clients)
	deltaSets := make([]backendClusterDeltaSet, 0, len(bases))
	for _, base := range bases {
		deltaSets = append(deltaSets, backendClusterDeltaSet{
			Name:               base.Name,
			BaseFingerprint:    base.Fingerprint,
			ClientsFingerprint: clientSnapshot.Fingerprint,
			ResolvedClients:    clientSnapshot,
		})
	}

	baseCol := krt.NewStaticCollection[baseEnvoyCluster](nil, bases)
	deltaCol := krt.NewStaticCollection[backendClusterDeltaSet](nil, deltaSets)
	clientCol := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, clients)
	pcc := PerClientEnvoyClusters{base: baseCol, deltas: deltaCol, clients: clientCol}
	return pcc, &testClusterCols{bases: baseCol, deltas: deltaCol, clients: clientCol}
}

