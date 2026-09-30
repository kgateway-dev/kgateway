package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clusterWrapperV builds a coherent wrapper publishing exactly these clusters,
// each with its ClusterLoadAssignment, so a carried-forward cluster can be
// checked to bring its endpoints with it.
func clusterWrapperV(version string, clusters ...string) XdsSnapWrapper {
	clusterItems := make([]envoycachetypes.ResourceWithTTL, 0, len(clusters))
	endpointItems := make([]envoycachetypes.ResourceWithTTL, 0, len(clusters))
	for _, name := range clusters {
		clusterItems = append(clusterItems, envoycachetypes.ResourceWithTTL{Resource: edsClusterProto(name)})
		endpointItems = append(endpointItems, envoycachetypes.ResourceWithTTL{
			Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: name},
		})
	}
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Listener] = envoycache.NewResources(version, nil)
	snap.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL(version, clusterItems)
	snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL(version, endpointItems)
	return XdsSnapWrapper{snap: snap, proxyKey: publishGateTestClient}
}

func publishedClusterNames(t *testing.T, pt *ProxyTranslator) []string {
	t.Helper()
	snap, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	require.NoError(t, err)
	names := make([]string, 0)
	for name := range snap.GetResourcesAndTTL(resourcev3.ClusterType) {
		names = append(names, name)
	}
	return names
}

func publishedEndpointNames(t *testing.T, pt *ProxyTranslator) []string {
	t.Helper()
	snap, err := pt.xdsCache.GetSnapshot(publishGateTestClient)
	require.NoError(t, err)
	names := make([]string, 0)
	for name := range snap.GetResourcesAndTTL(resourcev3.EndpointType) {
		names = append(names, name)
	}
	return names
}

func newGraceTestTranslator(t *testing.T, grace time.Duration) *ProxyTranslator {
	pt := NewProxyTranslator(newTestSnapshotCache(t), stubPriorXDS{has: true}, 50*time.Millisecond, true, scopedClustersWithGrace(grace))
	return &pt
}

// TestDereferenceGraceKeepsTheClusterUntilTheWindowCloses is the removal-side
// guarantee. Envoy is delivered CDS before RDS, so dropping a de-referenced
// cluster in the same snapshot as the route that stopped naming it sends the
// removal first and 503s the gap. The cluster must survive the publish that
// de-references it, and disappear on its own afterwards.
func TestDereferenceGraceKeepsTheClusterUntilTheWindowCloses(t *testing.T) {
	pt := newGraceTestTranslator(t, 100*time.Millisecond)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going-away"))
	require.ElementsMatch(t, []string{"kept", "going-away"}, publishedClusterNames(t, pt))

	// The route stops naming going-away, so it leaves the emitted set.
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))
	assert.ElementsMatch(t, []string{"kept", "going-away"}, publishedClusterNames(t, pt),
		"the de-referenced cluster must outlive the route update that dropped it")
	assert.ElementsMatch(t, []string{"kept", "going-away"}, publishedEndpointNames(t, pt),
		"a carried cluster travels with its ClusterLoadAssignment")

	require.Eventually(t, func() bool {
		return len(publishedClusterNames(t, pt)) == 1
	}, 2*time.Second, 5*time.Millisecond,
		"nothing else recomputes this client, so the grace timer must publish the removal itself")
	assert.ElementsMatch(t, []string{"kept"}, publishedClusterNames(t, pt))
	assert.ElementsMatch(t, []string{"kept"}, publishedEndpointNames(t, pt),
		"the pruned cluster's CLA must go with it, or the EDS set outlives its CDS entry")
}

// TestDereferenceGraceIsDisabledByZero: an operator who accepts the removal
// race gets today's immediate behavior, and the gate keeps no state for it.
func TestDereferenceGraceIsDisabledByZero(t *testing.T) {
	pt := newGraceTestTranslator(t, 0)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going-away"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))

	assert.ElementsMatch(t, []string{"kept"}, publishedClusterNames(t, pt))
	assert.Empty(t, pt.gate.dereferenced, "a disabled grace must not allocate per-client state")
}

// TestDereferenceGraceDoesNotOscillateOnAFlappingRoute: a route that flips away
// and back must leave the cluster continuously published. If re-referencing did
// not clear the record, the original window would still expire and prune a
// cluster the configuration currently names.
func TestDereferenceGraceDoesNotOscillateOnAFlappingRoute(t *testing.T) {
	pt := newGraceTestTranslator(t, 60*time.Millisecond)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "flapping"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))
	pt.syncXds(ctx, clusterWrapperV("v3", "kept", "flapping"))

	assert.Empty(t, pt.gate.dereferenced[publishGateTestClient],
		"re-referencing must clear the record, not leave a window running")

	// Well past the original window: the cluster is referenced, so it stays.
	time.Sleep(150 * time.Millisecond)
	assert.ElementsMatch(t, []string{"kept", "flapping"}, publishedClusterNames(t, pt),
		"an expired window from an earlier de-reference must not prune a currently-referenced cluster")
}

// TestDereferenceGraceForgetsExpiredClusters keeps the per-client record from
// growing without bound: once a cluster is actually pruned its entry is dropped,
// so a long-lived proxy does not accumulate one entry per cluster it ever lost.
func TestDereferenceGraceForgetsExpiredClusters(t *testing.T) {
	pt := newGraceTestTranslator(t, 20*time.Millisecond)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going-away"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))

	require.Eventually(t, func() bool {
		return len(publishedClusterNames(t, pt)) == 1
	}, 2*time.Second, 5*time.Millisecond)

	pt.gate.mu.Lock()
	defer pt.gate.mu.Unlock()
	assert.Empty(t, pt.gate.dereferenced[publishGateTestClient],
		"a pruned cluster's record must be dropped, not retained forever")
}

// TestClientDepartedCancelsTheGraceTimer: a timer must never publish to a key
// whose client has gone, which is the same rule the first-publish and
// flip-release bounds already follow.
func TestClientDepartedCancelsTheGraceTimer(t *testing.T) {
	pt := newGraceTestTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "going-away"))
	pt.syncXds(ctx, clusterWrapperV("v2", "kept"))
	require.NotEmpty(t, pt.gate.dereferenced[publishGateTestClient])

	pt.gate.clientDeparted(publishGateTestClient)

	assert.Empty(t, pt.gate.dereferenced, "a departed client leaves no armed timer behind")
}

// TestDereferenceGraceNeverResurrectsAnErroredCluster: carrying forward is
// fail-closed. A cluster whose current translation errored must not be served
// from the published snapshot, or a backend that just failed validation would
// keep serving its pre-error configuration for the length of the window.
func TestDereferenceGraceNeverResurrectsAnErroredCluster(t *testing.T) {
	pt := newGraceTestTranslator(t, time.Hour)
	ctx := context.Background()

	pt.syncXds(ctx, clusterWrapperV("v1", "kept", "now-errored"))

	deferredWrap := clusterWrapperV("v2", "kept")
	deferredWrap.erroredClusters = []string{"now-errored"}
	pt.syncXds(ctx, deferredWrap)

	assert.ElementsMatch(t, []string{"kept"}, publishedClusterNames(t, pt),
		"an errored cluster must not be carried forward by the grace window")
}

// TestGraceDereferencedClustersReportsSoonestExpiry: the timer is armed to the
// earliest deadline, so a second de-reference during an open window cannot
// postpone the first one's removal.
func TestGraceDereferencedClustersReportsSoonestExpiry(t *testing.T) {
	g := newPublishGate(0, false, scopedClustersWithGrace(time.Minute))
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: edsClusterProto("first-out")},
		{Resource: edsClusterProto("second-out")},
	})
	building := envoycache.Resources{Items: map[string]envoycachetypes.ResourceWithTTL{}}

	start := time.Now()
	graced, soonest := g.graceDereferencedClustersLocked(publishGateTestClient, published, building, start)
	require.Len(t, graced, 2)
	require.InDelta(t, time.Minute, soonest, float64(time.Second))

	// 30s later the first entry has half its window left; a fresh third
	// de-reference must not push the deadline out.
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v2", []envoycachetypes.ResourceWithTTL{
		{Resource: edsClusterProto("first-out")},
		{Resource: edsClusterProto("second-out")},
		{Resource: edsClusterProto("third-out")},
	})
	_, soonest = g.graceDereferencedClustersLocked(publishGateTestClient, published, building, start.Add(30*time.Second))
	assert.InDelta(t, 30*time.Second, soonest, float64(time.Second),
		"the timer must fire for the oldest window, not the newest")
}

// TestGraceDereferencedClustersDropsExpiredEntries is the state-machine half of
// the prune: past the window the cluster is no longer graced, which is what lets
// the re-publish actually remove it.
func TestGraceDereferencedClustersDropsExpiredEntries(t *testing.T) {
	g := newPublishGate(0, false, scopedClustersWithGrace(time.Second))
	published := &envoycache.Snapshot{}
	published.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("v1", []envoycachetypes.ResourceWithTTL{
		{Resource: edsClusterProto("going-away")},
	})
	building := envoycache.Resources{Items: map[string]envoycachetypes.ResourceWithTTL{}}

	start := time.Now()
	graced, _ := g.graceDereferencedClustersLocked(publishGateTestClient, published, building, start)
	require.Len(t, graced, 1)

	graced, soonest := g.graceDereferencedClustersLocked(publishGateTestClient, published, building, start.Add(2*time.Second))
	assert.Empty(t, graced, "past the window the cluster stops being published")
	assert.Zero(t, soonest, "with nothing graced there is nothing left to wake up for")
}
