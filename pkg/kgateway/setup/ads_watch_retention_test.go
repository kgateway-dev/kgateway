package setup

import (
	"context"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/stream/v3"
	"github.com/stretchr/testify/require"
)

// These tests characterize the go-control-plane snapshot cache's handling of a
// named ADS watch it cannot answer, which is the mechanism behind proxies
// stranded on stale endpoints. In ADS mode the cache answers a named request
// (EDS, RDS, SDS) only when the snapshot holds no resource of that type outside
// the request's names, and what it does with the unanswerable watch decides
// whether the proxy can ever be updated again without reconnecting.
//
// They are deliberately written against the dependency rather than our code:
// they say what this pinned go-control-plane does, so a bump that changes it
// fails here instead of in production.

const retentionNode = "watch-retention-test-node"

type retentionHash struct{}

func (retentionHash) ID(*envoycorev3.Node) string { return retentionNode }

func edsOnlySnapshot(t *testing.T, version string, clusters ...string) *envoycache.Snapshot {
	t.Helper()

	resources := make([]cachetypes.Resource, 0, len(clusters))
	for _, name := range clusters {
		resources = append(resources, &envoyendpointv3.ClusterLoadAssignment{ClusterName: name})
	}
	snapshot, err := envoycache.NewSnapshot(version, map[envoyresource.Type][]cachetypes.Resource{
		envoyresource.EndpointType: resources,
	})
	require.NoError(t, err)
	return snapshot
}

func namedEDSRequest(version string, names ...string) *envoycache.Request {
	return &discoveryv3.DiscoveryRequest{
		TypeUrl:       envoyresource.EndpointType,
		ResourceNames: names,
		VersionInfo:   version,
	}
}

// newSotwSubscription returns the subscription by pointer, because
// stream.Subscription is a struct whose mutators take a pointer receiver: the
// sotw server keeps one per stream and type and hands the cache a copy on each
// request.
func newSotwSubscription(names ...string) *stream.Subscription {
	sub := stream.NewSotwSubscription(names, false)
	return &sub
}

func ackSubscription(t *testing.T, resp envoycache.Response, sub *stream.Subscription, req *envoycache.Request) {
	t.Helper()

	sub.SetReturnedResources(resp.GetReturnedResources())
	req.VersionInfo = resp.GetResponseVersion()
}

func awaitResponse(t *testing.T, ch chan envoycache.Response, what string) envoycache.Response {
	t.Helper()

	select {
	case resp := <-ch:
		return resp
	case <-time.After(2 * time.Second):
		t.Fatalf("expected a response: %s", what)
		return nil
	}
}

func requireNoResponse(t *testing.T, ch chan envoycache.Response, what string) {
	t.Helper()

	select {
	case <-ch:
		t.Fatalf("expected no response: %s", what)
	case <-time.After(200 * time.Millisecond):
	}
}

// A parked named watch that a new snapshot cannot answer must survive that
// snapshot, so a later snapshot the watch CAN accept is delivered.
//
// This is the behavior kgateway depends on to keep a proxy's endpoints fresh
// while some other cluster of the same proxy is in an unusable state (a
// rejected cluster, or one the proxy has not applied yet): the endpoint update
// is withheld while the snapshot is misaligned, then delivered once it is not.
// Older go-control-plane discarded the watch instead, leaving the proxy with no
// watch for anything to answer — endpoints frozen until it reconnected, even
// after the control plane had corrected the snapshot.
func TestADSCacheRetainsNamedWatchItCannotAnswer(t *testing.T) {
	ctx := context.Background()
	cache := envoycache.NewSnapshotCache(true, retentionHash{}, nil)
	require.NoError(t, cache.SetSnapshot(ctx, retentionNode, edsOnlySnapshot(t, "1", "backend")))

	req := namedEDSRequest("", "backend")
	sub := newSotwSubscription("backend")
	initial := make(chan envoycache.Response, 1)
	_, err := cache.CreateWatch(req, *sub, initial)
	require.NoError(t, err)
	ackSubscription(t, awaitResponse(t, initial, "initial endpoints"), sub, req)

	// The proxy re-requests at the version it accepted: the watch parks.
	parked := make(chan envoycache.Response, 1)
	cancel, err := cache.CreateWatch(req, *sub, parked)
	require.NoError(t, err)
	defer cancel()
	require.Equal(t, 1, cache.GetStatusInfo(retentionNode).GetNumWatches())

	// A snapshot that also carries endpoints for a cluster this proxy has not
	// requested cannot be sent to this watch.
	require.NoError(t, cache.SetSnapshot(ctx, retentionNode, edsOnlySnapshot(t, "2", "backend", "backend-v2")))
	requireNoResponse(t, parked, "snapshot names a resource the watch did not request")
	require.Equal(t, 1, cache.GetStatusInfo(retentionNode).GetNumWatches(),
		"the unanswerable watch must be retained, or the proxy can never be updated without reconnecting")

	// Once the snapshot is answerable again, the retained watch receives it.
	require.NoError(t, cache.SetSnapshot(ctx, retentionNode, edsOnlySnapshot(t, "3", "backend")))
	resp := awaitResponse(t, parked, "corrected snapshot delivered to the retained watch")
	version := resp.GetResponseVersion()
	require.Equal(t, "3", version)
}

// A NEW named request the current snapshot cannot answer is still dropped
// without registering a watch, so a later answerable snapshot has nothing to
// deliver to. Upstream marks this path with a TODO ("likely unneeded now, to be
// cleaned"); until it is removed, a proxy that (re)requests endpoints while the
// snapshot is misaligned — the window around a rejected or not-yet-applied
// cluster update — can be left waiting on a request that is never answered.
//
// This test documents the residual gap. If it starts failing, upstream has
// closed it and any local mitigation can be dropped.
func TestADSCacheDropsNewNamedRequestItCannotAnswer(t *testing.T) {
	ctx := context.Background()
	cache := envoycache.NewSnapshotCache(true, retentionHash{}, nil)
	require.NoError(t, cache.SetSnapshot(ctx, retentionNode, edsOnlySnapshot(t, "2", "backend", "backend-v2")))

	dropped := make(chan envoycache.Response, 1)
	cancel, err := cache.CreateWatch(namedEDSRequest("1", "backend"), *newSotwSubscription("backend"), dropped)
	require.NoError(t, err)
	defer cancel()

	requireNoResponse(t, dropped, "snapshot names a resource the request did not ask for")
	require.Equal(t, 0, cache.GetStatusInfo(retentionNode).GetNumWatches(),
		"the request is dropped without a watch (residual gap; see the test doc)")

	require.NoError(t, cache.SetSnapshot(ctx, retentionNode, edsOnlySnapshot(t, "3", "backend")))
	requireNoResponse(t, dropped, "nothing to answer: the request left no watch behind")
}
