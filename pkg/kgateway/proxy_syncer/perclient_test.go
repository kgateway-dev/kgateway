package proxy_syncer

import (
	"errors"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
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

func TestFilterEndpointResourcesForStaticClusters_FiltersStaticClusterCLAs(t *testing.T) {
	// Clusters: one STATIC ("static-cluster"), one EDS ("eds-cluster").
	// Endpoints: CLAs for both. Expect only CLA for "eds-cluster" in result.
	clusters := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{Name: "static-cluster", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}}},
		{Resource: &envoyclusterv3.Cluster{Name: "eds-cluster", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}}},
	})
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "static-cluster"}},
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "eds-cluster"}},
	})

	out := filterEndpointResourcesForStaticClusters(clusters, endpoints)

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 endpoint resource, got %d", len(out.Items))
	}
	if _, ok := out.Items["eds-cluster"]; !ok {
		t.Errorf("expected CLA for eds-cluster to remain, got keys: %v", mapKeys(out.Items))
	}
	if _, ok := out.Items["static-cluster"]; ok {
		t.Error("expected CLA for static-cluster to be filtered out")
	}
}

func TestFilterEndpointResourcesForStaticClusters_ReturnsOriginalWhenNoFiltering(t *testing.T) {
	// Only EDS clusters; no STATIC. Endpoints should be returned unchanged (same reference when possible).
	clusters := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{Name: "eds-only", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}}},
	})
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "eds-only"}},
	})

	out := filterEndpointResourcesForStaticClusters(clusters, endpoints)

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 endpoint resource, got %d", len(out.Items))
	}
	if out.Version != endpoints.Version {
		t.Errorf("version: got %q, want %q", out.Version, endpoints.Version)
	}
	// Implementation returns original endpoints when nothing filtered
	if len(out.Items) != len(endpoints.Items) {
		t.Errorf("expected same count as input: got %d, want %d", len(out.Items), len(endpoints.Items))
	}
	if _, ok := out.Items["eds-only"]; !ok {
		t.Errorf("expected CLA for eds-only, got keys: %v", mapKeys(out.Items))
	}
}

func TestFilterEndpointResourcesForStaticClusters_EmptyClustersAndEndpoints(t *testing.T) {
	emptyClusters := envoycache.NewResourcesWithTTL("v1", nil)
	emptyEndpoints := envoycache.NewResourcesWithTTL("v1", nil)

	out := filterEndpointResourcesForStaticClusters(emptyClusters, emptyEndpoints)

	if len(out.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(out.Items))
	}
}

func TestFilterEndpointResourcesForStaticClusters_EmptyClustersNonEmptyEndpoints(t *testing.T) {
	emptyClusters := envoycache.NewResourcesWithTTL("v1", nil)
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "any"}},
	})

	out := filterEndpointResourcesForStaticClusters(emptyClusters, endpoints)

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 endpoint when no static clusters, got %d", len(out.Items))
	}
}

func TestFilterEndpointResourcesForStaticClusters_EmptyEndpoints(t *testing.T) {
	clusters := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{Name: "static-cluster", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}}},
	})
	emptyEndpoints := envoycache.NewResourcesWithTTL("v1", nil)

	out := filterEndpointResourcesForStaticClusters(clusters, emptyEndpoints)

	if len(out.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(out.Items))
	}
}

func TestFilterEndpointResourcesForStaticClusters_MixedStaticAndNonStatic(t *testing.T) {
	// Two STATIC, two EDS. CLAs for all four. Result should have only the two EDS CLAs.
	clusters := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{Name: "static-a", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}}},
		{Resource: &envoyclusterv3.Cluster{Name: "eds-a", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}}},
		{Resource: &envoyclusterv3.Cluster{Name: "static-b", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}}},
		{Resource: &envoyclusterv3.Cluster{Name: "eds-b", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}}},
	})
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "static-a"}},
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "eds-a"}},
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "static-b"}},
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "eds-b"}},
	})

	out := filterEndpointResourcesForStaticClusters(clusters, endpoints)

	if len(out.Items) != 2 {
		t.Fatalf("expected 2 endpoint resources (eds-a, eds-b), got %d: %v", len(out.Items), mapKeys(out.Items))
	}
	if _, ok := out.Items["eds-a"]; !ok {
		t.Errorf("expected CLA for eds-a, got keys: %v", mapKeys(out.Items))
	}
	if _, ok := out.Items["eds-b"]; !ok {
		t.Errorf("expected CLA for eds-b, got keys: %v", mapKeys(out.Items))
	}
	if _, ok := out.Items["static-a"]; ok {
		t.Error("expected static-a CLA to be filtered out")
	}
	if _, ok := out.Items["static-b"]; ok {
		t.Error("expected static-b CLA to be filtered out")
	}
}

func TestFilterEndpointResourcesForErroredClusters(t *testing.T) {
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "healthy-cluster"}},
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "errored-cluster"}},
		// This cluster is defined in Envoy's bootstrap, not in the dynamic CDS snapshot.
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "local-cluster"}},
	})

	out := filterEndpointResourcesForErroredClusters(endpoints, []string{"errored-cluster"})

	g := gomega.NewWithT(t)
	g.Expect(out.Items).To(gomega.HaveKey("healthy-cluster"))
	g.Expect(out.Items).To(gomega.HaveKey("local-cluster"),
		"filtering must not require every CLA to have a matching dynamic CDS resource")
	g.Expect(out.Items).ToNot(gomega.HaveKey("errored-cluster"))
	g.Expect(out.Version).ToNot(gomega.Equal(endpoints.Version),
		"entering the error state must change the EDS version")

	updatedEndpoints := endpoints
	updatedEndpoints.Version = "v2"
	updatedOut := filterEndpointResourcesForErroredClusters(updatedEndpoints, []string{"errored-cluster"})
	g.Expect(updatedOut.Version).ToNot(gomega.Equal(out.Version),
		"the filtered version must retain the underlying endpoint version")

	unchanged := filterEndpointResourcesForErroredClusters(endpoints, []string{"not-an-endpoint"})
	g.Expect(unchanged.Version).To(gomega.Equal(endpoints.Version))
	g.Expect(unchanged.Items).To(gomega.HaveLen(len(endpoints.Items)))

	recovered := filterEndpointResourcesForErroredClusters(endpoints, nil)
	g.Expect(recovered.Version).To(gomega.Equal(endpoints.Version),
		"leaving the error state must restore the original EDS version")
	g.Expect(recovered.Items).To(gomega.HaveKey("errored-cluster"))
}

// TestSnapshotPerClientStillPublishesWhenReferencedClusterErrored pins the
// fail-closed behavior for errored clusters: a cluster whose backend
// translation failed is excluded from the CDS payload (routes to it get
// 500/NC in Envoy) but does not prevent the rest of the snapshot from
// publishing.
func TestSnapshotPerClientStillPublishesWhenReferencedClusterErrored(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})

	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})
	routes := sliceToResources([]*envoyroutev3.RouteConfiguration{
		{
			Name: "route-config",
			VirtualHosts: []*envoyroutev3.VirtualHost{
				{
					Name:    "vhost",
					Domains: []string{"*"},
					Routes: []*envoyroutev3.Route{
						{
							Name: "single-route",
							Action: &envoyroutev3.Route_Route{
								Route: &envoyroutev3.RouteAction{
									ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "cluster-a"},
								},
							},
						},
						{
							Name: "errored-route",
							Action: &envoyroutev3.Route_Route{
								Route: &envoyroutev3.RouteAction{
									ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "cluster-b"},
								},
							},
						},
					},
				},
			},
		},
	})
	listeners := sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:         routes,
		Listeners:      listeners,
	}})
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		{
			Client:         ucc,
			Name:           "cluster-a",
			Cluster:        &envoyclusterv3.Cluster{Name: "cluster-a"},
			ClusterVersion: 1,
		},
		{
			Client:         ucc,
			Name:           "cluster-b",
			Cluster:        &envoyclusterv3.Cluster{Name: "cluster-b"},
			ClusterVersion: 2,
			Error:          errors.New("boom"),
		},
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
		PerClientEnvoyClusters{
			clusters: clusterCol,
			index: krtpkg.UnnamedIndex(clusterCol, func(cluster uccWithCluster) []string {
				return []string{cluster.Client.ResourceName()}
			}),
		},
	)

	g.Eventually(func() int {
		return len(snapshots.List())
	}, time.Second, 20*time.Millisecond).Should(gomega.Equal(1))

	snap := snapshots.List()[0]
	g.Expect(snap.erroredClusters).To(gomega.ConsistOf("cluster-b"))
	g.Expect(snap.snap.Resources[envoycachetypes.Cluster].Items).ToNot(gomega.HaveKey("cluster-b"))
}

// TestSnapshotPerClientPublishesEvenWithUnresolvableBackendRef verifies that
// a user BackendRef typo — e.g. an HTTPRoute pointing at a Service that does
// not exist — does not prevent the snapshot from publishing. IR-time
// resolution substitutes wellknown.BlackholeClusterName for the unresolved
// target, so valid routes still reach Envoy.
func TestSnapshotPerClientPublishesEvenWithUnresolvableBackendRef(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	routes := sliceToResources([]*envoyroutev3.RouteConfiguration{
		{
			Name: "route-config",
			VirtualHosts: []*envoyroutev3.VirtualHost{
				{
					Name:    "vhost",
					Domains: []string{"*"},
					Routes: []*envoyroutev3.Route{
						{
							Name: "good-route",
							Action: &envoyroutev3.Route_Route{
								Route: &envoyroutev3.RouteAction{
									ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "cluster-a"},
								},
							},
						},
						{
							// Simulates a BackendRef whose target Service does
							// not exist: IR translation substitutes blackhole.
							Name: "typo-backendref-route",
							Action: &envoyroutev3.Route_Route{
								Route: &envoyroutev3.RouteAction{
									ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: wellknown.BlackholeClusterName},
								},
							},
						},
					},
				},
			},
		},
	})
	listeners := sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:         routes,
		Listeners:      listeners,
	}})

	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		{
			Client:         ucc,
			Name:           "cluster-a",
			Cluster:        &envoyclusterv3.Cluster{Name: "cluster-a"},
			ClusterVersion: 1,
		},
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
		PerClientEnvoyClusters{
			clusters: clusterCol,
			index: krtpkg.UnnamedIndex(clusterCol, func(cluster uccWithCluster) []string {
				return []string{cluster.Client.ResourceName()}
			}),
		},
	)

	g.Eventually(func() int {
		return len(snapshots.List())
	}, 2*time.Second, 20*time.Millisecond).Should(gomega.Equal(1),
		"a snapshot must publish so Envoy can serve the good route even when another route's BackendRef is unresolvable")
}

// TestSnapshotPerClientKeepsPublishingWhenMisconfiguredBackendRefArrivesAtRuntime
// verifies that a BackendRef pointing at a nonexistent Service arriving after
// the control plane is already serving does not cause the per-client snapshot
// to be withdrawn. IR translation substitutes blackhole for the unresolved
// target, so the snapshot re-publishes with the new route set rather than
// being withdrawn. The existing good route keeps flowing; the typo'd route
// blackholes in Envoy — no 500/NC for valid traffic.
func TestSnapshotPerClientKeepsPublishingWhenMisconfiguredBackendRefArrivesAtRuntime(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	goodRoutes := sliceToResources([]*envoyroutev3.RouteConfiguration{
		{
			Name: "route-config",
			VirtualHosts: []*envoyroutev3.VirtualHost{{
				Name:    "vhost",
				Domains: []string{"*"},
				Routes: []*envoyroutev3.Route{{
					Name: "good-route",
					Action: &envoyroutev3.Route_Route{
						Route: &envoyroutev3.RouteAction{
							ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "cluster-a"},
						},
					},
				}},
			}},
		},
	})
	listeners := sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}})
	initial := GatewayXdsResources{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:         goodRoutes,
		Listeners:      listeners,
	}
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{initial})

	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		{
			Client:         ucc,
			Name:           "cluster-a",
			Cluster:        &envoyclusterv3.Cluster{Name: "cluster-a"},
			ClusterVersion: 1,
		},
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
		PerClientEnvoyClusters{
			clusters: clusterCol,
			index: krtpkg.UnnamedIndex(clusterCol, func(cluster uccWithCluster) []string {
				return []string{cluster.Client.ResourceName()}
			}),
		},
	)

	g.Eventually(func() int {
		return len(snapshots.List())
	}, time.Second, 20*time.Millisecond).Should(gomega.Equal(1))

	// A misconfigured route arrives whose BackendRef cannot be resolved;
	// IR translation substitutes blackhole.
	badRoutes := sliceToResources([]*envoyroutev3.RouteConfiguration{
		{
			Name: "route-config",
			VirtualHosts: []*envoyroutev3.VirtualHost{{
				Name:    "vhost",
				Domains: []string{"*"},
				Routes: []*envoyroutev3.Route{
					{
						Name: "good-route",
						Action: &envoyroutev3.Route_Route{
							Route: &envoyroutev3.RouteAction{
								ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "cluster-a"},
							},
						},
					},
					{
						Name: "typo-backendref-route",
						Action: &envoyroutev3.Route_Route{
							Route: &envoyroutev3.RouteAction{
								ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: wellknown.BlackholeClusterName},
							},
						},
					},
				},
			}},
		},
	})
	updated := initial
	updated.Routes = badRoutes
	mostXdsSnapshots.UpdateObject(updated)

	g.Consistently(func() int {
		return len(snapshots.List())
	}, 500*time.Millisecond, 20*time.Millisecond).Should(gomega.Equal(1),
		"snapshot must stay published after an unresolvable BackendRef arrives; withdrawing it strands Envoy on restart")
}

// TestSnapshotPerClientPublishesWithMissingOrUnusableEndpoints pins that endpoint
// readiness is deliberately *not* a publication precondition. Publication proceeds
// once the basic KRT inputs exist, even when a route-referenced EDS cluster has no
// ClusterLoadAssignment at all, or has one whose only endpoint is UNHEALTHY.
//
// This is the inverse of the whole-snapshot readiness gate that #13868 added and
// #14184 reverted: gating here could stay unsatisfied indefinitely, stranding warm
// clients on stale endpoints and starving new pods into crashloops. Envoy's initial
// fetch timeout covers the transient case of a CLA that has not landed yet.
//
// The first-connect delay in pkg/krtcollections/uniqueclients.go mitigates only
// initial/reconnect convergence. It is not a make-before-break guarantee for live
// route updates, and this test must not be relaxed into asserting one.
func TestSnapshotPerClientPublishesWithMissingOrUnusableEndpoints(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	routes := sliceToResources([]*envoyroutev3.RouteConfiguration{
		{
			Name: "route-config",
			VirtualHosts: []*envoyroutev3.VirtualHost{
				{
					Name:    "vhost",
					Domains: []string{"*"},
					Routes: []*envoyroutev3.Route{
						{
							Name: "route-to-cluster-without-cla",
							Action: &envoyroutev3.Route_Route{
								Route: &envoyroutev3.RouteAction{
									ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "cluster-no-cla"},
								},
							},
						},
						{
							Name: "route-to-cluster-with-unhealthy-cla",
							Action: &envoyroutev3.Route_Route{
								Route: &envoyroutev3.RouteAction{
									ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "cluster-unhealthy"},
								},
							},
						},
					},
				},
			},
		},
	})
	listeners := sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:         routes,
		Listeners:      listeners,
	}})

	edsCluster := func(name string) *envoyclusterv3.Cluster {
		return &envoyclusterv3.Cluster{
			Name:                 name,
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
		}
	}
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		{Client: ucc, Name: "cluster-no-cla", Cluster: edsCluster("cluster-no-cla"), ClusterVersion: 1},
		{Client: ucc, Name: "cluster-unhealthy", Cluster: edsCluster("cluster-unhealthy"), ClusterVersion: 2},
	})

	// cluster-no-cla intentionally has no row here at all.
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{
		{
			Client: ucc,
			Endpoints: &envoyendpointv3.ClusterLoadAssignment{
				ClusterName: "cluster-unhealthy",
				Endpoints: []*envoyendpointv3.LocalityLbEndpoints{
					{
						LbEndpoints: []*envoyendpointv3.LbEndpoint{
							{HealthStatus: envoycorev3.HealthStatus_UNHEALTHY},
						},
					},
				},
			},
			EndpointsHash: 3,
			endpointsName: "cluster-unhealthy",
		},
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
		PerClientEnvoyClusters{
			clusters: clusterCol,
			index: krtpkg.UnnamedIndex(clusterCol, func(cluster uccWithCluster) []string {
				return []string{cluster.Client.ResourceName()}
			}),
		},
	)

	g.Eventually(func() int {
		return len(snapshots.List())
	}, time.Second, 20*time.Millisecond).Should(gomega.Equal(1),
		"snapshot must publish even though a referenced EDS cluster has no CLA and another has only an UNHEALTHY endpoint")

	g.Consistently(func() int {
		return len(snapshots.List())
	}, 500*time.Millisecond, 20*time.Millisecond).Should(gomega.Equal(1),
		"snapshot must stay published; deferring on endpoint readiness can strand clients indefinitely (#14184)")

	snap := snapshots.List()[0]
	g.Expect(snap.erroredClusters).To(gomega.BeEmpty(), "neither cluster failed translation")

	clusterItems := snap.snap.Resources[envoycachetypes.Cluster].Items
	g.Expect(clusterItems).To(gomega.HaveKey("cluster-no-cla"),
		"a referenced EDS cluster with no CLA must still reach CDS")
	g.Expect(clusterItems).To(gomega.HaveKey("cluster-unhealthy"))

	endpointItems := snap.snap.Resources[envoycachetypes.Endpoint].Items
	g.Expect(endpointItems).To(gomega.HaveKey("cluster-unhealthy"),
		"an all-unhealthy CLA must be published, not withheld")
	g.Expect(endpointItems).ToNot(gomega.HaveKey("cluster-no-cla"),
		"no empty CLA is synthesized for a cluster whose endpoints have not landed; Envoy's initial fetch timeout covers it")
}

func mapKeys[M ~map[K]V, K comparable, V any](m M) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
