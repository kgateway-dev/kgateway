package proxy_syncer

import (
	"errors"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// endpointDigestOf folds content digests the way the endpoints row does.
func endpointDigestOf(contentHashes map[string]uint64) uint64 {
	var digest uint64
	for name, contentHash := range contentHashes {
		digest ^= edsEntryDigest(name, contentHash)
	}
	return digest
}

func edsTestCluster(name, serviceName string) *envoyclusterv3.Cluster {
	c := &envoyclusterv3.Cluster{Name: name, ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}}
	if serviceName != "" {
		c.EdsClusterConfig = &envoyclusterv3.Cluster_EdsClusterConfig{ServiceName: serviceName}
	}
	return c
}

// publishedClusterRow builds a per-client row the way the base and overlay
// paths do: ClusterVersion is the proto's content hash.
func publishedClusterRow(ucc ir.UniquelyConnectedClient, c *envoyclusterv3.Cluster) uccWithCluster {
	version := utils.HashProto(c)
	return uccWithCluster{Client: ucc, Name: c.GetName(), Cluster: sharedproto.WrapPrehashed(c, version), ClusterVersion: version}
}

// TestEndpointSetVersionFollowsContentNotInputs pins the property the EDS
// version is built for: rows whose inputs differ (different EndpointsHash, as
// after a backend policy generation bump) but whose assignments are
// byte-identical publish under the same version, so nothing is pushed; a
// content change moves it; and the fold does not depend on order.
func TestEndpointSetVersionFollowsContentNotInputs(t *testing.T) {
	g := gomega.NewWithT(t)
	a := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a", Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{}}}
	b := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "b"}
	changed := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a", Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{}, {}}}

	g.Expect(edsEntryDigest("a", contentHashOf(a))^edsEntryDigest("b", contentHashOf(b))).
		To(gomega.Equal(edsEntryDigest("b", contentHashOf(b))^edsEntryDigest("a", contentHashOf(a))), "the fold must not depend on order")
	g.Expect(endpointDigestOf(map[string]uint64{"a": contentHashOf(changed), "b": contentHashOf(b)})).
		ToNot(gomega.Equal(endpointDigestOf(map[string]uint64{"a": contentHashOf(a), "b": contentHashOf(b)})), "a content change must move the digest")
	g.Expect(endpointDigestOf(map[string]uint64{"x": 7, "y": 7})).ToNot(gomega.BeZero(),
		"equal digests under different names must not cancel to the empty set's digest")

	// Rows with different input hashes and equal content differ as rows, so
	// KRT recomputes, but digest identically, so the version does not move.
	row1 := UccWithEndpoints{Endpoints: sharedproto.Wrap(a), EndpointsHash: 1, ContentHash: contentHashOf(a), endpointsName: "a"}
	row2 := UccWithEndpoints{Endpoints: sharedproto.Wrap(a), EndpointsHash: 2, ContentHash: contentHashOf(a), endpointsName: "a"}
	g.Expect(row1.Equals(row2)).To(gomega.BeFalse(), "an input change is still a row change")
	g.Expect(edsEntryDigest("a", row1.ContentHash)).To(gomega.Equal(edsEntryDigest("a", row2.ContentHash)),
		"the published EDS version must not move when only inputs changed")

	// The reverse: equal inputs with different content is a row change.
	row3 := UccWithEndpoints{Endpoints: sharedproto.Wrap(changed), EndpointsHash: 1, ContentHash: contentHashOf(changed), endpointsName: "a"}
	g.Expect(row1.Equals(row3)).To(gomega.BeFalse(), "a content change behind an equal input hash must be a row change")
}

// TestEDSClustersDigestFollowsOnlyEDSClusters pins the cluster half of the
// EDS version: rebuilding an EDS cluster moves it even when its assignment is
// byte-identical, so the re-warming cluster is answered, while a STATIC or
// errored cluster, which has no assignment, does not.
func TestEDSClustersDigestFollowsOnlyEDSClusters(t *testing.T) {
	g := gomega.NewWithT(t)
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	static := func(policy envoyclusterv3.Cluster_LbPolicy) uccWithCluster {
		return publishedClusterRow(ucc, &envoyclusterv3.Cluster{
			Name:                 "static",
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC},
			LbPolicy:             policy,
		})
	}
	eds := publishedClusterRow(ucc, edsTestCluster("a", ""))
	before := assemblePerClientClusters(ucc, []uccWithCluster{eds, static(envoyclusterv3.Cluster_ROUND_ROBIN)})

	staticChanged := assemblePerClientClusters(ucc, []uccWithCluster{eds, static(envoyclusterv3.Cluster_RANDOM)})
	g.Expect(staticChanged.clustersHash).ToNot(gomega.Equal(before.clustersHash), "the CDS version tracks every cluster")
	g.Expect(staticChanged.edsClustersDigest).To(gomega.Equal(before.edsClustersDigest), "a STATIC cluster must not move the EDS version")

	rebuilt := edsTestCluster("a", "")
	rebuilt.LbPolicy = envoyclusterv3.Cluster_RANDOM
	edsChanged := assemblePerClientClusters(ucc, []uccWithCluster{publishedClusterRow(ucc, rebuilt), static(envoyclusterv3.Cluster_ROUND_ROBIN)})
	g.Expect(edsChanged.edsClustersDigest).ToNot(gomega.Equal(before.edsClustersDigest), "an EDS cluster rebuild must move the EDS version")
	g.Expect(edsChanged.Equals(*before)).To(gomega.BeFalse())

	errored := uccWithCluster{Client: ucc, Name: "broken", Error: errors.New("invalid"), PerClientError: true}
	withError := assemblePerClientClusters(ucc, []uccWithCluster{eds, static(envoyclusterv3.Cluster_ROUND_ROBIN), errored})
	g.Expect(withError.edsClustersDigest).To(gomega.Equal(before.edsClustersDigest), "an errored cluster is not published and must not move the EDS version")
}

// TestEDSClustersDigestFollowsServiceNameAliases covers clusters that share
// one assignment through eds_cluster_config.service_name: a rebuild of either
// must refresh the shared assignment.
func TestEDSClustersDigestFollowsServiceNameAliases(t *testing.T) {
	g := gomega.NewWithT(t)
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	rows := func(aPolicy, bPolicy envoyclusterv3.Cluster_LbPolicy) []uccWithCluster {
		a, b := edsTestCluster("a", "shared-service"), edsTestCluster("b", "shared-service")
		a.LbPolicy, b.LbPolicy = aPolicy, bPolicy
		return []uccWithCluster{publishedClusterRow(ucc, a), publishedClusterRow(ucc, b)}
	}
	before := assemblePerClientClusters(ucc, rows(envoyclusterv3.Cluster_ROUND_ROBIN, envoyclusterv3.Cluster_ROUND_ROBIN))
	for _, changed := range [][]uccWithCluster{
		rows(envoyclusterv3.Cluster_RANDOM, envoyclusterv3.Cluster_ROUND_ROBIN),
		rows(envoyclusterv3.Cluster_ROUND_ROBIN, envoyclusterv3.Cluster_RANDOM),
	} {
		g.Expect(assemblePerClientClusters(ucc, changed).edsClustersDigest).ToNot(gomega.Equal(before.edsClustersDigest),
			"either cluster's rebuild must refresh the shared assignment")
	}
}

// TestFilteredEndpointDigestMatchesDirectSet pins that filtering versions a
// subset by what it holds: an excluded assignment cannot affect the digest,
// known content digests are reused rather than re-marshaled, and an unfiltered
// set passes its precomputed digest through.
func TestFilteredEndpointDigestMatchesDirectSet(t *testing.T) {
	g := gomega.NewWithT(t)
	a := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a"}
	b := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "b"}
	contentHashes := map[string]uint64{"a": contentHashOf(a), "b": contentHashOf(b)}
	all := envoycache.NewResourcesWithTTL("input", []envoycachetypes.ResourceWithTTL{{Resource: a}, {Resource: b}})
	onlyA := envoycache.NewResourcesWithTTL("cds", []envoycachetypes.ResourceWithTTL{{Resource: edsTestCluster("a", "")}})

	_, _, digest := filterEndpointsForClusters(onlyA, all, contentHashes, endpointDigestOf(contentHashes))
	g.Expect(digest).To(gomega.Equal(endpointDigestOf(map[string]uint64{"a": contentHashOf(a)})), "an excluded assignment cannot affect the digest")
	_, _, marshaled := filterEndpointsForClusters(onlyA, all, nil, 0)
	g.Expect(marshaled).To(gomega.Equal(digest), "a digest from known content hashes must equal one from marshaling")

	_, _, reused := filterEndpointsForClusters(onlyA, all, map[string]uint64{"a": 42}, 0)
	g.Expect(reused).To(gomega.Equal(edsEntryDigest("a", 42)), "a known content digest must be reused, not re-marshaled")

	both := envoycache.NewResourcesWithTTL("cds", []envoycachetypes.ResourceWithTTL{{Resource: edsTestCluster("a", "")}, {Resource: edsTestCluster("b", "")}})
	out, synthesized, passed := filterEndpointsForClusters(both, all, contentHashes, 1234)
	g.Expect(synthesized).To(gomega.BeEmpty())
	g.Expect(out.Version).To(gomega.Equal("input"), "an unfiltered set must be returned as is")
	g.Expect(passed).To(gomega.Equal(uint64(1234)), "an unfiltered set must pass its precomputed digest through")

	withMissing := envoycache.NewResourcesWithTTL("cds", []envoycachetypes.ResourceWithTTL{{Resource: edsTestCluster("a", "")}, {Resource: edsTestCluster("c", "")}})
	_, synthesized, digest = filterEndpointsForClusters(withMissing, all, contentHashes, 0)
	g.Expect(synthesized).To(gomega.HaveKey("c"))
	g.Expect(digest).To(gomega.Equal(endpointDigestOf(map[string]uint64{
		"a": contentHashOf(a),
		"c": contentHashOf(&envoyendpointv3.ClusterLoadAssignment{ClusterName: "c"}),
	})), "a synthesized empty is digested from its proto")
}

// TestCarriedEndpointVersionMatchesDirectPublication pins that carrying a
// cluster and its assignment forward yields the version direct publication of
// the same pair would, so resolving the carry does not push again.
func TestCarriedEndpointVersionMatchesDirectPublication(t *testing.T) {
	g := gomega.NewWithT(t)
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	cluster := edsTestCluster("a", "")
	cla := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a", Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{}}}

	// Direct publication of cluster a with its assignment.
	direct := edsVersionInputs{
		endpoints: endpointDigestOf(map[string]uint64{"a": contentHashOf(cla)}),
		clusters:  assemblePerClientClusters(ucc, []uccWithCluster{publishedClusterRow(ucc, cluster)}).edsClustersDigest,
	}
	prior := &envoycache.Snapshot{}
	prior.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds", []envoycachetypes.ResourceWithTTL{{Resource: cluster}})
	prior.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL(direct.version(), []envoycachetypes.ResourceWithTTL{{Resource: cla}})

	// The next build lacks cluster a entirely, so it is carried.
	next := &envoycache.Snapshot{}
	next.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL(edsVersionInputs{}.version(), nil)
	carried, _ := resolveDeferredPerCluster(XdsSnapWrapper{snap: next, missingReferenced: []string{"a"}}, prior, false)
	g.Expect(carried.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("a"))
	g.Expect(carried.Resources[envoycachetypes.Endpoint].Version).To(gomega.Equal(direct.version()),
		"the same published assignment and cluster must have the same version whether derived or carried")
}

// TestEDSVersionInputsDoNotCancel pins that the three digests are combined
// positionally, so equal endpoint and cluster digests cannot cancel.
func TestEDSVersionInputsDoNotCancel(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(edsVersionInputs{endpoints: 7, clusters: 7}.version()).ToNot(gomega.Equal(edsVersionInputs{}.version()))
	g.Expect(edsVersionInputs{endpoints: 7}.version()).ToNot(gomega.Equal(edsVersionInputs{clusters: 7}.version()))
	g.Expect(edsVersionInputs{extraClusters: 7}.version()).ToNot(gomega.Equal(edsVersionInputs{}.version()))
}
