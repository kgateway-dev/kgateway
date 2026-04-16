package proxy_syncer

import (
	"errors"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/wrapperspb"
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

func TestSnapshotPerClientDefersUntilAllReferencedClustersAreReady(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniqlyConnectedClient(role, "", nil, ir.PodLocality{})

	uccs := krt.NewStaticCollection[ir.UniqlyConnectedClient](nil, []ir.UniqlyConnectedClient{ucc})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes: sliceToResources([]*envoyroutev3.RouteConfiguration{
			{
				Name: "route-config",
				VirtualHosts: []*envoyroutev3.VirtualHost{
					{
						Name:    "vhost",
						Domains: []string{"*"},
						Routes: []*envoyroutev3.Route{
							{
								Name: "weighted-route",
								Action: &envoyroutev3.Route_Route{
									Route: &envoyroutev3.RouteAction{
										ClusterSpecifier: &envoyroutev3.RouteAction_WeightedClusters{
											WeightedClusters: &envoyroutev3.WeightedCluster{
												Clusters: []*envoyroutev3.WeightedCluster_ClusterWeight{
													{Name: "cluster-a", Weight: wrapperspb.UInt32(1)},
													{Name: "cluster-b", Weight: wrapperspb.UInt32(1)},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}),
		Listeners: sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}}),
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

	g.Consistently(func() int {
		return len(snapshots.List())
	}, 200*time.Millisecond, 20*time.Millisecond).Should(gomega.Equal(0))

	clusterCol.UpdateObject(uccWithCluster{
		Client:         ucc,
		Name:           "cluster-b",
		Cluster:        &envoyclusterv3.Cluster{Name: "cluster-b"},
		ClusterVersion: 2,
	})

	g.Eventually(func() int {
		return len(snapshots.List())
	}, time.Second, 20*time.Millisecond).Should(gomega.Equal(1))

	snap := snapshots.List()[0].snap
	g.Expect(snap.Resources[envoycachetypes.Cluster].Items).To(gomega.HaveKey("cluster-a"))
	g.Expect(snap.Resources[envoycachetypes.Cluster].Items).To(gomega.HaveKey("cluster-b"))
}

func TestSnapshotPerClientStillPublishesWhenReferencedClusterErrored(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniqlyConnectedClient(role, "", nil, ir.PodLocality{})

	uccs := krt.NewStaticCollection[ir.UniqlyConnectedClient](nil, []ir.UniqlyConnectedClient{ucc})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes: sliceToResources([]*envoyroutev3.RouteConfiguration{
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
		}),
		Listeners: sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}}),
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

func mapKeys[M ~map[K]V, K comparable, V any](m M) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
