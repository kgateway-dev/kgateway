package proxy_syncer

import (
	"testing"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

func edsSetVersionItems(names ...string) map[string]envoycachetypes.ResourceWithTTL {
	items := make([]envoycachetypes.ResourceWithTTL, 0, len(names))
	for _, name := range names {
		items = append(items, envoycachetypes.ResourceWithTTL{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: name}})
	}
	return envoycache.NewResourcesWithTTL("", items).Items
}

// A set published through a filter is versioned exactly as the same set
// published directly: the version is a function of the published rows, not
// of the path that published them.
func TestEndpointSetVersionIsPathIndependent(t *testing.T) {
	rows := map[string]uint64{"a": 11, "b": 22, "c": 33}

	direct := endpointSetVersion(endpointSetHashes(edsSetVersionItems("a", "c"), map[string]uint64{"a": 11, "c": 33}, nil))
	filtered := endpointSetVersion(endpointSetHashes(edsSetVersionItems("a", "c"), rows, nil))
	require.Equal(t, direct, filtered, "dropping b must version {a, c} as if b had never been built")

	full := endpointSetVersion(endpointSetHashes(edsSetVersionItems("a", "b", "c"), rows, nil))
	require.NotEqual(t, full, filtered, "dropping b must move the version")
	require.Equal(t, full, endpointSetVersion(rows), "the full set keeps the XOR of its row hashes")
}

func TestEndpointSetVersionIsOrderIndependent(t *testing.T) {
	rows := map[string]uint64{"a": 11, "b": 22, "c": 33}
	v1 := endpointSetVersion(map[string]uint64{"a": rows["a"], "b": rows["b"], "c": rows["c"]})
	v2 := endpointSetVersion(map[string]uint64{"c": rows["c"], "a": rows["a"], "b": rows["b"]})
	require.Equal(t, v1, v2)
}

// A row that changes behind a filter still moves the filtered version, and a
// row that recovers its inputs restores the version it had.
func TestEndpointSetVersionFollowsRowHashes(t *testing.T) {
	items := edsSetVersionItems("a", "c")
	before := endpointSetVersion(endpointSetHashes(items, map[string]uint64{"a": 11, "b": 22, "c": 33}, nil))
	changed := endpointSetVersion(endpointSetHashes(items, map[string]uint64{"a": 12, "b": 22, "c": 33}, nil))
	require.NotEqual(t, before, changed, "a changed row behind the filter must move the version")
	restored := endpointSetVersion(endpointSetHashes(items, map[string]uint64{"a": 11, "b": 22, "c": 33}, nil))
	require.Equal(t, before, restored, "restoring the row must restore the version")
}

// Contribution precedence: a built row wins over a carried hash, a carried
// hash wins over the name hash, and anything else is versioned by name.
func TestEndpointSetHashesPrecedence(t *testing.T) {
	items := edsSetVersionItems("built", "carried", "synth")
	rows := map[string]uint64{"built": 1, "unpublished": 9}
	carried := func(name string) (uint64, bool) {
		switch name {
		case "built":
			return 100, true
		case "carried":
			return 200, true
		}
		return 0, false
	}

	got := endpointSetHashes(items, rows, carried)
	require.Equal(t, map[string]uint64{
		"built":   1,
		"carried": 200,
		"synth":   utils.HashString("synth"),
	}, got, "only published assignments contribute, each from its most authoritative source")

	withoutCarry := endpointSetHashes(items, rows, nil)
	require.Equal(t, utils.HashString("carried"), withoutCarry["carried"], "with nothing remembered a carried assignment is versioned by name")
	require.Equal(t, uint64(1), withoutCarry["built"])
}

// A carried assignment restored with the hash it was published under gives
// the composed set the version the directly built set would have.
func TestEndpointSetVersionCarryRestoresPublishedVersion(t *testing.T) {
	publishedRows := map[string]uint64{"old": 11, "other": 22}
	published := endpointSetVersion(endpointSetHashes(edsSetVersionItems("old", "other"), publishedRows, nil))

	// The candidate rebuilt only "other"; "old" is copied from the published
	// snapshot and remembered under its published contribution.
	lastPublished := func(name string) (uint64, bool) {
		h, ok := publishedRows[name]
		return h, ok
	}
	composed := endpointSetVersion(endpointSetHashes(edsSetVersionItems("old", "other"), map[string]uint64{"other": 22}, lastPublished))
	require.Equal(t, published, composed, "carrying old forward unchanged must not move the version")

	otherChanged := endpointSetVersion(endpointSetHashes(edsSetVersionItems("old", "other"), map[string]uint64{"other": 23}, lastPublished))
	require.NotEqual(t, published, otherChanged, "a rebuilt row still moves the composed version")
}

func TestVersionEndpointSetKeepsItems(t *testing.T) {
	res := envoycache.NewResourcesWithTTL("ignored", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a"}},
	})
	hashes := map[string]uint64{"a": 5}
	out := versionEndpointSet(res, hashes)
	require.Equal(t, endpointSetVersion(hashes), out.Version)
	require.Equal(t, res.Items, out.Items)
}

// The gate remembers the contributions of the EDS set it last wrote for a
// client, answers carry-forward lookups from them, and forgets them with the
// client's snapshot.
func TestPublishGateRemembersPublishedEDSHashes(t *testing.T) {
	gate := newPublishGate(0, false, nil, clusterScoping{})
	cache := envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil)
	snap := &envoycache.Snapshot{}
	snap.Resources[envoycachetypes.Endpoint] = envoycache.NewResourcesWithTTL("1", []envoycachetypes.ResourceWithTTL{
		{Resource: &envoyendpointv3.ClusterLoadAssignment{ClusterName: "a"}},
	})

	gate.mu.Lock()
	require.Nil(t, gate.lastPublishedEDSLocked("client"), "nothing is remembered before the first write")
	gate.mu.Unlock()

	require.NoError(t, gate.publish(t.Context(), cache, XdsSnapWrapper{proxyKey: "client", snap: snap, endpointHashes: map[string]uint64{"a": 7}}))
	gate.mu.Lock()
	lookup := gate.lastPublishedEDSLocked("client")
	gate.mu.Unlock()
	require.NotNil(t, lookup)
	h, ok := lookup("a")
	require.True(t, ok)
	require.Equal(t, uint64(7), h)
	_, ok = lookup("b")
	require.False(t, ok)

	require.NoError(t, gate.publish(t.Context(), cache, XdsSnapWrapper{proxyKey: "client", snap: snap, endpointHashes: map[string]uint64{"a": 8}}))
	gate.mu.Lock()
	h, _ = gate.lastPublishedEDSLocked("client")("a")
	gate.mu.Unlock()
	require.Equal(t, uint64(8), h, "a later write replaces the remembered contributions")

	require.True(t, gate.reclaimDeparted(cache, "client", stubConnections{connected: false}))
	gate.mu.Lock()
	require.Nil(t, gate.lastPublishedEDSLocked("client"), "clearing the snapshot forgets its contributions")
	gate.mu.Unlock()
}

func TestPublishGatePreservesHashesWhenSettlingEndpoints(t *testing.T) {
	gate := newPublishGate(0, false, stubPriorXDS{has: true}, clusterScoping{})
	cache := envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil)
	ucc := newTestUcc("client")
	build := func(hashes map[string]uint64) *envoycache.Snapshot {
		snap := &envoycache.Snapshot{}
		snap.Resources[envoycachetypes.Cluster] = envoycache.NewResourcesWithTTL("cds", []envoycachetypes.ResourceWithTTL{
			edsClusterForClient(ucc, "a", 1).Cluster.ResourceWithTTL(),
			edsClusterForClient(ucc, "b", 2).Cluster.ResourceWithTTL(),
		})
		snap.Resources[envoycachetypes.Endpoint] = versionEndpointSet(envoycache.Resources{Items: edsSetVersionItems("a", "b")}, hashes)
		return snap
	}
	priorHashes := map[string]uint64{"a": 11, "b": 22}
	prior := build(priorHashes)
	live := endpointsForClient(ucc, "a", 11).Endpoints.ResourceWithTTL()
	prior.Resources[envoycachetypes.Endpoint].Items["a"] = live
	require.NoError(t, gate.publish(t.Context(), cache, XdsSnapWrapper{proxyKey: "client", snap: prior, endpointHashes: priorHashes}))

	hashes := map[string]uint64{"a": utils.HashString("a"), "b": 23}
	next := build(hashes)
	wrap := XdsSnapWrapper{proxyKey: "client", snap: next, endpointHashes: hashes, synthesizedEndpoints: []string{"a"}}
	require.NoError(t, gate.publish(t.Context(), cache, wrap))
	served, err := cache.GetSnapshot("client")
	require.NoError(t, err)
	require.Equal(t, live, served.GetResourcesAndTTL("type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment")["a"])
	require.Equal(t, map[string]uint64{"a": 11, "b": 23}, gate.publishedEDS["client"])
	require.Equal(t, endpointSetVersion(gate.publishedEDS["client"]), served.(*envoycache.Snapshot).Resources[envoycachetypes.Endpoint].Version)
	require.Equal(t, utils.HashString("a"), hashes["a"], "settling must not mutate the input hashes")

	gate.clientDeparted("client")
	require.Nil(t, gate.publishedEDS["client"], "a new connection must not inherit stale contributions")
	require.NoError(t, gate.publish(t.Context(), cache, wrap))
	served, err = cache.GetSnapshot("client")
	require.NoError(t, err)
	require.Equal(t, map[string]uint64{"b": 23}, gate.publishedEDS["client"], "an omitted placeholder must not contribute to the version")
	require.Equal(t, endpointSetVersion(map[string]uint64{"b": 23}), served.(*envoycache.Snapshot).Resources[envoycachetypes.Endpoint].Version)
}
