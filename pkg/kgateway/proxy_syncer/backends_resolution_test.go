package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// A delta set's stored client snapshot is what lets FetchClustersForClient read
// a sparse absence as "no overlay applies" rather than "not evaluated yet". If
// Equals accepted a stale snapshot, KRT would keep the old row, the requesting
// client would never appear in it, and its CDS would be withheld with no event
// left to recover it. ClientsFingerprint alone cannot carry that weight — it is
// a 64-bit hash — so Equals compares the interner's generation, which is only
// advanced after a full membership check.
func TestBackendClusterDeltaSetEqualsRejectsStaleSnapshotUnderFingerprintCollision(t *testing.T) {
	clientA := ir.NewUniquelyConnectedClient("a", "ns", map[string]string{"id": "a"}, ir.PodLocality{})
	clientB := ir.NewUniquelyConnectedClient("b", "ns", map[string]string{"id": "b"}, ir.PodLocality{})

	interner := &clientInputSnapshotInterner{}
	before := interner.intern([]ir.UniquelyConnectedClient{clientA})
	after := interner.intern([]ir.UniquelyConnectedClient{clientA, clientB})
	require.NotEqual(t, before.generation, after.generation,
		"a genuinely different client set must get its own generation")

	setFor := func(snapshot clientInputSnapshot, fingerprint clientSetFingerprint) backendClusterDeltaSet {
		return backendClusterDeltaSet{
			Name:               "cluster",
			ClientsFingerprint: fingerprint,
			ResolvedClients:    snapshot,
		}
	}

	// Force the collision the fingerprint cannot rule out: two different client
	// sets presenting the same fingerprint. Without the generation check these
	// rows would compare equal and the stale snapshot would be retained.
	stale := setFor(before, 1)
	fresh := setFor(after, 1)
	assert.False(t, stale.Equals(fresh),
		"a delta set whose resolved snapshot moved must not compare equal, even on a fingerprint collision")
	assert.False(t, stale.ResolvedClients.ContainsCurrent(clientB),
		"the stale snapshot is exactly the one that would withhold clientB forever")

	// Re-interning an unchanged set must still compare equal, or every backend
	// would republish on every recompute.
	same := interner.intern([]ir.UniquelyConnectedClient{clientA, clientB})
	assert.Equal(t, after.generation, same.generation, "an unchanged client set must reuse its generation")
	assert.True(t, setFor(after, 1).Equals(setFor(same, 1)))
}

// The delta transform finds its base by the backend's memoized ClusterName, and
// FetchClustersForClient withholds a client's whole CDS for any base that has no
// matching delta set. A cluster renamed during translation would therefore
// blackhole every connected client permanently. Nothing in tree renames it, so
// this pins the containment: the renamed backend alone is dropped, and the
// clients keep serving every other backend.
func TestNewPerClientEnvoyClusters_RenamedClusterDropsOnlyThatBackend(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	backendGK := schema.GroupKind{Group: "group", Kind: "kind"}

	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			backendGK: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					if in.GetName() == "renamed" {
						out.Name = "something-else"
					}
					return nil
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{},
	}

	good := ir.NewBackendObjectIR(ir.ObjectSource{Group: "group", Kind: "kind", Namespace: "ns", Name: "good"}, 80, "", "")
	renamed := ir.NewBackendObjectIR(ir.ObjectSource{Group: "group", Kind: "kind", Namespace: "ns", Name: "renamed"}, 80, "", "")
	finalBackends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{&good, &renamed},
		krtopts.ToOptions("FinalBackends")...)
	client := ir.NewUniquelyConnectedClient("c", "ns", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{client}, krtopts.ToOptions("UCCs")...)

	pcc := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)
	require.Eventually(t, pcc.HasSynced, time.Second, 10*time.Millisecond)

	var got []uccWithCluster
	require.Eventually(t, func() bool {
		got = pcc.FetchClustersForClient(krt.TestingDummyContext{}, client)
		return len(got) > 0
	}, 2*time.Second, 20*time.Millisecond,
		"a renamed cluster must not withhold the client's entire CDS")

	require.Len(t, got, 1, "only the renamed backend should be dropped")
	assert.Equal(t, good.ClusterName(), got[0].Name)
}
