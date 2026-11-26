package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A dropped backend's own listener still ships, so it can name a cluster that
// scoping removed from CDS. A route that retargets onto that cluster before its
// CLA derives is still held: the client never had the cluster, so it has no
// endpoints to keep. Unscoped, a cluster the published config names is treated
// as live, as before scoping.
func TestRetargetOntoAClusterOnlyAShippedBackendListenerNamesIsHeld(t *testing.T) {
	served, published := retargetOntoAClusterOnlyABackendListenerNames(t, true)
	assert.Equal(t, published.GetVersion(resourcev3.RouteType), served.GetVersion(resourcev3.RouteType),
		"the retarget waits for proxy's endpoints; the tunnel's listener naming proxy does not make it live")
	assert.True(t, hasResource(served.Resources[envoycachetypes.Cluster].Items, "proxy"), "proxy's CDS still goes out so it can warm")
}

func TestRetargetOntoAClusterABackendListenerNamesPublishesUnscoped(t *testing.T) {
	served, published := retargetOntoAClusterOnlyABackendListenerNames(t, false)
	assert.NotEqual(t, published.GetVersion(resourcev3.RouteType), served.GetVersion(resourcev3.RouteType),
		"unscoped, the gate keeps its behavior from before scoping")
}

func retargetOntoAClusterOnlyABackendListenerNames(t *testing.T, scoped bool) (served, published *envoycache.Snapshot) {
	t.Helper()
	pt := newPublishGateTestTranslator(t, false, time.Hour)
	ctx := context.Background()
	ucc := newTestUcc(publishGateTestClient)

	listeners := sliceToResources([]*envoylistenerv3.Listener{
		httpListenerWithRDS(t, "listener", "route-config"),
		tunnelListener("tunnel_unused", "proxy"),
	})
	oldCluster := edsClusterForClient(ucc, "cluster-old", 1)
	oldCLA := endpointsForClient(ucc, "cluster-old", 2)
	published = &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Listener] = listeners
	published.Resources[envoycachetypes.Route] = routeResourcesForClusters("cluster-old")
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-old", []envoycachetypes.ResourceWithTTL{
		oldCluster.Cluster.ResourceWithTTL(),
	})
	published.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-old", []envoycachetypes.ResourceWithTTL{
		oldCLA.Endpoints.ResourceWithTTL(),
	})
	require.NoError(t, pt.gate.publish(ctx, pt.xdsCache, XdsSnapWrapper{snap: published, proxyKey: publishGateTestClient}))

	// Retarget onto proxy before its CLA derives.
	proxy := edsClusterForClient(ucc, "proxy", 3)
	retarget := &envoycache.Snapshot{}
	retarget.Resources[envoycachetypes.Listener] = listeners
	retarget.Resources[envoycachetypes.Route] = routeResourcesForClusters("proxy")
	retarget.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds-new", []envoycachetypes.ResourceWithTTL{
		oldCluster.Cluster.ResourceWithTTL(),
		proxy.Cluster.ResourceWithTTL(),
	})
	retarget.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("eds-new", []envoycachetypes.ResourceWithTTL{
		oldCLA.Endpoints.ResourceWithTTL(),
		emptyEndpointsForClient(ucc, "proxy", 4).Endpoints.ResourceWithTTL(),
	})
	pt.syncXds(ctx, XdsSnapWrapper{
		snap:                       retarget,
		proxyKey:                   publishGateTestClient,
		missingEndpointsReferenced: []string{"proxy"},
		scopesClusters:             scoped,
	})

	return servedSnapshot(t, pt), published
}
