package proxy_syncer

// Tests for the per-cluster readiness resolution.
//
// snapshotPerClient no longer defers the whole snapshot when a referenced
// cluster is not ready; syncXds resolves the gaps per cluster against the
// currently-published snapshot:
//
//   - a cluster whose CLA row was derived publishes that row as the
//     backend's truth even when it is empty (scale-to-zero, #14352) — such
//     a snapshot is not deferred at all;
//   - a newly-referenced cluster whose CLA has NOT been derived holds only
//     the route flip (bounded by the publish budget); every other update
//     (other clusters' endpoints, the warming cluster's own CDS entry)
//     keeps publishing.

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

// TestSnapshotPerClientScaleToZeroPublishesEmptyCLA: when a previously-active
// cluster's Service scales to zero, the now-empty ClusterLoadAssignment
// publishes so Envoy stops routing to dead endpoints (the original "traffic
// to upstream endpoints which no longer exist" complaint, #14184). The
// derived-but-empty row is the backend's truth: the wrapper is not even
// deferred, so it publishes on the plain path with no resolution or budget
// involved (#14352).
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
	)

	cache := newTestSnapshotCache(t)
	registerSyncXds(snapshots, NewProxyTranslator(cache, nil, 0, true))
	nodeID := ucc.ResourceName()

	initialServed := eventuallyCacheSnapshot(t, cache, nodeID)
	g.Expect(initialServed.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-old"),
		"steady state: the active cluster's CLA is served")

	// The Service scales 3 -> 0: the CLA still exists but has no usable
	// endpoint.
	endpointCol.UpdateObject(endpointScaledToZero)

	// C2: truth wins. The empty CLA reaches the served cache so Envoy drops
	// the dead endpoints; the routes stay as they are (only that cluster's
	// traffic degrades, accurately).
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

// TestSnapshotPerClientWarmingClusterHoldsOnlyRouteFlip covers the isolation
// half of the flip hold: while a newly-referenced cluster's CLA has not been
// derived (a backend that never produces EndpointSlices, or derivation that
// has not caught up), only the route flip is held. The warming cluster's CDS
// entry and every unrelated update keep publishing — the old whole-snapshot
// gate stranded them all behind the one unready backend. (These tests run
// with budget 0, so the hold itself is unbounded here; the flip-release
// bound is covered in publish_gate_test.go.)
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
	)

	cache := newTestSnapshotCache(t)
	registerSyncXds(snapshots, NewProxyTranslator(cache, nil, 0, true))
	nodeID := ucc.ResourceName()

	initialServed := eventuallyCacheSnapshot(t, cache, nodeID)
	g.Expect(initialServed.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-old"))
	initialEndpointVersion := initialServed.Resources[envoycachetypes.Endpoint].Version
	initialRouteVersion := initialServed.Resources[envoycachetypes.Route].Version

	// Routes now additionally reference cluster-new, a backend whose CLA is
	// never derived (an ExternalName-like backend with no EndpointSlices, or
	// derivation that has not caught up). Its cluster arrives; a synthesized
	// empty CLA stands in indefinitely. Meanwhile cluster-old's endpoints
	// change.
	updatedRoutes := routeResourcesForClusters("cluster-old", "cluster-new")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.UpdateObject(edsClusterForClient(ucc, "cluster-new", 3))
	endpointCol.UpdateObject(endpointsForClient(ucc, "cluster-old", 5))

	// C3 isolation: only the flip onto cluster-new is held. The warming
	// cluster reaches the served CDS, and cluster-old's endpoint update
	// publishes — nothing is stranded behind the unready backend.
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

// TestSnapshotPerClientFlipReleasesOnByteIdenticalDerivedEmptyCLA pins the
// hardest release trigger for a held flip: the blocking cluster's CLA derives
// EMPTY, which is byte-identical to the synthesized placeholder already in
// the snapshot, so every per-type version — including the recomputed EDS
// version, which hashes CLA protos — is unchanged. The only observable
// change is the wrapper's gap classification, so if Equals does not compare
// it, KRT suppresses the event and the flip stays held forever (the test
// runs with budget 0, so no timer can mask the miss).
//
// The errored cluster exists to keep filterEndpointResourcesForClusters on
// its recomputed-version branch in BOTH builds (its derived CLA is dropped
// every time because the errored cluster is excluded from CDS); without it
// the post-derivation build takes the returns-original branch, whose version
// string comes from a different hash scheme and changes incidentally.
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
		// The errored cluster's derived CLA: dropped by the EDS filter on
		// every build, pinning the recomputed-version branch.
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
	)

	cache := newTestSnapshotCache(t)
	registerSyncXds(snapshots, NewProxyTranslator(cache, nil, 0, true))
	nodeID := ucc.ResourceName()

	initialServed := eventuallyCacheSnapshot(t, cache, nodeID)
	g.Expect(snapshotReferencesCluster(initialServed, "cluster-old")).To(gomega.BeTrue())

	// The flip: routes retarget onto cluster-new, whose CLA has not been
	// derived, so resolution holds the served routes at cluster-old.
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

	// cluster-new's CLA derives, empty — byte-identical to the synthesized
	// placeholder, so no per-type version changes. The classification change
	// alone must release the flip.
	endpointCol.UpdateObject(emptyEndpointsForClient(ucc, "cluster-new", 5))

	g.Eventually(func() bool {
		released := eventuallyCacheSnapshot(t, cache, nodeID)
		return snapshotReferencesCluster(released, "cluster-new")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"a derived-but-empty CLA is the backend's truth; the held flip must release even though no snapshot version changed")
	assertSnapshotCoherent(t, eventuallyCacheSnapshot(t, cache, nodeID))
}

// TestSnapshotPerClientErroredClusterIsNotCarriedDuringHeldFlip pins the
// fail-closed rule for errored clusters: even while a route flip is held for
// an unrelated warming cluster, a previously-referenced cluster whose current
// translation is errored is dropped from the served CDS instead of being
// carried forward from the published snapshot. Serving it with its stale
// (pre-error) config would silently bypass the policy whose failure errored
// it — Gateway API conformance requires requests to a backend targeted by an
// invalid BackendTLSPolicy to receive a 5xx
// (BackendTLSPolicyInvalidCACertificateRef); the fail-open variant was
// rejected in PR #13976.
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
	)

	cache := newTestSnapshotCache(t)
	registerSyncXds(snapshots, NewProxyTranslator(cache, nil, 0, true))
	nodeID := ucc.ResourceName()

	initialServed := eventuallyCacheSnapshot(t, cache, nodeID)
	g.Expect(initialServed.Resources[envoycachetypes.Cluster].Items).To(gomega.HaveKey("cluster-old"))

	// Simultaneously: routes retarget to additionally reference cluster-new
	// (whose CLA is never derived, so the flip is held), and cluster-old's
	// translation goes errored (e.g. its BackendTLSPolicy became invalid).
	updatedRoutes := routeResourcesForClusters("cluster-old", "cluster-new")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.UpdateObject(edsClusterForClient(ucc, "cluster-new", 3))
	erroredOld := clusterOld
	erroredOld.Error = errors.New("backend tls policy references a nonexistent ca certificate")
	// Mirror production versioning: baseClusterVersion returns 0 for an
	// errored translation, which is what propagates the ok->errored
	// transition through the version-based delta equality.
	erroredOld.ClusterVersion = 0
	clusterCol.UpdateObject(erroredOld)

	// Fail closed: the errored cluster must leave the served CDS (its held
	// routes 5xx) even though the flip is held for cluster-new; the warming
	// cluster still reaches the served CDS. The served snapshot legitimately
	// contains a route to the dropped errored cluster, so the coherence
	// assertion does not apply here.
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
