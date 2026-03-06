package proxy_syncer

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
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

func mapKeys[M ~map[K]V, K comparable, V any](m M) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
