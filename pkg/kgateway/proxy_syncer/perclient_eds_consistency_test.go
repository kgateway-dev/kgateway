package proxy_syncer

// EDS consistency regressions. The per-client CDS has a cluster for every
// backend, so an EDS cluster can lack a CLA, yet the built snapshot must pass
// Snapshot.Consistent(). filterEndpointResourcesForClusters drops orphan and
// STATIC CLAs and synthesizes missing ones. A referenced placeholder defers
// publication; a derived empty CLA does not. The gate may omit placeholders to
// keep held endpoints.

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

	// Drop the stale CLA and synthesize an empty one for the EDS cluster.
	clusters := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{
			Name:                 "eds-x",
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
		}},
	})
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "stale-gone"}},
	})

	out, _, _ := filterEndpointResourcesForClusters(clusters, endpoints, nil, "")

	g.Expect(out.Items).To(gomega.HaveKey("eds-x"), "EDS cluster without a CLA must get a synthesized assignment")
	g.Expect(out.Items).ToNot(gomega.HaveKey("stale-gone"), "CLA for a cluster absent from CDS must be dropped")
	cla, ok := out.Items["eds-x"].Resource.(*envoyendpointv3.ClusterLoadAssignment)
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(cla.GetEndpoints()).To(gomega.BeEmpty(), "the synthesized assignment is empty (active with no hosts)")
}

// An unreferenced EDS cluster without endpoints must still produce an
// EDS-consistent snapshot through the production assembly path.
func TestSnapshotPerClientPublishesConsistentSnapshotForUnreferencedEDSClusterWithoutEndpoints(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	// The listener uses this empty route config; no cluster is referenced.
	routes := routeResourcesForClusters()
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})

	// An unreferenced EDS cluster with no endpoint row.
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
		clusterScoping{},
	)

	snap := eventuallySingleSnapshot(t, snapshots)

	g.Expect(snap.Consistent()).ToNot(gomega.HaveOccurred(),
		"published snapshot must be EDS-consistent: the unreferenced EDS cluster gets a synthesized empty CLA")
	g.Expect(snap.Resources[envoycachetypes.Cluster].Items).To(gomega.HaveKey("cluster-0"))
	g.Expect(snap.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-0"),
		"the EDS cluster must have a (synthesized empty) ClusterLoadAssignment")
}
