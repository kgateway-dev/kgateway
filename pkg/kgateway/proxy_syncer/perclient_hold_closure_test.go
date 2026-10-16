package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyextauthzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoyhttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listenerCallingAuthz is an HTTP listener whose ext_authz filter calls
// clusterName, routing through RDS route-config.
func listenerCallingAuthz(t *testing.T, clusterName string) *envoylistenerv3.Listener {
	t.Helper()
	authz := &envoyextauthzv3.ExtAuthz{Services: &envoyextauthzv3.ExtAuthz_GrpcService{GrpcService: &envoycorev3.GrpcService{
		TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{ClusterName: clusterName}},
	}}}
	hcm := &envoyhttpv3.HttpConnectionManager{
		RouteSpecifier: &envoyhttpv3.HttpConnectionManager_Rds{Rds: &envoyhttpv3.Rds{RouteConfigName: "route-config"}},
		HttpFilters: []*envoyhttpv3.HttpFilter{{
			Name:       envoywellknown.HTTPExternalAuthorization,
			ConfigType: &envoyhttpv3.HttpFilter_TypedConfig{TypedConfig: mustMessageToAny(t, authz)},
		}},
	}
	return &envoylistenerv3.Listener{Name: "listener", FilterChains: []*envoylistenerv3.FilterChain{{
		Name: "http",
		Filters: []*envoylistenerv3.Filter{{
			Name:       envoywellknown.HTTPConnectionManager,
			ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: mustMessageToAny(t, hcm)},
		}},
	}}}
}

// TestHeldFlipKeepsClustersTheHeldListenerCalls: while a route flip is held,
// the published routes AND listeners stay live, so every cluster they depend
// on must stay in CDS -- not only the routes' targets. Here one change both
// moves the route onto a not-yet-derived cluster and drops the ext_authz
// backend from the build; the held listener still calls ext_authz, so its
// cluster (and CLA) must be carried for as long as the hold lasts.
func TestHeldFlipKeepsClustersTheHeldListenerCalls(t *testing.T) {
	pt := newPublishGateTestTranslator(t, true, time.Hour)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()
	ucc := newTestUcc(publishGateTestClient)

	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Listener] = sliceToResources([]*envoylistenerv3.Listener{listenerCallingAuthz(t, "auth-cluster")})
	published.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-old")
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-1", []envoycachetypes.ResourceWithTTL{
		edsClusterForClient(ucc, "cluster-old", 1).Cluster.ResourceWithTTL(),
		edsClusterForClient(ucc, "auth-cluster", 2).Cluster.ResourceWithTTL(),
	})
	published.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-1", []envoycachetypes.ResourceWithTTL{
		endpointsForClient(ucc, "cluster-old", 3).Endpoints.ResourceWithTTL(),
		endpointsForClient(ucc, "auth-cluster", 4).Endpoints.ResourceWithTTL(),
	})
	require.NoError(t, pt.gate.publish(ctx, pt.xdsCache, XdsSnapWrapper{snap: published, proxyKey: publishGateTestClient}))

	build := &envoycache.Snapshot{}
	build.Resources[envoycachetypes.Listener] = sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	build.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-new")
	build.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-2", []envoycachetypes.ResourceWithTTL{
		edsClusterForClient(ucc, "cluster-new", 5).Cluster.ResourceWithTTL(),
	})
	build.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-2", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "cluster-new"}},
	})
	pt.syncXds(ctx, XdsSnapWrapper{
		snap:                       build,
		proxyKey:                   publishGateTestClient,
		missingEndpointsReferenced: []string{"cluster-new"},
		synthesizedEndpoints:       []string{"cluster-new"},
	})

	served := servedSnapshot(t, pt)
	require.True(t, snapshotReferencesCluster(served, "cluster-old"), "the flip onto the underived cluster is held")
	assert.True(t, hasResource(served.Resources[envoycachetypes.Cluster].Items, "auth-cluster"),
		"the held listener's ext_authz filter calls auth-cluster; it must stay in CDS while the hold lasts")
	assert.True(t, hasResource(served.Resources[envoycachetypes.Endpoint].Items, "auth-cluster"),
		"a carried cluster travels with its CLA")
	assert.True(t, hasResource(served.Resources[envoycachetypes.Cluster].Items, "cluster-old"),
		"the held routes' target is carried too")
}

// TestCarryKeepsBuiltAssignmentSharedByServiceName: two clusters share one EDS
// assignment through EdsClusterConfig.service_name. When one is carried and the
// other is built, the shared assignment must be the built one: the carried
// copy is stale for the built cluster, and the EDS version describes the built
// content.
func TestCarryKeepsBuiltAssignmentSharedByServiceName(t *testing.T) {
	pt := newPublishGateTestTranslator(t, true, time.Hour)
	t.Cleanup(func() { pt.gate.clientDeparted(publishGateTestClient) })
	ctx := context.Background()
	ucc := newTestUcc(publishGateTestClient)
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})

	c1 := edsClusterForClientWithServiceName(ucc, "c1", "shared", 1)
	c2 := edsClusterForClientWithServiceName(ucc, "c2", "shared", 2)
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Listener] = listeners
	published.Resources[envoycachetypes.Route] = routeResourcesForClusters("c1", "c2")
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-1",
		[]envoycachetypes.ResourceWithTTL{c1.Cluster.ResourceWithTTL(), c2.Cluster.ResourceWithTTL()})
	published.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-1",
		[]envoycachetypes.ResourceWithTTL{endpointsForClient(ucc, "shared", 10).Endpoints.ResourceWithTTL()})
	require.NoError(t, pt.gate.publish(ctx, pt.xdsCache, XdsSnapWrapper{snap: published, proxyKey: publishGateTestClient}))

	// c1 is missing from this build (carried); c2 is built against a derived,
	// now-empty "shared" assignment (the backend scaled to zero).
	build := &envoycache.Snapshot{}
	build.Resources[envoycachetypes.Listener] = listeners
	build.Resources[envoycachetypes.Route] = routeResourcesForClusters("c1", "c2")
	build.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-2",
		[]envoycachetypes.ResourceWithTTL{c2.Cluster.ResourceWithTTL()})
	build.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-2",
		[]envoycachetypes.ResourceWithTTL{emptyEndpointsForClient(ucc, "shared", 20).Endpoints.ResourceWithTTL()})
	pt.syncXds(ctx, XdsSnapWrapper{snap: build, proxyKey: publishGateTestClient, missingReferenced: []string{"c1"}})

	served := servedSnapshot(t, pt)
	require.True(t, hasResource(served.Resources[envoycachetypes.Cluster].Items, "c1"), "c1 is carried")
	cla, ok := served.Resources[envoycachetypes.Endpoint].Items["shared"].Resource.(*envoyendpointv3.ClusterLoadAssignment)
	require.True(t, ok)
	assert.Empty(t, cla.GetEndpoints(), "the shared assignment is c2's built (empty) one, not c1's carried copy")
}
