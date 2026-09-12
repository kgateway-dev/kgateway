package proxy_syncer

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/onsi/gomega"
)

func TestClusterVersionEqualityTracksIndividualVersions(t *testing.T) {
	g := gomega.NewWithT(t)
	a := clustersWithErrors{clustersHash: 3, clusterVersions: map[string]uint64{"a": 1, "b": 2}}
	b := clustersWithErrors{clustersHash: 3, clusterVersions: map[string]uint64{"a": 2, "b": 1}}
	g.Expect(a.Equals(b)).To(gomega.BeFalse(), "swapped digests must propagate even if the CDS XOR is unchanged")
	b.clusterVersions = map[string]uint64{"b": 2, "a": 1}
	g.Expect(a.Equals(b)).To(gomega.BeTrue(), "map insertion order must not affect equality")
}

func TestEndpointClusterDigestsFollowServiceName(t *testing.T) {
	g := gomega.NewWithT(t)
	cluster := func(name string) envoycachetypes.ResourceWithTTL {
		return envoycachetypes.ResourceWithTTL{Resource: &envoyclusterv3.Cluster{
			Name:                 name,
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
			EdsClusterConfig:     &envoyclusterv3.Cluster_EdsClusterConfig{ServiceName: "shared-service"},
		}}
	}
	clusters := envoycache.NewResourcesWithTTL("cds", []envoycachetypes.ResourceWithTTL{cluster("a"), cluster("b")})
	before := endpointClusterDigests(clusters, map[string]uint64{"a": 1, "b": 2})
	g.Expect(before).To(gomega.HaveLen(1), "both clusters request the same EDS resource")
	g.Expect(before).To(gomega.HaveKey("shared-service"))
	for _, changed := range []map[string]uint64{{"a": 3, "b": 2}, {"a": 1, "b": 3}} {
		g.Expect(endpointClusterDigests(clusters, changed)).ToNot(gomega.Equal(before), "either cluster's rebuild must refresh the shared assignment")
	}
}

func TestCarriedEndpointVersionMatchesDirectPublication(t *testing.T) {
	g := gomega.NewWithT(t)
	cluster := &envoyclusterv3.Cluster{Name: "a", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}}
	cla := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a"}
	prior := &envoycache.Snapshot{}
	prior.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds", []envoycachetypes.ResourceWithTTL{{Resource: cluster}})
	prior.Resources[envoycachetypes.Endpoint] = versionEndpointResources(
		envoycache.NewResourcesWithTTL("input", []envoycachetypes.ResourceWithTTL{{Resource: cla}}), nil,
		endpointClusterDigests(prior.Resources[envoycachetypes.Cluster], nil))
	for _, upstreamVersion := range []string{"input-1", "input-2"} {
		next := &envoycache.Snapshot{}
		next.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL(upstreamVersion, nil)
		carried, _ := resolveDeferredPerCluster(XdsSnapWrapper{snap: next, missingReferenced: []string{"a"}}, prior, false)
		g.Expect(carried.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("a"))
		g.Expect(carried.Resources[envoycachetypes.Endpoint].Version).To(gomega.Equal(prior.Resources[envoycachetypes.Endpoint].Version),
			"the same published assignment and cluster must have the same version whether derived or carried")
	}
}

func TestFilteredEndpointVersionMatchesDirectSet(t *testing.T) {
	g := gomega.NewWithT(t)
	a := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a"}
	b := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "b"}
	all := envoycache.NewResourcesWithTTL("input", []envoycachetypes.ResourceWithTTL{{Resource: a}, {Resource: b}})
	clusters := envoycache.NewResourcesWithTTL("cds", []envoycachetypes.ResourceWithTTL{{Resource: &envoyclusterv3.Cluster{
		Name: "a", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
	}}})
	digests := endpointClusterDigests(clusters, nil)
	filtered, _ := filterEndpointResourcesForClusters(clusters, all)
	filtered = versionEndpointResources(filtered, map[string]uint64{"a": contentHashOf(a), "b": contentHashOf(b)}, digests)
	direct := versionEndpointResources(envoycache.NewResourcesWithTTL("another-input", []envoycachetypes.ResourceWithTTL{{Resource: a}}), nil, digests)
	g.Expect(filtered.Version).To(gomega.Equal(direct.Version), "an excluded assignment cannot affect the final version")
}
