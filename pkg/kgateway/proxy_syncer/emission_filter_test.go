package proxy_syncer

import (
	"strconv"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

func clusterResourcesFor(names ...string) (envoycache.Resources, map[string]uint64) {
	items := make([]envoycachetypes.ResourceWithTTL, 0, len(names))
	versions := make(map[string]uint64, len(names))
	var hash, version uint64
	for _, name := range names {
		version++
		items = append(items, envoycachetypes.ResourceWithTTL{Resource: &envoyclusterv3.Cluster{Name: name}})
		versions[name] = version
		hash ^= version
	}
	return envoycache.NewResourcesWithTTL(strconv.FormatUint(hash, 10), items), versions
}

func emissionSet(names ...string) emittedClusters {
	set := emittedClusters{Names: make(map[string]struct{}, len(names))}
	for _, name := range names {
		set.Names[name] = struct{}{}
	}
	return set
}

// TestFilterClustersToEmittedIsInertInAllMode: ALL is the default and must be
// byte-identical to not having the feature, or every existing deployment changes
// shape on upgrade.
func TestFilterClustersToEmittedIsInertInAllMode(t *testing.T) {
	clusters, versions := clusterResourcesFor("routed", "unreferenced")

	got, gotVersions, filtered := filterClustersToEmitted(
		clusterScoping{}, emissionSet("routed"), clusters, versions)

	assert.False(t, filtered)
	assert.Equal(t, clusters.Items, got.Items, "ALL must not drop anything")
	assert.Equal(t, versions, gotVersions)
}

// TestFilterClustersToEmittedDropsUnreferencedBackends is the feature: the
// clusters no route, filter or ancillary reference names leave CDS, and with
// them their stats and their share of every proxy's memory.
func TestFilterClustersToEmittedDropsUnreferencedBackends(t *testing.T) {
	clusters, versions := clusterResourcesFor("routed", "ancillary", "unreferenced-a", "unreferenced-b")

	got, gotVersions, filtered := filterClustersToEmitted(
		scopedClusters(), emissionSet("routed", "ancillary"), clusters, versions)

	require.True(t, filtered)
	assert.ElementsMatch(t, []string{"routed", "ancillary"}, resourceNames(got))
	assert.Equal(t, map[string]uint64{"routed": versions["routed"], "ancillary": versions["ancillary"]}, gotVersions)
}

// TestFilterClustersToEmittedVersionsOverWhatSurvives: a dropped cluster that
// leaves the CDS version unchanged is a cluster Envoy goes on serving, which
// would make the whole filter invisible to the data plane.
func TestFilterClustersToEmittedVersionsOverWhatSurvives(t *testing.T) {
	clusters, versions := clusterResourcesFor("routed", "unreferenced")
	before := clusters.Version

	_, gotVersions, filtered := filterClustersToEmitted(
		scopedClusters(), emissionSet("routed"), clusters, versions)

	require.True(t, filtered)
	after := strconv.FormatUint(emittedClustersHash(gotVersions), 10)
	assert.NotEqual(t, before, after, "dropping a cluster must move the CDS version")
	assert.Equal(t, strconv.FormatUint(versions["routed"], 10), after,
		"the version must be the fold of exactly the retained clusters")
}

// TestFilterClustersToEmittedRevertsForRequestTimeSelectors is the guard doing
// its job. The candidates such a route may select are named nowhere, so pruning
// would not 503 — it would silently route to the plugin's fallback. Reverting to
// emit-all for that gateway is the only safe answer.
func TestFilterClustersToEmittedRevertsForRequestTimeSelectors(t *testing.T) {
	clusters, versions := clusterResourcesFor("routed", "candidate-a", "candidate-b")
	unfilterable := emissionSet("routed")
	unfilterable.RequestTimeSelectors = []requestTimeSelector{{selectorClusterHeader, "x-target"}}

	got, gotVersions, filtered := filterClustersToEmitted(
		scopedClusters(), unfilterable, clusters, versions)

	assert.False(t, filtered, "an unfilterable gateway must keep every cluster")
	assert.Equal(t, clusters.Items, got.Items)
	assert.Equal(t, versions, gotVersions)
}

// TestFilterClustersToEmittedReportsNoChangeWhenEverythingIsReferenced keeps the
// common fully-routed case off the re-versioning path, so a deployment where
// every backend is referenced produces the same version it did before.
func TestFilterClustersToEmittedReportsNoChangeWhenEverythingIsReferenced(t *testing.T) {
	clusters, versions := clusterResourcesFor("a", "b")

	got, gotVersions, filtered := filterClustersToEmitted(
		scopedClusters(), emissionSet("a", "b"), clusters, versions)

	assert.False(t, filtered)
	assert.Equal(t, clusters.Items, got.Items)
	assert.Equal(t, versions, gotVersions)
}

// TestFilterClustersToEmittedKeepsBlackhole: routes whose backends fail
// resolution target the blackhole cluster. The collector always puts it in the
// set; this pins that the filter therefore keeps it.
func TestFilterClustersToEmittedKeepsBlackhole(t *testing.T) {
	blackhole := wellknown.BlackholeClusterName
	clusters, versions := clusterResourcesFor(blackhole, "unreferenced")
	emission := collectReferencedClustersForEmission(envoycache.Resources{}, envoycache.Resources{})
	require.Contains(t, emission.Names, blackhole, "collector must supply the blackhole name this test filters on")

	got, _, filtered := filterClustersToEmitted(
		scopedClusters(), emission, clusters, versions)

	require.True(t, filtered)
	assert.Equal(t, []string{blackhole}, resourceNames(got))
}

// TestSnapshotPerClientEmitsOnlyReferencedClusters is the feature end to end:
// the same inputs, published twice, differ by the setting alone. The routed
// backend and the blackhole survive; the two backends no route names do not,
// and their ClusterLoadAssignments go with them because EDS is aligned to
// whatever CDS ends up containing.
func TestSnapshotPerClientEmitsOnlyReferencedClusters(t *testing.T) {
	for _, tc := range []struct {
		name     string
		scoping  clusterScoping
		expected []string
	}{
		{
			name:     "all emits every translated backend",
			scoping:  clusterScoping{},
			expected: []string{"routed", "unreferenced-a", "unreferenced-b"},
		},
		{
			name:     "referenced emits only what the config names",
			scoping:  scopedClusters(),
			expected: []string{"routed"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
			ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
			uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

			routes := sliceToResources([]*envoyroutev3.RouteConfiguration{{
				Name: "route-config",
				VirtualHosts: []*envoyroutev3.VirtualHost{{
					Name:    "vhost",
					Domains: []string{"*"},
					Routes: []*envoyroutev3.Route{{
						Name: "routed",
						Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
							ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "routed"},
						}},
					}},
				}},
			}})
			listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
			mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
				NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
				Routes:             routes,
				Listeners:          listeners,
				ReferencedClusters: collectReferencedClusters(routes, listeners),
				EmittedClusters:    collectReferencedClustersForEmission(routes, listeners),
			}})

			clusterRows := make([]uccWithCluster, 0, 3)
			endpointRows := make([]UccWithEndpoints, 0, 3)
			var version uint64
			for _, name := range []string{"routed", "unreferenced-a", "unreferenced-b"} {
				version++
				clusterRows = append(clusterRows, uccWithCluster{
					Client:         ucc,
					Name:           name,
					Cluster:        sharedproto.Wrap(edsClusterProto(name)),
					ClusterVersion: version,
				})
				endpointRows = append(endpointRows, UccWithEndpoints{
					Client:        ucc,
					endpointsName: name,
					resourceName:  uccEndpointsResourceName(ucc, name),
					Endpoints:     sharedproto.Wrap(&envoyendpointv3.ClusterLoadAssignment{ClusterName: name}),
					EndpointsHash: version,
				})
			}
			clusterCol := krt.NewStaticCollection[uccWithCluster](nil, clusterRows)
			endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, endpointRows)

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
				tc.scoping,
			)

			wrap := eventuallyCoherentWrapper(t, snapshots)
			g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Cluster].Items)).To(gomega.ConsistOf(toAny(tc.expected)...))
			g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Endpoint].Items)).To(gomega.ConsistOf(toAny(tc.expected)...),
				"EDS must follow CDS: a dropped cluster's CLA must go with it")
		})
	}
}

func toAny(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

func edsClusterProto(name string) *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
		EdsClusterConfig:     &envoyclusterv3.Cluster_EdsClusterConfig{ServiceName: name},
	}
}
