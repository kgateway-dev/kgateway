package proxy_syncer

import (
	"maps"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/onsi/gomega"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

func TestGatewayXdsResourcesEqualsComparesRoutingTargets(t *testing.T) {
	original := GatewayXdsResources{ReferencedClusters: map[string]struct{}{"old": {}}}
	same := original
	same.ReferencedClusters = maps.Clone(original.ReferencedClusters)
	if !original.Equals(same) || !same.Equals(original) {
		t.Fatal("equal target sets must compare equal")
	}
	changed := original
	changed.ReferencedClusters = map[string]struct{}{"new": {}}
	if original.Equals(changed) || changed.Equals(original) {
		t.Fatal("routing target changes must reach publication even with equal resource versions")
	}
}

func TestPublicationRetainsOnlySupportedBootstrapEndpoints(t *testing.T) {
	for _, knowsLocal := range []bool{false, true} {
		name := "legacy"
		if knowsLocal {
			name = "capable"
		}
		t.Run(name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
			ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
			ucc.KnowsLocalCluster = knowsLocal
			localName, _, _ := ucc.LocalClusterInfo()
			clients := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})
			clusterRows := krt.NewStaticCollection[uccWithCluster](nil, nil)
			clusters := newTestPerClientClustersFromCol(clusterRows, clients)
			snapshots := snapshotPerClient(
				krtutil.NewKrtOptions(t.Context().Done(), nil), clients,
				krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"}}}),
				newTestPerClientEndpoints(nil), clusters,
				newTestPerClientEndpoints([]UccWithEndpoints{{
					Client: ucc, endpointsName: localName, resourceName: uccEndpointsResourceName(ucc, localName),
					Endpoints: sharedproto.Wrap(&envoyendpointv3.ClusterLoadAssignment{ClusterName: localName}), EndpointsHash: 1,
				}}),
			)
			g.Eventually(func() bool { return snapshots.GetKey(ucc.ResourceName()) != nil }, time.Second).Should(gomega.BeTrue())
			snapshot := snapshots.GetKey(ucc.ResourceName()).snap
			_, hasLocal := snapshot.Resources[envoycachetypes.Endpoint].Items[localName]
			if hasLocal != knowsLocal {
				t.Fatalf("local CLA present=%v, client support=%v", hasLocal, knowsLocal)
			}
			if err := snapshotConsistencyError(ucc.ResourceName(), snapshot); err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Resources[envoycachetypes.Cluster].Items) != 0 {
				t.Fatal("bootstrap cluster must not be inserted into dynamic CDS")
			}
		})
	}
}

func TestSnapshotConsistencyChecksBootstrapWithoutHidingOtherGaps(t *testing.T) {
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	localName, _, _ := ucc.LocalClusterInfo()
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: localName}},
	})
	if err := snapshotConsistencyError(ucc.ResourceName(), snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Resources[envoycachetypes.Cluster].Items) != 0 {
		t.Fatal("validation mutated the shared CDS map")
	}
	broken := *snap
	broken.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{Name: "missing", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}}},
	})
	if err := snapshotConsistencyError(ucc.ResourceName(), &broken); err == nil {
		t.Fatal("missing dynamic EDS assignment must still fail consistency")
	}
	if err := snapshotConsistencyError("unassociated-client", snap); err == nil {
		t.Fatal("unknown bootstrap resources must not be exempted")
	}
}

func newTestPerClientEndpoints(initial []UccWithEndpoints) PerClientEnvoyEndpoints {
	col := krt.NewStaticCollection[UccWithEndpoints](nil, initial)
	return PerClientEnvoyEndpoints{endpoints: col, index: krtpkg.UnnamedIndex(col, func(ep UccWithEndpoints) []string {
		return []string{ep.Client.ResourceName()}
	})}
}

func TestEndpointFilterErrorRecoveryAdvancesVersion(t *testing.T) {
	g := gomega.NewWithT(t)
	edsCluster := func(name string) envoycachetypes.ResourceWithTTL {
		return envoycachetypes.ResourceWithTTL{Resource: &envoyclusterv3.Cluster{Name: name, ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}}}
	}
	endpoints := envoycache.NewResourcesWithTTL("both-endpoints", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "healthy"}},
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "errored"}},
	})
	healthyOnly := envoycache.NewResourcesWithTTL("healthy-only", []envoycachetypes.ResourceWithTTL{edsCluster("healthy")})
	filtered, _ := filterEndpointResourcesForClusters(healthyOnly, endpoints)
	g.Expect(filtered.Items).To(gomega.HaveLen(1))
	g.Expect(filtered.Items).To(gomega.HaveKey("healthy"))
	g.Expect(filtered.Version).ToNot(gomega.Equal(endpoints.Version), "entering an error must trigger an EDS push")
	recovered := envoycache.NewResourcesWithTTL("recovered", []envoycachetypes.ResourceWithTTL{edsCluster("healthy"), edsCluster("errored")})
	restored, _ := filterEndpointResourcesForClusters(recovered, endpoints)
	g.Expect(restored.Items).To(gomega.HaveLen(2))
	g.Expect(restored.Version).To(gomega.Equal(endpoints.Version), "recovery must restore the original EDS version and trigger another push")
	g.Expect(endpoints.Items).To(gomega.HaveLen(2), "filtering must not mutate the upstream EDS map")
}
