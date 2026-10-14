package proxy_syncer

import (
	"errors"
	"maps"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func staticTestCluster(name string) *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{Name: name, ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}}
}

// requiredSet flattens a requiredEDSNames, failing if extra repeats base.
func requiredSet(g *gomega.WithT, r requiredEDSNames) map[string]struct{} {
	out := maps.Clone(r.base)
	if out == nil {
		out = map[string]struct{}{}
	}
	for name := range r.extra {
		g.Expect(r.base).ToNot(gomega.HaveKey(name), "extra must not repeat a base name")
		out[name] = struct{}{}
	}
	g.Expect(r.len()).To(gomega.Equal(len(out)))
	return out
}

// TestClientRequiredEDSNamesMatchesMergedPayload pins the row-derived set to
// what resolving the merged CDS payload from scratch yields, including a
// gateway cluster that shadows a row cluster and service_name aliases.
func TestClientRequiredEDSNamesMatchesMergedPayload(t *testing.T) {
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	row := assemblePerClientClusters(ucc, []uccWithCluster{
		publishedClusterRow(ucc, edsTestCluster("a", "")),
		publishedClusterRow(ucc, edsTestCluster("b", "shared")),
		publishedClusterRow(ucc, edsTestCluster("c", "shared")),
		publishedClusterRow(ucc, staticTestCluster("s")),
		{Client: ucc, Name: "broken", Error: errors.New("invalid"), PerClientError: true},
	})
	resource := func(c *envoyclusterv3.Cluster) envoycachetypes.ResourceWithTTL {
		return envoycachetypes.ResourceWithTTL{Resource: c}
	}

	cases := []struct {
		name      string
		extra     []envoycachetypes.ResourceWithTTL
		bootstrap string
	}{
		{name: "row only"},
		{name: "bootstrap name", bootstrap: "local"},
		{name: "bootstrap name already required", bootstrap: "a"},
		{name: "gateway clusters", extra: []envoycachetypes.ResourceWithTTL{
			resource(edsTestCluster("gw-eds", "")), resource(staticTestCluster("gw-static")),
		}},
		{name: "gateway cluster shadows an EDS row cluster", extra: []envoycachetypes.ResourceWithTTL{
			resource(staticTestCluster("a")),
		}},
		{name: "gateway cluster shadows one alias", extra: []envoycachetypes.ResourceWithTTL{
			resource(staticTestCluster("b")),
		}, bootstrap: "local"},
		{name: "gateway cluster renames a row cluster's assignment", extra: []envoycachetypes.ResourceWithTTL{
			resource(edsTestCluster("s", "now-eds")),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			before := maps.Clone(row.edsNames)

			merged := maps.Clone(row.clusters.Items)
			for _, item := range tc.extra {
				merged[envoycache.GetResourceName(item.Resource)] = item
			}
			want := requiredSet(g, requiredEDSNames{base: edsNamesOf(merged)}.with(tc.bootstrap))

			got := requiredSet(g, clientRequiredEDSNames(row, tc.extra, merged, tc.bootstrap))
			g.Expect(got).To(gomega.Equal(want))
			g.Expect(row.edsNames).To(gomega.Equal(before), "the row's set is shared and must not be written to")
		})
	}
}

// TestAssembledEDSNamesFollowThePublishedPayload covers two rows with one name:
// the payload keeps only the later one, and the row's EDS names must agree.
func TestAssembledEDSNamesFollowThePublishedPayload(t *testing.T) {
	g := gomega.NewWithT(t)
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	row := assemblePerClientClusters(ucc, []uccWithCluster{
		publishedClusterRow(ucc, edsTestCluster("dup", "dup-svc")),
		publishedClusterRow(ucc, staticTestCluster("dup")),
		publishedClusterRow(ucc, edsTestCluster("a", "")),
	})
	g.Expect(row.edsNames).To(gomega.Equal(edsNamesOf(row.clusters.Items)))
	g.Expect(row.edsNames).To(gomega.Equal(map[string]struct{}{"a": {}}))
}
