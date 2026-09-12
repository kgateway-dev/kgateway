package proxy_syncer

// Regression tests for EDS snapshot consistency. The per-client CDS includes a
// cluster for every backend, referenced or not, so an EDS cluster can be in
// CDS before (or without) its ClusterLoadAssignment. The published snapshot
// must still satisfy go-control-plane's Snapshot.Consistent() invariant: every
// EDS cluster has exactly one CLA and there are no CLAs without a cluster.
// filterEndpointResourcesForClusters drops stale/STATIC CLAs and synthesizes
// empty assignments for EDS clusters that have no CLA yet; referenced clusters
// backed by a synthesized empty mark the wrapper deferred (presence, not
// contents), and the empties reach Envoy for unreferenced clusters and on the
// bounded publish paths (see publishGate).

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/onsi/gomega"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

func TestFilterEndpointResourcesForClusters_SynthesizesEmptyCLAForEDSClusterWithoutCLA(t *testing.T) {
	g := gomega.NewWithT(t)

	// One EDS cluster in CDS; one stale CLA whose cluster is gone. Expect: the
	// EDS cluster gets a synthesized empty CLA, and the stale CLA is dropped.
	clusters := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{
			Name:                 "eds-x",
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
		}},
	})
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "stale-gone"}},
	})

	out, _ := filterEndpointResourcesForClusters(clusters, endpoints)

	g.Expect(out.Items).To(gomega.HaveKey("eds-x"), "EDS cluster without a CLA must get a synthesized assignment")
	g.Expect(out.Items).ToNot(gomega.HaveKey("stale-gone"), "CLA for a cluster absent from CDS must be dropped")
	cla, ok := out.Items["eds-x"].Resource.(*envoyendpointv3.ClusterLoadAssignment)
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(cla.GetEndpoints()).To(gomega.BeEmpty(), "the synthesized assignment is empty (active with no hosts)")
}

// TestSnapshotPerClientPublishesConsistentSnapshotForUnreferencedEDSClusterWithoutEndpoints
// exercises the production path: an EDS cluster lands in the per-client CDS
// that no route references and has no endpoints. The published snapshot must
// still be EDS-consistent (Snapshot.Consistent() passes), which before this
// change it was not.
func TestSnapshotPerClientPublishesConsistentSnapshotForUnreferencedEDSClusterWithoutEndpoints(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	// A route config the listener references, but with no routes (so no cluster
	// is referenced).
	routes := routeResourcesForClusters()
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})

	// An unreferenced EDS cluster in CDS, with no endpoints anywhere.
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		edsClusterForClient(ucc, "cluster-0", 1),
	})
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, nil)

	snapshots := snapshotPerClient(
		krtutil.KrtOptions{},
		uccs,
		mostXdsSnapshots,
		PerClientEnvoyEndpoints{
			endpoints: endpointCol,
			index: krtpkg.UnnamedIndex(endpointCol, func(ep UccWithEndpoints) []string {
				return []string{ep.Client.ResourceName()}
			}),
		},
		newTestPerClientClustersFromCol(clusterCol, uccs),
	)

	snap := eventuallySingleSnapshot(t, snapshots)

	g.Expect(snap.Consistent()).ToNot(gomega.HaveOccurred(),
		"published snapshot must be EDS-consistent: the unreferenced EDS cluster gets a synthesized empty CLA")
	g.Expect(snap.Resources[envoycachetypes.Cluster].Items).To(gomega.HaveKey("cluster-0"))
	g.Expect(snap.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-0"),
		"the EDS cluster must have a (synthesized empty) ClusterLoadAssignment")
}
