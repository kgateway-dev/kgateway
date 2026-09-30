package proxy_syncer

import (
	"context"
	"errors"
	"testing"
	"time"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoygrpcaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	envoyextauthzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoyjwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	envoyhttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoydiscoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	envoystreamv3 "github.com/envoyproxy/go-control-plane/pkg/server/stream/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	kgtranslator "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

func TestPerClientEnvoyEndpointsUsesResolvedReplacementHash(t *testing.T) {
	g := gomega.NewWithT(t)
	krtopts := krtutil.NewKrtOptions(t.Context().Done(), nil)
	pluginGK := schema.GroupKind{Group: "test.example.io", Kind: "EndpointReplacement"}

	translator := kgtranslator.NewCombinedTranslator(t.Context(), sdk.Plugin{
		ContributesPolicies: sdk.ContributesPolicies{
			pluginGK: {
				PerClientEditEndpoints: func(_ krt.HandlerContext, _ context.Context, _ ir.UniquelyConnectedClient, out endpoints.EndpointInputsEditor) uint64 {
					replacement := out.NewEndpointSet()
					replacement.Add(ir.PodLocality{}, ir.EndpointWithMd{LbEndpoint: &envoyendpointv3.LbEndpoint{
						HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{Endpoint: &envoyendpointv3.Endpoint{
							Address: &envoycorev3.Address{Address: &envoycorev3.Address_Pipe{Pipe: &envoycorev3.Pipe{Path: out.Hostname()}}},
						}},
					}})
					out.ReplaceEndpoints(replacement)
					return 0 // Replacement content is already reflected by LbEpsEqualityHash.
				},
			},
		},
	}, nil, nil)

	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "backend"}, 80, "", "")
	source := ir.NewEndpointsForBackend(backend)
	source.Hostname = "replacement-a"
	source.Add(ir.PodLocality{}, ir.EndpointWithMd{LbEndpoint: &envoyendpointv3.LbEndpoint{
		HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{Endpoint: &envoyendpointv3.Endpoint{
			Address: &envoycorev3.Address{Address: &envoycorev3.Address_Pipe{Pipe: &envoycorev3.Pipe{Path: "source"}}},
		}},
	}})
	sourceHash := source.LbEpsEqualityHash
	ucc := ir.NewUniquelyConnectedClient("client", "ns", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc}, krtopts.ToOptions("ReplacementHashClients")...)
	sources := krt.NewStaticCollection(nil, []ir.EndpointsForBackend{*source}, krtopts.ToOptions("ReplacementHashEndpoints")...)
	perClient := NewPerClientEnvoyEndpoints(
		krtopts,
		uccs,
		sources,
		translator.ResolveEndpoints,
		translator.BuildClusterLoadAssignment,
	)

	var initialHash uint64
	g.Eventually(func() string {
		rows := perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, ucc)
		if len(rows) != 1 {
			return ""
		}
		initialHash = rows[0].EndpointsHash
		return endpointPipePath(rows[0].Endpoints.Clone())
	}, time.Second, 20*time.Millisecond).Should(gomega.Equal("replacement-a"))
	g.Expect(initialHash).ToNot(gomega.Equal(sourceHash),
		"the row key must use the replacement set's resolved hash, not the source hash")

	updated := *source
	updated.Hostname = "replacement-b"
	g.Expect(updated.LbEpsEqualityHash).To(gomega.Equal(sourceHash),
		"precondition: only the plugin's replacement output changes the endpoint hash")
	sources.UpdateObject(updated)

	var updatedHash uint64
	g.Eventually(func() string {
		rows := perClient.FetchEndpointsForClient(krt.TestingDummyContext{}, ucc)
		if len(rows) != 1 {
			return ""
		}
		updatedHash = rows[0].EndpointsHash
		return endpointPipePath(rows[0].Endpoints.Clone())
	}, time.Second, 20*time.Millisecond).Should(gomega.Equal("replacement-b"),
		"a replacement-only update must not be suppressed by stale KRT equality")
	g.Expect(updatedHash).ToNot(gomega.Equal(initialHash))
}

func endpointPipePath(cla *envoyendpointv3.ClusterLoadAssignment) string {
	if len(cla.GetEndpoints()) == 0 || len(cla.GetEndpoints()[0].GetLbEndpoints()) == 0 {
		return ""
	}
	return cla.GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetPipe().GetPath()
}

func TestFilterEndpointResourcesForClusters_FiltersStaticClusterCLAs(t *testing.T) {
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

	out, _ := filterEndpointResourcesForClusters(clusters, endpoints)

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 endpoint resource, got %d", len(out.Items))
	}
	if _, ok := out.Items["eds-cluster"]; !ok {
		t.Errorf("expected CLA for eds-cluster to remain, got keys: %v", mapKeys(out.Items))
	}
	if _, ok := out.Items["static-cluster"]; ok {
		t.Error("expected CLA for static-cluster to be filtered out")
	}
	if out.Version == endpoints.Version {
		t.Errorf("version should change when endpoint resources are filtered: got %q", out.Version)
	}
}

func TestFilterEndpointResourcesForClusters_ReturnsOriginalWhenNoFiltering(t *testing.T) {
	// Only EDS clusters; no STATIC. Endpoints should be returned unchanged (same reference when possible).
	clusters := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{Name: "eds-only", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}}},
	})
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "eds-only"}},
	})

	out, _ := filterEndpointResourcesForClusters(clusters, endpoints)

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

func TestFilterEndpointResourcesForClusters_EmptyClustersAndEndpoints(t *testing.T) {
	emptyClusters := envoycache.NewResourcesWithTTL("v1", nil)
	emptyEndpoints := envoycache.NewResourcesWithTTL("v1", nil)

	out, _ := filterEndpointResourcesForClusters(emptyClusters, emptyEndpoints)

	if len(out.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(out.Items))
	}
}

func TestFilterEndpointResourcesForClusters_EmptyClustersNonEmptyEndpoints(t *testing.T) {
	emptyClusters := envoycache.NewResourcesWithTTL("v1", nil)
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "any"}},
	})

	out, _ := filterEndpointResourcesForClusters(emptyClusters, endpoints)

	if len(out.Items) != 0 {
		t.Fatalf("expected no endpoint resources when no EDS clusters are emitted, got %d", len(out.Items))
	}
}

func TestFilterEndpointResourcesForClusters_EmptyEndpoints(t *testing.T) {
	clusters := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{Name: "static-cluster", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STATIC}}},
	})
	emptyEndpoints := envoycache.NewResourcesWithTTL("v1", nil)

	out, _ := filterEndpointResourcesForClusters(clusters, emptyEndpoints)

	if len(out.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(out.Items))
	}
}

func TestFilterEndpointResourcesForClusters_MixedStaticAndNonStatic(t *testing.T) {
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

	out, _ := filterEndpointResourcesForClusters(clusters, endpoints)

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

func TestFilterEndpointResourcesForClusters_FiltersStaleClusterLoadAssignments(t *testing.T) {
	clusters := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{Name: "cluster-a", ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}}},
	})
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "cluster-a"}},
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "cluster-b"}},
	})

	out, _ := filterEndpointResourcesForClusters(clusters, endpoints)

	if len(out.Items) != 1 {
		t.Fatalf("expected only the CLA required by CDS, got %d: %v", len(out.Items), mapKeys(out.Items))
	}
	if _, ok := out.Items["cluster-a"]; !ok {
		t.Errorf("expected CLA for cluster-a, got keys: %v", mapKeys(out.Items))
	}
	if _, ok := out.Items["cluster-b"]; ok {
		t.Error("expected stale CLA for cluster-b to be filtered out")
	}
	if out.Version == endpoints.Version {
		t.Errorf("version should change when stale endpoint resources are filtered: got %q", out.Version)
	}
}

func TestFilterEndpointResourcesForClusters_UsesEDSServiceName(t *testing.T) {
	clusters := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyclusterv3.Cluster{
			Name: "cluster-a",
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
				Type: envoyclusterv3.Cluster_EDS,
			},
			EdsClusterConfig: &envoyclusterv3.Cluster_EdsClusterConfig{
				ServiceName: "service-a",
			},
		}},
	})
	endpoints := envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "cluster-a"}},
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "service-a"}},
	})

	out, _ := filterEndpointResourcesForClusters(clusters, endpoints)

	if len(out.Items) != 1 {
		t.Fatalf("expected only the service-name CLA required by CDS, got %d: %v", len(out.Items), mapKeys(out.Items))
	}
	if _, ok := out.Items["service-a"]; !ok {
		t.Errorf("expected CLA for service-a, got keys: %v", mapKeys(out.Items))
	}
	if _, ok := out.Items["cluster-a"]; ok {
		t.Error("expected cluster-name CLA to be filtered when EDS service_name is set")
	}
}

func TestSnapshotPerClientDefersUntilAllReferencedClustersAreReady(t *testing.T) {
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
	})
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		{
			Client:         ucc,
			Name:           "cluster-a",
			Cluster:        sharedproto.Wrap(&envoyclusterv3.Cluster{Name: "cluster-a"}),
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
		newTestPerClientClustersFromCol(clusterCol, uccs),
	)

	// The wrapper is built immediately but marked deferred: cluster-b is
	// referenced and missing, so syncXds holds the route flip (or withholds
	// entirely for a never-published client).
	wrap := eventuallyDeferredWrapper(t, snapshots)
	g.Expect(wrap.missingReferenced).To(gomega.ConsistOf("cluster-b"),
		"the missing referenced cluster must be recorded for syncXds to resolve")
	g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Cluster].Items)).To(gomega.ConsistOf("cluster-a"))

	clusterCol.UpdateObject(uccWithCluster{
		Client:         ucc,
		Name:           "cluster-b",
		Cluster:        sharedproto.Wrap(&envoyclusterv3.Cluster{Name: "cluster-b"}),
		ClusterVersion: 2,
	})

	wrap = eventuallyCoherentWrapper(t, snapshots)
	g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Cluster].Items)).To(gomega.ConsistOf("cluster-a", "cluster-b"),
		"published CDS names must exactly match the current coherent per-client cluster inputs")
}

func TestSnapshotPerClientDefersUntilReferencedEDSClustersHaveEndpoints(t *testing.T) {
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
							Name: "eds-route",
							Action: &envoyroutev3.Route_Route{
								Route: &envoyroutev3.RouteAction{
									ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "cluster-a"},
								},
							},
						},
					},
				},
			},
		},
	})
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		{
			Client: ucc,
			Name:   "cluster-a",
			Cluster: sharedproto.Wrap(&envoyclusterv3.Cluster{
				Name: "cluster-a",
				ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
					Type: envoyclusterv3.Cluster_EDS,
				},
			}),
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
		newTestPerClientClustersFromCol(clusterCol, uccs),
	)

	// Deferred: the referenced EDS cluster has no usable endpoint yet. Its
	// CLA is a synthesized empty so the snapshot stays EDS-consistent.
	wrap := eventuallyDeferredWrapper(t, snapshots)
	g.Expect(wrap.missingEndpointsReferenced).To(gomega.ConsistOf("cluster-a"))
	g.Expect(wrap.snap.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-a"),
		"the unready EDS cluster gets a synthesized empty CLA")

	endpointCol.UpdateObject(endpointsForClient(ucc, "cluster-a", 3))

	wrap = eventuallyCoherentWrapper(t, snapshots)
	g.Expect(wrap.snap.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-a"))
}

func TestSnapshotPerClientFiltersStaleEndpointResourcesWhenClusterRemoved(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})

	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})
	listeners := sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}})
	routesForClusters := func(clusterNames ...string) envoycache.Resources {
		routes := make([]*envoyroutev3.Route, 0, len(clusterNames))
		for _, clusterName := range clusterNames {
			routes = append(routes, &envoyroutev3.Route{
				Name: "route-" + clusterName,
				Action: &envoyroutev3.Route_Route{
					Route: &envoyroutev3.RouteAction{
						ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: clusterName},
					},
				},
			})
		}

		return sliceToResources([]*envoyroutev3.RouteConfiguration{
			{
				Name: "route-config",
				VirtualHosts: []*envoyroutev3.VirtualHost{
					{
						Name:    "vhost",
						Domains: []string{"*"},
						Routes:  routes,
					},
				},
			},
		})
	}

	initialRoutes := routesForClusters("cluster-a", "cluster-b")
	initial := GatewayXdsResources{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             initialRoutes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(initialRoutes, listeners),
	}
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{initial})

	clusterA := uccWithCluster{
		Client: ucc,
		Name:   "cluster-a",
		Cluster: sharedproto.Wrap(&envoyclusterv3.Cluster{
			Name: "cluster-a",
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
				Type: envoyclusterv3.Cluster_EDS,
			},
		}),
		ClusterVersion: 1,
	}
	clusterB := uccWithCluster{
		Client: ucc,
		Name:   "cluster-b",
		Cluster: sharedproto.Wrap(&envoyclusterv3.Cluster{
			Name: "cluster-b",
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
				Type: envoyclusterv3.Cluster_EDS,
			},
		}),
		ClusterVersion: 2,
	}
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{clusterA, clusterB})
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{
		endpointsForClient(ucc, "cluster-a", 3),
		endpointsForClient(ucc, "cluster-b", 4),
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

	g.Eventually(func() int {
		return len(snapshots.List())
	}, time.Second, 20*time.Millisecond).Should(gomega.Equal(1))
	initialSnap := snapshots.List()[0].snap
	initialEndpointVersion := initialSnap.Resources[envoycachetypes.Endpoint].Version
	g.Expect(initialSnap.Resources[envoycachetypes.Cluster].Items).To(gomega.HaveKey("cluster-b"))
	g.Expect(initialSnap.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-b"))

	updatedRoutes := routesForClusters("cluster-a")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.DeleteObject(clusterB.ResourceName())

	g.Eventually(func() bool {
		list := snapshots.List()
		if len(list) != 1 {
			return false
		}
		snap := list[0].snap
		clusters := snap.Resources[envoycachetypes.Cluster].Items
		endpoints := snap.Resources[envoycachetypes.Endpoint].Items
		_, hasClusterA := clusters["cluster-a"]
		_, hasClusterB := clusters["cluster-b"]
		_, hasEndpointA := endpoints["cluster-a"]
		_, hasEndpointB := endpoints["cluster-b"]
		return hasClusterA && !hasClusterB && hasEndpointA && !hasEndpointB
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"stale cluster-b CLA must not be published after cluster-b is removed from CDS")

	g.Expect(snapshots.List()[0].snap.Resources[envoycachetypes.Endpoint].Version).ToNot(gomega.Equal(initialEndpointVersion),
		"EDS version must change when filtering removes a stale endpoint resource")
}

func TestSnapshotPerClientFilteredEdsSnapshotRespondsToNamedADSRequestAfterClusterRemoved(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})

	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	initialRoutes := routeResourcesForClusters("cluster-a", "cluster-b")
	initial := GatewayXdsResources{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             initialRoutes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(initialRoutes, listeners),
	}
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{initial})

	clusterA := edsClusterForClient(ucc, "cluster-a", 1)
	clusterB := edsClusterForClient(ucc, "cluster-b", 2)
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{clusterA, clusterB})
	endpointA := endpointsForClient(ucc, "cluster-a", 3)
	endpointB := endpointsForClient(ucc, "cluster-b", 4)
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{endpointA, endpointB})

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

	initialSnap := eventuallySingleSnapshot(t, snapshots)
	initialEndpointVersion := initialSnap.Resources[envoycachetypes.Endpoint].Version
	g.Expect(initialSnap.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-b"))

	updatedRoutes := routeResourcesForClusters("cluster-a")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.DeleteObject(clusterB.ResourceName())

	var updatedSnap *envoycache.Snapshot
	g.Eventually(func() bool {
		updatedSnap = eventuallyCurrentSnapshot(snapshots)
		if updatedSnap == nil {
			return false
		}
		endpoints := updatedSnap.Resources[envoycachetypes.Endpoint].Items
		_, hasEndpointA := endpoints["cluster-a"]
		_, hasEndpointB := endpoints["cluster-b"]
		return hasEndpointA && !hasEndpointB
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue())

	updatedEndpointVersion := updatedSnap.Resources[envoycachetypes.Endpoint].Version
	g.Expect(updatedEndpointVersion).ToNot(gomega.Equal(initialEndpointVersion))

	cache := newTestSnapshotCache(t)
	nodeID := ucc.ResourceName()
	g.Expect(cache.SetSnapshot(context.Background(), nodeID, updatedSnap)).ToNot(gomega.HaveOccurred())

	req := &envoydiscoveryv3.DiscoveryRequest{
		Node:          &envoycorev3.Node{Id: nodeID},
		TypeUrl:       envoyresourcev3.EndpointType,
		ResourceNames: []string{"cluster-a"},
		VersionInfo:   initialEndpointVersion,
	}
	sub := envoystreamv3.NewSotwSubscription(req.GetResourceNames(), true)
	sub.SetReturnedResources(map[string]string{
		"cluster-a": initialEndpointVersion,
		"cluster-b": initialEndpointVersion,
	})
	responses := make(chan envoycache.Response, 1)
	_, err := cache.CreateWatch(req, sub, responses)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	select {
	case response := <-responses:
		g.Expect(response.GetResponseVersion()).To(gomega.Equal(updatedEndpointVersion))
		g.Expect(response.GetReturnedResources()).To(gomega.HaveKeyWithValue("cluster-a", updatedEndpointVersion))
		g.Expect(response.GetReturnedResources()).ToNot(gomega.HaveKey("cluster-b"))
		discoveryResponse, err := response.GetDiscoveryResponse()
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(discoveryResponse.GetResources()).To(gomega.HaveLen(1))
	case <-time.After(time.Second):
		t.Fatal("expected filtered EDS snapshot to answer the named ADS EDS request")
	}
}

func TestSnapshotPerClientDefersMakeBeforeBreakRouteUntilNewEndpointReady(t *testing.T) {
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
	clusterNew := edsClusterForClient(ucc, "cluster-new", 2)
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{clusterOld})
	endpointOld := endpointsForClient(ucc, "cluster-old", 3)
	endpointNew := endpointsForClient(ucc, "cluster-new", 4)
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{endpointOld})

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
	g.Expect(initialServed.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-old"))
	assertSnapshotCoherent(t, initialServed)

	updatedRoutes := routeResourcesForClusters("cluster-new")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.UpdateObject(clusterNew)

	// The flip onto cluster-new is held while its EDS is absent: the served
	// routes keep targeting cluster-old, but cluster-new already enters the
	// served CDS (with a synthesized empty CLA) so it can warm — the
	// reference-ahead shape.
	var heldServed *envoycache.Snapshot
	g.Eventually(func() bool {
		heldServed = eventuallyCacheSnapshot(t, cache, nodeID)
		return snapshotReferencesCluster(heldServed, "cluster-old") &&
			!snapshotReferencesCluster(heldServed, "cluster-new") &&
			hasResource(heldServed.Resources[envoycachetypes.Cluster].Items, "cluster-new")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"the route flip must be held while cluster-new warms in the served CDS")
	assertSnapshotCoherent(t, heldServed)

	endpointCol.UpdateObject(endpointNew)

	var warmedServed *envoycache.Snapshot
	g.Eventually(func() bool {
		warmedServed = eventuallyCacheSnapshot(t, cache, nodeID)
		return snapshotReferencesCluster(warmedServed, "cluster-new") &&
			hasResource(warmedServed.Resources[envoycachetypes.Cluster].Items, "cluster-new") &&
			hasResource(warmedServed.Resources[envoycachetypes.Endpoint].Items, "cluster-new")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"once CDS and EDS for cluster-new are ready, the held route flip publishes")
	assertSnapshotCoherent(t, warmedServed)

	clusterCol.DeleteObject(clusterOld.ResourceName())
	endpointCol.DeleteObject(endpointOld.ResourceName())

	var finalServed *envoycache.Snapshot
	g.Eventually(func() bool {
		finalServed = eventuallyCacheSnapshot(t, cache, nodeID)
		clusters := finalServed.Resources[envoycachetypes.Cluster].Items
		endpoints := finalServed.Resources[envoycachetypes.Endpoint].Items
		return snapshotReferencesCluster(finalServed, "cluster-new") &&
			hasResource(clusters, "cluster-new") &&
			!hasResource(clusters, "cluster-old") &&
			hasResource(endpoints, "cluster-new") &&
			!hasResource(endpoints, "cluster-old")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"old cluster resources should be removable after the active route has moved to the warmed cluster")
	assertSnapshotCoherent(t, finalServed)
}

// TestSnapshotPerClientRetargetToDerivedBackend pins the two phases of a
// route retarget onto a brand-new backend under presence semantics: while
// the new cluster's CLA has not been derived (the per-client endpoints
// collection has no row for it), the flip is held at the published routes;
// once a CLA row is derived — even an EMPTY one — it is the backend's truth
// (presence, not contents, gates deferral: "empty forever, on purpose" is a
// production-proven config shape, #14352) and the flip publishes, with the
// route failing until endpoints arrive, exactly as the whole-snapshot
// existence gate this replaced behaved.
func TestSnapshotPerClientRetargetToDerivedBackend(t *testing.T) {
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
	clusterNew := edsClusterForClient(ucc, "cluster-new", 2)
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{clusterOld})
	endpointOld := endpointsForClient(ucc, "cluster-old", 3)
	endpointNewEmpty := emptyEndpointsForClient(ucc, "cluster-new", 4)
	endpointNewReady := endpointsForClient(ucc, "cluster-new", 5)
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{endpointOld})

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
	g.Expect(initialServed.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-old"))
	assertSnapshotCoherent(t, initialServed)

	updatedRoutes := routeResourcesForClusters("cluster-new")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.UpdateObject(clusterNew)

	// Phase 1: no CLA row has been derived for cluster-new (only a
	// synthesized empty stands in), so the flip is held: served routes keep
	// targeting cluster-old while cluster-new warms in the served CDS.
	var heldServed *envoycache.Snapshot
	g.Eventually(func() bool {
		heldServed = eventuallyCacheSnapshot(t, cache, nodeID)
		return snapshotReferencesCluster(heldServed, "cluster-old") &&
			!snapshotReferencesCluster(heldServed, "cluster-new") &&
			hasResource(heldServed.Resources[envoycachetypes.Cluster].Items, "cluster-new")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"an underived CLA must not flip the served route onto the new cluster")
	assertSnapshotCoherent(t, heldServed)

	// Phase 2: a derived-but-EMPTY CLA row appears (the EndpointSlice exists
	// with no ready endpoints). Presence is truth: the flip publishes and the
	// route fails until endpoints arrive — it must not stay pinned (#14352).
	endpointCol.UpdateObject(endpointNewEmpty)

	var flippedServed *envoycache.Snapshot
	g.Eventually(func() bool {
		flippedServed = eventuallyCacheSnapshot(t, cache, nodeID)
		return snapshotReferencesCluster(flippedServed, "cluster-new") &&
			hasResource(flippedServed.Resources[envoycachetypes.Endpoint].Items, "cluster-new")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"a derived-but-empty CLA is the backend's truth; the flip must publish")
	assertSnapshotCoherent(t, flippedServed)

	endpointCol.UpdateObject(endpointNewReady)

	var readyServed *envoycache.Snapshot
	g.Eventually(func() bool {
		readyServed = eventuallyCacheSnapshot(t, cache, nodeID)
		if !snapshotReferencesCluster(readyServed, "cluster-new") {
			return false
		}
		item, ok := readyServed.Resources[envoycachetypes.Endpoint].Items["cluster-new"]
		if !ok {
			return false
		}
		cla, ok := item.Resource.(*envoyendpointv3.ClusterLoadAssignment)
		return ok && len(cla.GetEndpoints()) > 0
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"once endpoints arrive, the served CLA carries them")
	assertSnapshotCoherent(t, readyServed)
}

func TestSnapshotPerClientDefersWeightedRouteUntilAllEndpointsReady(t *testing.T) {
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
	clusterNew := edsClusterForClient(ucc, "cluster-new", 2)
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{clusterOld})
	endpointOld := endpointsForClient(ucc, "cluster-old", 3)
	endpointNew := endpointsForClient(ucc, "cluster-new", 4)
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{endpointOld})

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
	g.Expect(initialServed.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("cluster-old"))
	assertSnapshotCoherent(t, initialServed)

	updatedRoutes := weightedRouteResourcesForClusters("cluster-old", "cluster-new")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.UpdateObject(clusterNew)

	// The weighted split is held while cluster-new has no usable endpoints:
	// the served routes keep sending 100% to cluster-old while cluster-new
	// warms in the served CDS.
	var heldServed *envoycache.Snapshot
	g.Eventually(func() bool {
		heldServed = eventuallyCacheSnapshot(t, cache, nodeID)
		return snapshotReferencesCluster(heldServed, "cluster-old") &&
			!snapshotReferencesCluster(heldServed, "cluster-new") &&
			hasResource(heldServed.Resources[envoycachetypes.Cluster].Items, "cluster-new")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"the weighted split must be held while a referenced cluster's CLA is underived")
	assertSnapshotCoherent(t, heldServed)

	endpointCol.UpdateObject(endpointNew)

	var readyServed *envoycache.Snapshot
	g.Eventually(func() bool {
		readyServed = eventuallyCacheSnapshot(t, cache, nodeID)
		return snapshotReferencesCluster(readyServed, "cluster-old") &&
			snapshotReferencesCluster(readyServed, "cluster-new") &&
			hasResource(readyServed.Resources[envoycachetypes.Endpoint].Items, "cluster-old") &&
			hasResource(readyServed.Resources[envoycachetypes.Endpoint].Items, "cluster-new")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"once both weighted route CLAs contain usable endpoints, the weighted split publishes")
	assertSnapshotCoherent(t, readyServed)
}

func TestSnapshotPerClientDefersUntilReferencedEDSServiceNameHasEndpoints(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})

	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})
	routes := routeResourcesForClusters("cluster-a")
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		edsClusterForClientWithServiceName(ucc, "cluster-a", "backend-service", 1),
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
		newTestPerClientClustersFromCol(clusterCol, uccs),
	)

	// Deferred: the referenced EDS cluster resolves its CLA by service_name,
	// which has no usable endpoint yet (synthesized empty).
	wrap := eventuallyDeferredWrapper(t, snapshots)
	g.Expect(wrap.missingEndpointsReferenced).To(gomega.ConsistOf("cluster-a"))
	g.Expect(wrap.snap.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("backend-service"),
		"the synthesized empty CLA must use the EDS service_name")

	endpointCol.UpdateObject(endpointsForClient(ucc, "backend-service", 3))

	wrap = eventuallyCoherentWrapper(t, snapshots)
	snap := wrap.snap
	g.Expect(snap.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("backend-service"))
	g.Expect(snap.Resources[envoycachetypes.Endpoint].Items).ToNot(gomega.HaveKey("cluster-a"))
	assertSnapshotCoherent(t, snap)
}

func TestSnapshotPerClientServiceNameEdsSnapshotRespondsToNamedADSRequestAfterClusterRemoved(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})

	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	initialRoutes := routeResourcesForClusters("cluster-a", "cluster-b")
	initial := GatewayXdsResources{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             initialRoutes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(initialRoutes, listeners),
	}
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{initial})

	clusterA := edsClusterForClientWithServiceName(ucc, "cluster-a", "service-a", 1)
	clusterB := edsClusterForClientWithServiceName(ucc, "cluster-b", "service-b", 2)
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{clusterA, clusterB})
	endpointA := endpointsForClient(ucc, "service-a", 3)
	endpointB := endpointsForClient(ucc, "service-b", 4)
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{endpointA, endpointB})

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

	initialSnap := eventuallySingleSnapshot(t, snapshots)
	initialEndpointVersion := initialSnap.Resources[envoycachetypes.Endpoint].Version
	g.Expect(initialSnap.Resources[envoycachetypes.Endpoint].Items).To(gomega.HaveKey("service-b"))

	updatedRoutes := routeResourcesForClusters("cluster-a")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.DeleteObject(clusterB.ResourceName())

	var updatedSnap *envoycache.Snapshot
	g.Eventually(func() bool {
		updatedSnap = eventuallyCurrentSnapshot(snapshots)
		if updatedSnap == nil {
			return false
		}
		endpoints := updatedSnap.Resources[envoycachetypes.Endpoint].Items
		_, hasEndpointA := endpoints["service-a"]
		_, hasEndpointB := endpoints["service-b"]
		return hasEndpointA && !hasEndpointB
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue())

	updatedEndpointVersion := updatedSnap.Resources[envoycachetypes.Endpoint].Version
	g.Expect(updatedEndpointVersion).ToNot(gomega.Equal(initialEndpointVersion))

	cache := newTestSnapshotCache(t)
	nodeID := ucc.ResourceName()
	g.Expect(cache.SetSnapshot(context.Background(), nodeID, updatedSnap)).ToNot(gomega.HaveOccurred())

	req := &envoydiscoveryv3.DiscoveryRequest{
		Node:          &envoycorev3.Node{Id: nodeID},
		TypeUrl:       envoyresourcev3.EndpointType,
		ResourceNames: []string{"service-a"},
		VersionInfo:   initialEndpointVersion,
	}
	sub := envoystreamv3.NewSotwSubscription(req.GetResourceNames(), true)
	sub.SetReturnedResources(map[string]string{
		"service-a": initialEndpointVersion,
		"service-b": initialEndpointVersion,
	})
	responses := make(chan envoycache.Response, 1)
	_, err := cache.CreateWatch(req, sub, responses)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	select {
	case response := <-responses:
		g.Expect(response.GetResponseVersion()).To(gomega.Equal(updatedEndpointVersion))
		g.Expect(response.GetReturnedResources()).To(gomega.HaveKeyWithValue("service-a", updatedEndpointVersion))
		g.Expect(response.GetReturnedResources()).ToNot(gomega.HaveKey("service-b"))
		discoveryResponse, err := response.GetDiscoveryResponse()
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(discoveryResponse.GetResources()).To(gomega.HaveLen(1))
	case <-time.After(time.Second):
		t.Fatal("expected service_name-filtered EDS snapshot to answer the named ADS EDS request")
	}
}

func TestSnapshotPerClientEndpointOnlyUpdateOnlyChangesEDSVersion(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	routes := routeResourcesForClusters("cluster-a")
	listeners := sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{
		edsClusterForClient(ucc, "cluster-a", 1),
	})
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{
		endpointsForClient(ucc, "cluster-a", 3),
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

	initialSnap := eventuallySingleSnapshot(t, snapshots)
	initialClusterVersion := initialSnap.Resources[envoycachetypes.Cluster].Version
	initialEndpointVersion := initialSnap.Resources[envoycachetypes.Endpoint].Version
	initialRouteVersion := initialSnap.Resources[envoycachetypes.Route].Version
	initialListenerVersion := initialSnap.Resources[envoycachetypes.Listener].Version

	endpointCol.UpdateObject(endpointsForClient(ucc, "cluster-a", 99))
	g.Consistently(func() string {
		return eventuallyCurrentSnapshot(snapshots).Resources[envoycachetypes.Endpoint].Version
	}, 200*time.Millisecond, 10*time.Millisecond).Should(gomega.Equal(initialEndpointVersion),
		"input-only changes must not move the published EDS version")
	changed := emptyEndpointsForClient(ucc, "cluster-a", 100)
	endpointCol.UpdateObject(changed)

	var updatedSnap *envoycache.Snapshot
	g.Eventually(func() bool {
		updatedSnap = eventuallyCurrentSnapshot(snapshots)
		return updatedSnap != nil && updatedSnap.Resources[envoycachetypes.Endpoint].Version != initialEndpointVersion
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue())

	g.Expect(updatedSnap.Resources[envoycachetypes.Cluster].Version).To(gomega.Equal(initialClusterVersion),
		"CDS version should not change for an endpoint-only update")
	g.Expect(updatedSnap.Resources[envoycachetypes.Route].Version).To(gomega.Equal(initialRouteVersion),
		"RDS version should not change for an endpoint-only update")
	g.Expect(updatedSnap.Resources[envoycachetypes.Listener].Version).To(gomega.Equal(initialListenerVersion),
		"LDS version should not change for an endpoint-only update")
}

func TestSnapshotPerClientPartialUpdateForOneClientDoesNotPoisonAnotherClient(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	clientA := ir.NewUniquelyConnectedClient(role, "pod-ns", map[string]string{"client": "a"}, ir.PodLocality{})
	clientB := ir.NewUniquelyConnectedClient(role, "pod-ns", map[string]string{"client": "b"}, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{clientA, clientB})

	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	initialRoutes := routeResourcesForClusters("cluster-old")
	initial := GatewayXdsResources{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             initialRoutes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(initialRoutes, listeners),
	}
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{initial})

	clusterOldA := edsClusterForClient(clientA, "cluster-old", 1)
	clusterOldB := edsClusterForClient(clientB, "cluster-old", 2)
	clusterNewA := edsClusterForClient(clientA, "cluster-new", 3)
	clusterNewB := edsClusterForClient(clientB, "cluster-new", 4)
	clusterCol := krt.NewStaticCollection[uccWithCluster](nil, []uccWithCluster{clusterOldA, clusterOldB})
	endpointOldA := endpointsForClient(clientA, "cluster-old", 5)
	endpointOldB := endpointsForClient(clientB, "cluster-old", 6)
	endpointNewB := endpointsForClient(clientB, "cluster-new", 7)
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, []UccWithEndpoints{endpointOldA, endpointOldB})

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
	translator := NewProxyTranslator(cache, nil, 0, true)
	snapshots.RegisterBatch(func(events []krt.Event[XdsSnapWrapper]) {
		for _, event := range events {
			if event.Event == controllers.EventDelete {
				continue
			}
			translator.syncXds(context.Background(), event.Latest())
		}
	}, true)

	initialServedA := eventuallyCacheSnapshot(t, cache, clientA.ResourceName())
	initialServedB := eventuallyCacheSnapshot(t, cache, clientB.ResourceName())
	g.Expect(snapshotReferencesCluster(initialServedA, "cluster-old")).To(gomega.BeTrue())
	g.Expect(snapshotReferencesCluster(initialServedB, "cluster-old")).To(gomega.BeTrue())

	updatedRoutes := routeResourcesForClusters("cluster-new")
	updated := initial
	updated.Routes = updatedRoutes
	updated.ReferencedClusters = collectReferencedClusters(updatedRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)
	clusterCol.UpdateObject(clusterNewA)
	clusterCol.UpdateObject(clusterNewB)
	endpointCol.UpdateObject(endpointNewB)

	var servedA *envoycache.Snapshot
	var servedB *envoycache.Snapshot
	g.Eventually(func() bool {
		servedA = eventuallyCacheSnapshot(t, cache, clientA.ResourceName())
		servedB = eventuallyCacheSnapshot(t, cache, clientB.ResourceName())
		return snapshotReferencesCluster(servedA, "cluster-old") &&
			!snapshotReferencesCluster(servedA, "cluster-new") &&
			snapshotReferencesCluster(servedB, "cluster-new") &&
			hasResource(servedB.Resources[envoycachetypes.Endpoint].Items, "cluster-new")
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"client A should retain old cache while client B publishes coherent new state")
}

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
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})
	pcc, _ := newTestPerClientClusters([]uccWithCluster{
		{
			Client:         ucc,
			Name:           "cluster-a",
			Cluster:        sharedproto.Wrap(&envoyclusterv3.Cluster{Name: "cluster-a"}),
			ClusterVersion: 1,
		},
		{
			Client:         ucc,
			Name:           "cluster-b",
			Cluster:        sharedproto.Wrap(&envoyclusterv3.Cluster{Name: "cluster-b"}),
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
		pcc,
	)

	g.Eventually(func() int {
		return len(snapshots.List())
	}, time.Second, 20*time.Millisecond).Should(gomega.Equal(1))

	snap := snapshots.List()[0]
	g.Expect(snap.erroredClusters).To(gomega.ConsistOf("cluster-b"))
	g.Expect(snap.snap.Resources[envoycachetypes.Cluster].Items).ToNot(gomega.HaveKey("cluster-b"))
}

// TestCollectReferencedClusters_ExcludesAncillaryReferences verifies that
// cluster names reachable only through ancillary / control-plane
// typed_config (access-log GrpcService, JWT jwks HttpUri, etc.) are
// deliberately NOT treated as gated dataplane targets. The plugin that
// emits these filters is responsible for also emitting the referenced
// cluster in the same per-gateway snapshot's ExtraClusters, so there is
// no reconnect race between them. Gating on ancillary references would
// starve the gateway forever on a plugin bug — which is not what this
// readiness guard is for.
func TestCollectReferencedClusters_ExcludesAncillaryReferences(t *testing.T) {
	g := gomega.NewWithT(t)

	hcm := &envoyhttpv3.HttpConnectionManager{
		AccessLog: []*envoyaccesslogv3.AccessLog{
			{
				Name: "envoy.access_loggers.http_grpc",
				ConfigType: &envoyaccesslogv3.AccessLog_TypedConfig{
					TypedConfig: mustMessageToAny(t, &envoygrpcaccesslogv3.HttpGrpcAccessLogConfig{
						CommonConfig: &envoygrpcaccesslogv3.CommonGrpcAccessLogConfig{
							TransportApiVersion: envoycorev3.ApiVersion_V3,
							LogName:             "grpc-log",
							GrpcService: &envoycorev3.GrpcService{
								TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
									EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{
										ClusterName: "access-log-cluster",
									},
								},
							},
						},
					}),
				},
			},
		},
		HttpFilters: []*envoyhttpv3.HttpFilter{
			{
				Name: "envoy.filters.http.jwt_authn",
				ConfigType: &envoyhttpv3.HttpFilter_TypedConfig{
					TypedConfig: mustMessageToAny(t, &envoyjwtauthnv3.JwtAuthentication{
						Providers: map[string]*envoyjwtauthnv3.JwtProvider{
							"provider": {
								JwksSourceSpecifier: &envoyjwtauthnv3.JwtProvider_RemoteJwks{
									RemoteJwks: &envoyjwtauthnv3.RemoteJwks{
										HttpUri: &envoycorev3.HttpUri{
											Uri: "https://example.com/jwks",
											HttpUpstreamType: &envoycorev3.HttpUri_Cluster{
												Cluster: "jwks-cluster",
											},
											Timeout: durationpb.New(time.Second),
										},
									},
								},
							},
						},
					}),
				},
			},
		},
	}

	listeners := sliceToResources([]*envoylistenerv3.Listener{
		{
			Name: "listener",
			FilterChains: []*envoylistenerv3.FilterChain{
				{
					Filters: []*envoylistenerv3.Filter{
						{
							Name: envoywellknown.HTTPConnectionManager,
							ConfigType: &envoylistenerv3.Filter_TypedConfig{
								TypedConfig: mustMessageToAny(t, hcm),
							},
						},
					},
				},
			},
		},
	})

	referenced := collectReferencedClusters(envoycache.Resources{}, listeners)

	g.Expect(referenced).ToNot(gomega.HaveKey("access-log-cluster"),
		"ancillary access-log cluster must not be treated as a gated dataplane target")
	g.Expect(referenced).ToNot(gomega.HaveKey("jwks-cluster"),
		"ancillary JWT jwks cluster must not be treated as a gated dataplane target")
}

// TestFindMissingReferencedClusters_HandlesScalarValueMaps verifies that the
// protoreflect walker does not panic when it encounters a proto field of type
// map<string, scalar> (e.g. map<string, string>). Without the IsMap/IsList
// guard on the fall-through branch, such fields fall through to a code path
// that calls v.Message() on a Map-kind Value and panics with
// "type mismatch: cannot convert map to message".
func TestFindMissingReferencedClusters_HandlesScalarValueMaps(t *testing.T) {
	g := gomega.NewWithT(t)

	hcm := &envoyhttpv3.HttpConnectionManager{
		HttpFilters: []*envoyhttpv3.HttpFilter{
			{
				Name: "envoy.filters.http.ext_authz",
				ConfigType: &envoyhttpv3.HttpFilter_TypedConfig{
					TypedConfig: mustMessageToAny(t, &envoyextauthzv3.ExtAuthzPerRoute{
						Override: &envoyextauthzv3.ExtAuthzPerRoute_CheckSettings{
							CheckSettings: &envoyextauthzv3.CheckSettings{
								ContextExtensions: map[string]string{
									"key1": "value1",
									"key2": "value2",
								},
							},
						},
					}),
				},
			},
		},
	}

	listeners := sliceToResources([]*envoylistenerv3.Listener{
		{
			Name: "listener",
			FilterChains: []*envoylistenerv3.FilterChain{
				{
					Filters: []*envoylistenerv3.Filter{
						{
							Name: envoywellknown.HTTPConnectionManager,
							ConfigType: &envoylistenerv3.Filter_TypedConfig{
								TypedConfig: mustMessageToAny(t, hcm),
							},
						},
					},
				},
			},
		},
	})

	g.Expect(func() {
		referenced := collectReferencedClusters(envoycache.Resources{}, listeners)
		findMissingReferencedClusters(referenced, nil, nil)
	}).ToNot(gomega.Panic())
}

// TestSnapshotPerClientPublishesEvenWithUnresolvableBackendRef verifies that
// a user BackendRef typo — e.g. an HTTPRoute pointing at a Service that does
// not exist — does not starve the readiness gate on startup. IR-time
// resolution substitutes wellknown.BlackholeClusterName for the unresolved
// target, and the gate explicitly skips blackhole so valid routes still
// reach Envoy.
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
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
	}})

	pcc, _ := newTestPerClientClusters([]uccWithCluster{
		{
			Client:         ucc,
			Name:           "cluster-a",
			Cluster:        sharedproto.Wrap(&envoyclusterv3.Cluster{Name: "cluster-a"}),
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
		pcc,
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
// target, which the gate skips, so the snapshot re-publishes with the new
// route set rather than being withdrawn. The existing good route keeps
// flowing; the typo'd route blackholes in Envoy — no 500/NC for valid
// traffic.
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
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             goodRoutes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(goodRoutes, listeners),
	}
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{initial})

	pcc, _ := newTestPerClientClusters([]uccWithCluster{
		{
			Client:         ucc,
			Name:           "cluster-a",
			Cluster:        sharedproto.Wrap(&envoyclusterv3.Cluster{Name: "cluster-a"}),
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
		pcc,
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
	updated.ReferencedClusters = collectReferencedClusters(badRoutes, listeners)
	mostXdsSnapshots.UpdateObject(updated)

	g.Consistently(func() int {
		return len(snapshots.List())
	}, 500*time.Millisecond, 20*time.Millisecond).Should(gomega.Equal(1),
		"snapshot must stay published after an unresolvable BackendRef arrives; withdrawing it strands Envoy on restart")
}

// TestSnapshotPerClientPublishesWhenAllRoutesAreRedirectOnly pins the
// defensive behaviour of snapshotPerClient when the per-client clusters
// collection is empty for a UCC. In production this is hard to reach because
// finalBackends emits a BackendObjectIR for every Service port in the
// cluster, so the per-client clusters collection is non-empty even for a
// gateway whose HTTPRoutes only use RequestRedirect or DirectResponse. The
// test constructs that condition synthetically — zero entries in the cluster
// collection and zero referenced clusters in the route config — to exercise
// the branch that treats a nil clusterSnapshot entry as "zero clusters"
// rather than "not yet computed". Without that branch the snapshot would
// stall indefinitely whenever the path is reached (controller starting
// before Service informers sync, or a deployment whose only backends are
// non-K8s and all fail translation); with it, publishing proceeds because
// the referenced-cluster gate has nothing to wait for.
func TestSnapshotPerClientPublishesWhenAllRoutesAreRedirectOnly(t *testing.T) {
	g := gomega.NewWithT(t)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	// Routes with only a Redirect action — no cluster references.
	routes := sliceToResources([]*envoyroutev3.RouteConfiguration{
		{
			Name: "route-config",
			VirtualHosts: []*envoyroutev3.VirtualHost{{
				Name:    "vhost",
				Domains: []string{"*"},
				Routes: []*envoyroutev3.Route{{
					Name: "redirect-only",
					Action: &envoyroutev3.Route_Redirect{
						Redirect: &envoyroutev3.RedirectAction{
							SchemeRewriteSpecifier: &envoyroutev3.RedirectAction_HttpsRedirect{HttpsRedirect: true},
						},
					},
				}},
			}},
		},
	})
	listeners := sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}})
	referencedClusters := collectReferencedClusters(routes, listeners)
	g.Expect(referencedClusters).To(gomega.BeEmpty(), "redirect-only routes must not produce cluster references")

	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: referencedClusters,
	}})

	// No per-client clusters at all for this UCC — simulates a deployment with
	// only redirect-only routes and no Service-backed backends contributing
	// clusters. The client-keyed collection still emits a complete empty row.
	pcc := newTestPerClientClustersFromCol(krt.NewStaticCollection[uccWithCluster](nil, nil), uccs)
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
		pcc,
	)

	g.Eventually(func() int {
		return len(snapshots.List())
	}, 2*time.Second, 20*time.Millisecond).Should(gomega.Equal(1),
		"a snapshot must publish even when there are zero per-client clusters and zero referenced clusters")

	snap := snapshots.List()[0].snap
	g.Expect(snap.Resources[envoycachetypes.Cluster].Items).To(gomega.BeEmpty())
	g.Expect(snap.Resources[envoycachetypes.Listener].Items).To(gomega.HaveKey("listener"))
}

func mapKeys[M ~map[K]V, K comparable, V any](m M) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func httpListenerWithRDS(t *testing.T, name, routeConfigName string) *envoylistenerv3.Listener {
	t.Helper()

	hcm := &envoyhttpv3.HttpConnectionManager{
		RouteSpecifier: &envoyhttpv3.HttpConnectionManager_Rds{
			Rds: &envoyhttpv3.Rds{
				RouteConfigName: routeConfigName,
			},
		},
	}
	return &envoylistenerv3.Listener{
		Name: name,
		FilterChains: []*envoylistenerv3.FilterChain{{
			Name: "http",
			Filters: []*envoylistenerv3.Filter{{
				Name: envoywellknown.HTTPConnectionManager,
				ConfigType: &envoylistenerv3.Filter_TypedConfig{
					TypedConfig: mustMessageToAny(t, hcm),
				},
			}},
		}},
	}
}

func routeResourcesForClusters(clusterNames ...string) envoycache.Resources {
	routes := make([]*envoyroutev3.Route, 0, len(clusterNames))
	for _, clusterName := range clusterNames {
		routes = append(routes, &envoyroutev3.Route{
			Name: "route-" + clusterName,
			Action: &envoyroutev3.Route_Route{
				Route: &envoyroutev3.RouteAction{
					ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: clusterName},
				},
			},
		})
	}

	return sliceToResources([]*envoyroutev3.RouteConfiguration{
		{
			Name: "route-config",
			VirtualHosts: []*envoyroutev3.VirtualHost{
				{
					Name:    "vhost",
					Domains: []string{"*"},
					Routes:  routes,
				},
			},
		},
	})
}

func weightedRouteResourcesForClusters(clusterNames ...string) envoycache.Resources {
	clusters := make([]*envoyroutev3.WeightedCluster_ClusterWeight, 0, len(clusterNames))
	for _, clusterName := range clusterNames {
		clusters = append(clusters, &envoyroutev3.WeightedCluster_ClusterWeight{
			Name:   clusterName,
			Weight: wrapperspb.UInt32(1),
		})
	}

	return sliceToResources([]*envoyroutev3.RouteConfiguration{
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
											Clusters: clusters,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
}

func edsClusterForClient(ucc ir.UniquelyConnectedClient, name string, version uint64) uccWithCluster {
	return uccWithCluster{
		Client: ucc,
		Name:   name,
		Cluster: sharedproto.Wrap(&envoyclusterv3.Cluster{
			Name: name,
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
				Type: envoyclusterv3.Cluster_EDS,
			},
		}),
		ClusterVersion: version,
	}
}

func edsClusterForClientWithServiceName(ucc ir.UniquelyConnectedClient, name, serviceName string, version uint64) uccWithCluster {
	return uccWithCluster{
		Client: ucc,
		Name:   name,
		Cluster: sharedproto.Wrap(&envoyclusterv3.Cluster{
			Name: name,
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
				Type: envoyclusterv3.Cluster_EDS,
			},
			EdsClusterConfig: &envoyclusterv3.Cluster_EdsClusterConfig{
				ServiceName: serviceName,
			},
		}),
		ClusterVersion: version,
	}
}

func endpointsForClient(ucc ir.UniquelyConnectedClient, name string, hash uint64) UccWithEndpoints {
	cla := &envoyendpointv3.ClusterLoadAssignment{ClusterName: name}
	cla.Endpoints = []*envoyendpointv3.LocalityLbEndpoints{
		{
			LbEndpoints: []*envoyendpointv3.LbEndpoint{
				{
					HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
						Endpoint: &envoyendpointv3.Endpoint{
							Address: &envoycorev3.Address{
								Address: &envoycorev3.Address_SocketAddress{
									SocketAddress: &envoycorev3.SocketAddress{
										Address: "127.0.0.1",
										PortSpecifier: &envoycorev3.SocketAddress_PortValue{
											PortValue: 8080,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return UccWithEndpoints{
		Client:        ucc,
		Endpoints:     sharedproto.Wrap(cla),
		EndpointsHash: hash,
		ContentHash:   contentHashOf(cla),
		endpointsName: name,
	}
}

func emptyEndpointsForClient(ucc ir.UniquelyConnectedClient, name string, hash uint64) UccWithEndpoints {
	cla := &envoyendpointv3.ClusterLoadAssignment{ClusterName: name}
	return UccWithEndpoints{
		Client:        ucc,
		Endpoints:     sharedproto.Wrap(cla),
		EndpointsHash: hash,
		ContentHash:   contentHashOf(cla),
		endpointsName: name,
	}
}

func eventuallySingleSnapshot(t *testing.T, snapshots krt.Collection[XdsSnapWrapper]) *envoycache.Snapshot {
	t.Helper()

	g := gomega.NewWithT(t)
	var snap *envoycache.Snapshot
	g.Eventually(func() bool {
		snap = eventuallyCurrentSnapshot(snapshots)
		return snap != nil
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue())

	return snap
}

func eventuallyCacheSnapshot(t *testing.T, cache envoycache.SnapshotCache, nodeID string) *envoycache.Snapshot {
	t.Helper()

	g := gomega.NewWithT(t)
	var snap *envoycache.Snapshot
	g.Eventually(func() bool {
		resourceSnapshot, err := cache.GetSnapshot(nodeID)
		if err != nil {
			return false
		}
		var ok bool
		snap, ok = resourceSnapshot.(*envoycache.Snapshot)
		return ok
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue())

	return snap
}

func eventuallyCurrentSnapshot(snapshots krt.Collection[XdsSnapWrapper]) *envoycache.Snapshot {
	list := snapshots.List()
	if len(list) != 1 {
		return nil
	}
	return list[0].snap
}

func hasResource(resources map[string]envoycachetypes.ResourceWithTTL, name string) bool {
	_, ok := resources[name]
	return ok
}

func snapshotReferencesCluster(snap *envoycache.Snapshot, name string) bool {
	if snap == nil {
		return false
	}
	for _, item := range snap.Resources[envoycachetypes.Route].Items {
		routeConfig, ok := item.Resource.(*envoyroutev3.RouteConfiguration)
		if !ok {
			continue
		}
		for _, virtualHost := range routeConfig.GetVirtualHosts() {
			for _, route := range virtualHost.GetRoutes() {
				if route.GetRoute().GetCluster() == name {
					return true
				}
				for _, cluster := range route.GetRoute().GetWeightedClusters().GetClusters() {
					if cluster.GetName() == name {
						return true
					}
				}
			}
		}
	}
	return false
}

func mustMessageToAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()

	out, err := utils.MessageToAny(msg)
	if err != nil {
		t.Fatalf("marshal Any: %v", err)
	}
	return out
}

func eventuallyDeferredWrapper(t *testing.T, snapshots krt.Collection[XdsSnapWrapper]) XdsSnapWrapper {
	t.Helper()

	g := gomega.NewWithT(t)
	var wrap XdsSnapWrapper
	g.Eventually(func() bool {
		list := snapshots.List()
		if len(list) != 1 {
			return false
		}
		wrap = list[0]
		return wrap.deferred
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"expected a single wrapper marked deferred")
	return wrap
}

func eventuallyCoherentWrapper(t *testing.T, snapshots krt.Collection[XdsSnapWrapper]) XdsSnapWrapper {
	t.Helper()

	g := gomega.NewWithT(t)
	var wrap XdsSnapWrapper
	g.Eventually(func() bool {
		list := snapshots.List()
		if len(list) != 1 {
			return false
		}
		wrap = list[0]
		return !wrap.deferred
	}, time.Second, 20*time.Millisecond).Should(gomega.BeTrue(),
		"expected a single coherent (non-deferred) wrapper")
	return wrap
}

// registerSyncXds wires a test ProxyTranslator to the wrapper collection the
// way proxy_syncer does in production: every non-delete event is pushed
// through syncXds against the translator's snapshot cache.
func registerSyncXds(snapshots krt.Collection[XdsSnapWrapper], translator ProxyTranslator) {
	snapshots.RegisterBatch(func(events []krt.Event[XdsSnapWrapper]) {
		for _, event := range events {
			if event.Event == controllers.EventDelete {
				continue
			}
			translator.syncXds(context.Background(), event.Latest())
		}
	}, true)
}

// assertSnapshotCoherent verifies the published snapshot's internal
// invariants: go-control-plane consistency (every EDS cluster has exactly one
// CLA, no CLA without a cluster) and reference closure (every route/listener-
// referenced cluster exists in CDS, so Envoy cannot 500/NC on a published
// route).
func assertSnapshotCoherent(t *testing.T, snap *envoycache.Snapshot) {
	t.Helper()
	if err := snap.Consistent(); err != nil {
		t.Fatalf("snapshot not consistent: %v", err)
	}
	refs := collectReferencedClusters(
		snap.Resources[envoycachetypes.Route],
		snap.Resources[envoycachetypes.Listener],
	)
	clusters := snap.Resources[envoycachetypes.Cluster].Items
	for name := range refs {
		if name == wellknown.BlackholeClusterName {
			continue
		}
		if _, ok := clusters[name]; !ok {
			t.Fatalf("route/listener references cluster %q absent from CDS", name)
		}
	}
}

// TestEndpointSetVersionFollowsContentNotInputs pins the property the EDS
// version is built for: two rows whose inputs differ (different EndpointsHash,
// as after a backend policy generation bump or an upgrade that changes the
// input hash function) but whose assignments are byte-identical publish under
// the same version, so nothing is pushed; a content change moves it; and the
// fold does not depend on order.
func TestEndpointSetVersionFollowsContentNotInputs(t *testing.T) {
	g := gomega.NewWithT(t)
	a := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a", Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{}}}
	b := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "b"}

	same := map[string]uint64{"a": contentHashOf(a), "b": contentHashOf(b)}
	g.Expect(endpointSetVersion(same, nil, nil)).To(gomega.Equal(endpointSetVersion(map[string]uint64{"b": contentHashOf(b), "a": contentHashOf(a)}, nil, nil)),
		"the fold must not depend on map order")

	changed := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a", Endpoints: []*envoyendpointv3.LocalityLbEndpoints{{}, {}}}
	g.Expect(endpointSetVersion(map[string]uint64{"a": contentHashOf(changed), "b": contentHashOf(b)}, nil, nil)).ToNot(gomega.Equal(endpointSetVersion(same, nil, nil)),
		"a content change must move the version")

	// Equal per-assignment digests under different names must not cancel.
	g.Expect(endpointSetVersion(map[string]uint64{"x": 7, "y": 7}, nil, nil)).ToNot(gomega.Equal(endpointSetVersion(map[string]uint64{}, nil, nil)),
		"two equal digests must not fold to the empty set's version")

	// Rows with different input hashes and equal content compare equal on
	// content and version, and different on inputs; KRT sees the input change
	// (the row is recomputed) but the published version does not move.
	row1 := UccWithEndpoints{Endpoints: sharedproto.Wrap(a), EndpointsHash: 1, ContentHash: contentHashOf(a), endpointsName: "a"}
	row2 := UccWithEndpoints{Endpoints: sharedproto.Wrap(a), EndpointsHash: 2, ContentHash: contentHashOf(a), endpointsName: "a"}
	g.Expect(row1.Equals(row2)).To(gomega.BeFalse(), "an input change is still a row change")
	g.Expect(endpointSetVersion(map[string]uint64{"a": row1.ContentHash}, nil, nil)).To(gomega.Equal(endpointSetVersion(map[string]uint64{"a": row2.ContentHash}, nil, nil)),
		"the published EDS version must not move when only inputs changed")

	// The reverse: equal inputs (a colliding or stale input hash) with
	// different content is a row change, because ContentHash is compared too.
	row3 := UccWithEndpoints{Endpoints: sharedproto.Wrap(changed), EndpointsHash: 1, ContentHash: contentHashOf(changed), endpointsName: "a"}
	g.Expect(row1.Equals(row3)).To(gomega.BeFalse(), "a content change behind an equal input hash must be a row change")
}

// TestEndpointSetVersionFollowsClusterVersion pins the other half of the EDS
// version: a change to the cluster an assignment belongs to moves the EDS
// version even when the assignment is byte-identical, because Envoy rebuilds
// the cluster and asks for its endpoints again at the version it last
// accepted, and the cache answers only a different version. A cluster that
// has no assignment in the set does not affect it.
func TestEndpointSetVersionFollowsClusterVersion(t *testing.T) {
	g := gomega.NewWithT(t)
	a := &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a"}
	content := map[string]uint64{"a": contentHashOf(a)}

	before := endpointSetVersion(content, nil, map[string]uint64{"a": 1, "unrelated": 5})
	g.Expect(endpointSetVersion(content, nil, map[string]uint64{"a": 1, "unrelated": 6})).To(gomega.Equal(before),
		"a cluster without an assignment in the set must not move the EDS version")
	g.Expect(endpointSetVersion(content, nil, map[string]uint64{"a": 2, "unrelated": 5})).ToNot(gomega.Equal(before),
		"a change to the assignment's own cluster must move the EDS version so the rebuilt cluster is answered")
	g.Expect(endpointSetVersion(content, nil, nil)).ToNot(gomega.Equal(before),
		"absent cluster digests are a distinct state from digest 1")
}
