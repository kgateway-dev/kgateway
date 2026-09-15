package proxy_syncer

// Per-cluster resolution tests. A derived CLA publishes even when empty and does
// not defer the snapshot (#14352). An underived new target holds routes,
// listeners and secrets up to the budget while CDS and EDS keep publishing.

import (
	"errors"
	"testing"
	"time"

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

// Scale-to-zero publishes the derived empty CLA without deferral, so Envoy stops
// routing to dead endpoints (#14184, #14352).
func TestSnapshotPerClientScaleToZeroPublishesEmptyCLA(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	routes := routeResourcesForClusters("cluster-old")
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})

	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		edsClusterForClient(ucc, "cluster-old", 1),
	})
	endpointReady := endpointsForClient(ucc, "cluster-old", 2)
	endpointScaledToZero := emptyEndpointsForClient(ucc, "cluster-old", 3)
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{endpointReady})

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

	cache := newTestSnapshotCache(t)
	registerSyncXds(snapshots, NewProxyTranslator(cache, nil, 0, true))
	nodeID := ucc.ResourceName()

	initialServed := eventuallyCacheSnapshot(t, cache, nodeID)
	g.Expect(initialServed.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-old"),
		"steady state: the active cluster's CLA is served")

	// Scale to zero; the CLA stays derived, now empty.
	endpointCol.UpdateObject(endpointScaledToZero)

	// Envoy gets the empty CLA and drops the dead endpoints; routes are unchanged.
	g.Eventually(func() bool {
		resourceSnapshot, err := cache.GetSnapshot(nodeID)
		if err != nil {
			return false
		}
		snap, ok := resourceSnapshot.(*envoycache.Snapshot)
		if !ok {
			return false
		}
		item, ok := snap.Resources[envoycachetypes.Endpoint].Items["cluster-old"]
		if !ok {
			return false
		}
		cla, ok := item.Resource.(*envoyendpointv3.ClusterLoadAssignment)
		return ok && len(cla.GetEndpoints()) == 0
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"the empty CLA must publish so Envoy stops routing to endpoints that no longer exist")
	served := eventuallyCacheSnapshot(t, cache, nodeID)
	g.Expect(snapshotReferencesCluster(served, "cluster-old")).To(gomega.BeTrue(),
		"the routes are unchanged; only the cluster's endpoint truth changed")
	assertSnapshotCoherent(t, served)
}

// An underived new target holds only the flip; the warming cluster's CDS entry
// and unrelated updates keep publishing. Budget 0 disables the release here;
// publish_gate_test.go covers it.
func TestSnapshotPerClientWarmingClusterHoldsOnlyRouteFlip(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	initialRoutes := routeResourcesForClusters("cluster-old")
	initial := GatewayXdsResources{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             initialRoutes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(initialRoutes, listeners),
	}
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{initial})

	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		edsClusterForClient(ucc, "cluster-old", 1),
	})
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{
		endpointsForClient(ucc, "cluster-old", 2),
	})

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

	cache := newTestSnapshotCache(t)
	registerSyncXds(snapshots, NewProxyTranslator(cache, nil, 0, true))
	nodeID := ucc.ResourceName()

	initialServed := eventuallyCacheSnapshot(t, cache, nodeID)
	g.Expect(initialServed.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-old"))
	initialEndpointVersion := initialServed.Resources[envoycachetypes.Endpoint].Version
	initialRouteVersion := initialServed.Resources[envoycachetypes.Route].Version

	// Add cluster-new, whose CLA never derives, and change cluster-old's endpoints.
	updatedRoutes := routeResourcesForClusters("cluster-old", "cluster-new")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.UpdateObject(edsClusterForClient(ucc, "cluster-new", 3))
	endpointCol.UpdateObject(endpointsForClient(ucc, "cluster-old", 5))

	// Hold the flip while publishing cluster-new and cluster-old's endpoint update.
	var heldServed *envoycache.Snapshot
	g.Eventually(func() bool {
		heldServed = eventuallyCacheSnapshot(t, cache, nodeID)
		return hasResource(heldServed.Resources[envoycachetypes.Cluster].Items, "cluster-new") &&
			heldServed.Resources[envoycachetypes.Endpoint].Version != initialEndpointVersion
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"the warming cluster and the unrelated endpoint update must both reach the served cache")
	g.Expect(snapshotReferencesCluster(heldServed, "cluster-new")).To(gomega.BeFalse(),
		"the route flip onto the unready cluster is held")
	g.Expect(snapshotReferencesCluster(heldServed, "cluster-old")).To(gomega.BeTrue(),
		"the previously-served routes keep flowing traffic")
	g.Expect(heldServed.Resources[envoycachetypes.Route].Version).To(gomega.Equal(initialRouteVersion),
		"held routes keep their published version")
	assertSnapshotCoherent(t, heldServed)
}

// TestSnapshotPerClientFlipReleasesOnByteIdenticalDerivedEmptyCLA: a held flip
// releases once its blocking cluster's CLA derives empty, byte-identical to the
// synthesized placeholder. With budget 0 only that build can release it, not a
// timer. The derivation changes the EDS version; Equals also compares the gap
// classification, which TestXdsSnapWrapperEquals_ComparesGapClassification
// pins directly.
//
// The errored cluster's CLA is dropped from every build, so both builds take the
// general EDS filtering path.
func TestSnapshotPerClientFlipReleasesOnByteIdenticalDerivedEmptyCLA(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	initialRoutes := routeResourcesForClusters("cluster-old")
	initial := GatewayXdsResources{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             initialRoutes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(initialRoutes, listeners),
	}
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{initial})

	erroredCluster := edsClusterForClient(ucc, "cluster-err", 0)
	erroredCluster.Error = errors.New("translation failed")
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		edsClusterForClient(ucc, "cluster-old", 1),
		erroredCluster,
	})
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{
		endpointsForClient(ucc, "cluster-old", 2),
		// The errored cluster's CLA is dropped every build, pinning the
		// recomputed-version branch.
		endpointsForClient(ucc, "cluster-err", 3),
	})

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

	cache := newTestSnapshotCache(t)
	registerSyncXds(snapshots, NewProxyTranslator(cache, nil, 0, true))
	nodeID := ucc.ResourceName()

	initialServed := eventuallyCacheSnapshot(t, cache, nodeID)
	g.Expect(snapshotReferencesCluster(initialServed, "cluster-old")).To(gomega.BeTrue())

	// Retarget to underived cluster-new; resolution holds routes at cluster-old.
	updatedRoutes := routeResourcesForClusters("cluster-new")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.UpdateObject(edsClusterForClient(ucc, "cluster-new", 4))

	var heldServed *envoycache.Snapshot
	g.Eventually(func() bool {
		heldServed = eventuallyCacheSnapshot(t, cache, nodeID)
		return snapshotReferencesCluster(heldServed, "cluster-old") &&
			!snapshotReferencesCluster(heldServed, "cluster-new") &&
			hasResource(heldServed.Resources[envoycachetypes.Cluster].Items, "cluster-new")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"the flip onto the underived cluster must be held")

	// cluster-new's CLA derives empty, byte-identical to the placeholder; the
	// held flip must release.
	endpointCol.UpdateObject(emptyEndpointsForClient(ucc, "cluster-new", 5))

	g.Eventually(func() bool {
		released := eventuallyCacheSnapshot(t, cache, nodeID)
		return snapshotReferencesCluster(released, "cluster-new")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"a derived-but-empty CLA is the backend's truth; the held flip must release even though no snapshot version changed")
	assertSnapshotCoherent(t, eventuallyCacheSnapshot(t, cache, nodeID))
}

// Errored clusters fail closed, even during an unrelated held flip. Carrying
// their pre-error config would bypass the failed policy; an invalid
// BackendTLSPolicy must 5xx (BackendTLSPolicyInvalidCACertificateRef; fail-open
// was rejected in #13976).
func TestSnapshotPerClientErroredClusterIsNotCarriedDuringHeldFlip(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	initialRoutes := routeResourcesForClusters("cluster-old")
	initial := GatewayXdsResources{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             initialRoutes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(initialRoutes, listeners),
	}
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{initial})

	clusterOld := edsClusterForClient(ucc, "cluster-old", 1)
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{clusterOld})
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{
		endpointsForClient(ucc, "cluster-old", 2),
	})

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

	cache := newTestSnapshotCache(t)
	registerSyncXds(snapshots, NewProxyTranslator(cache, nil, 0, true))
	nodeID := ucc.ResourceName()

	initialServed := eventuallyCacheSnapshot(t, cache, nodeID)
	g.Expect(initialServed.Resources[envoycachetypes.Cluster].Items).To(gomega.HaveKey("cluster-old"))

	// Add underived cluster-new (flip held) while cluster-old's translation errors.
	updatedRoutes := routeResourcesForClusters("cluster-old", "cluster-new")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.UpdateObject(edsClusterForClient(ucc, "cluster-new", 3))
	erroredOld := clusterOld
	erroredOld.Error = errors.New("backend tls policy references a nonexistent ca certificate")
	// Match production: baseClusterVersion returns 0 on errors, making the
	// ok-to-error transition visible to version-based equality.
	erroredOld.ClusterVersion = 0
	clusterCol.UpdateObject(erroredOld)

	// The errored cluster leaves CDS (its routes 5xx) while the warming cluster
	// arrives. Held routes still name it, so skip assertSnapshotCoherent.
	var heldServed *envoycache.Snapshot
	g.Eventually(func() bool {
		heldServed = eventuallyCacheSnapshot(t, cache, nodeID)
		clusters := heldServed.Resources[envoycachetypes.Cluster].Items
		return hasResource(clusters, "cluster-new") && !hasResource(clusters, "cluster-old")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"the errored cluster must not be carried forward; the warming cluster still publishes")
	g.Expect(heldServed.Resources[envoycachetypes.Endpoint].Items).ToNot(gomega.HaveKey("cluster-old"),
		"the errored cluster's CLA leaves with it")
	g.Expect(snapshotReferencesCluster(heldServed, "cluster-old")).To(gomega.BeTrue(),
		"the held routes still name the errored cluster, which now 5xxes (fail closed)")
	g.Expect(snapshotReferencesCluster(heldServed, "cluster-new")).To(gomega.BeFalse(),
		"the flip onto the warming cluster remains held")
}
