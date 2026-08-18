package proxy_syncer

import (
	"errors"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func clusterNamed(name string) *envoyclusterv3.Cluster {
	return &envoyclusterv3.Cluster{Name: name}
}

func uccWithClusterByName(got []uccWithCluster) map[string]uccWithCluster {
	m := make(map[string]uccWithCluster, len(got))
	for _, c := range got {
		m[c.Name] = c
	}
	return m
}

func waitSynced(t *testing.T, pcc PerClientEnvoyClusters) {
	t.Helper()
	require.Eventually(t, pcc.HasSynced, time.Second, 10*time.Millisecond)
}

// TestFetchClustersForClient_Merge exercises the base/delta merge: a base with
// no delta passes through unchanged, while a resolved delta overlays the base
// of the same name (winning on cluster + version).
func TestFetchClustersForClient_Merge(t *testing.T) {
	ucc := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{"k": "a"}, ir.PodLocality{})

	baseOnly := clusterNamed("base-only")
	overlaidBase := clusterNamed("overlaid")
	overlaidDelta := clusterNamed("overlaid")

	bases := []baseEnvoyCluster{
		{Name: "base-only", Cluster: sharedproto.Wrap(baseOnly), ClusterVersion: 1},
		{Name: "overlaid", Cluster: sharedproto.Wrap(overlaidBase), ClusterVersion: 2},
	}
	deltas := []uccClusterDelta{
		{Client: ucc, Name: "overlaid", Cluster: sharedproto.Wrap(overlaidDelta), ClusterVersion: 99},
	}

	pcc := newTestPerClientClustersRaw(bases, deltas, ucc)
	waitSynced(t, pcc)

	got := uccWithClusterByName(pcc.FetchClustersForClient(krt.TestingDummyContext{}, ucc))
	require.Len(t, got, 2)

	// base with no delta passes through unchanged
	require.True(t, got["base-only"].Cluster.Is(baseOnly), "base with no delta must alias the base proto")
	require.Equal(t, uint64(1), got["base-only"].ClusterVersion)

	// delta overlays the base of the same name, winning on cluster + version
	require.True(t, got["overlaid"].Cluster.Is(overlaidDelta), "delta must win over the base of the same name")
	require.Equal(t, uint64(99), got["overlaid"].ClusterVersion)
}

// TestFetchClustersForClient_DeltaErrorWinsOverBaseError documents the error
// precedence in the merge: a per-UCC delta error is the more specific signal and
// takes precedence over a base error of the same name.
func TestFetchClustersForClient_DeltaErrorWinsOverBaseError(t *testing.T) {
	ucc := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{"k": "a"}, ir.PodLocality{})
	baseErr := errors.New("base boom")
	deltaErr := errors.New("delta boom")

	bases := []baseEnvoyCluster{{Name: "c", Cluster: sharedproto.Wrap(clusterNamed("c")), ClusterVersion: 1, Error: baseErr}}
	deltas := []uccClusterDelta{{Client: ucc, Name: "c", Cluster: sharedproto.Wrap(clusterNamed("c")), ClusterVersion: 2, Error: deltaErr}}

	pcc := newTestPerClientClustersRaw(bases, deltas, ucc)
	waitSynced(t, pcc)

	got := pcc.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
	require.Len(t, got, 1)
	require.Equal(t, deltaErr, got[0].Error, "delta error should win over base error")
}

// TestFetchClustersForClient_FiltersByClient confirms a delta is scoped to its
// own UCC: the owning client sees the override while another client falls back
// to the shared base.
func TestFetchClustersForClient_FiltersByClient(t *testing.T) {
	uccA := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{"k": "a"}, ir.PodLocality{})
	uccB := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{"k": "b"}, ir.PodLocality{})

	base := clusterNamed("c")
	bases := []baseEnvoyCluster{{Name: "c", Cluster: sharedproto.Wrap(base), ClusterVersion: 1}}
	deltas := []uccClusterDelta{{Client: uccA, Name: "c", Cluster: sharedproto.Wrap(clusterNamed("c-a")), ClusterVersion: 50}}

	pcc := newTestPerClientClustersRaw(bases, deltas, uccA, uccB)
	waitSynced(t, pcc)

	// uccA sees its delta override
	gotA := pcc.FetchClustersForClient(krt.TestingDummyContext{}, uccA)
	require.Len(t, gotA, 1)
	require.Equal(t, uint64(50), gotA[0].ClusterVersion)

	// uccB has no delta, sees the shared base proto
	gotB := pcc.FetchClustersForClient(krt.TestingDummyContext{}, uccB)
	require.Len(t, gotB, 1)
	require.True(t, gotB[0].Cluster.Is(base), "client without a delta must alias the shared base proto")
	require.Equal(t, uint64(1), gotB[0].ClusterVersion)
}

// TestFetchClustersForClient_MatchingDeltaDoesNotRequireResolutionProof pins
// the sparse-read ordering: a materialized delta already carries the exact UCC
// it was computed for, so client-set resolution is needed only when falling
// back to the shared base. This models reading an older set while unrelated
// fleet membership is converging.
func TestFetchClustersForClient_MatchingDeltaDoesNotRequireResolutionProof(t *testing.T) {
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	base := clusterNamed("c")
	delta := clusterNamed("c")
	fingerprint := baseClusterFingerprint{ClusterVersion: 1}
	emptySnapshot := newClientInputSnapshot(nil)

	baseCol := krt.NewStaticCollection(nil, []baseEnvoyCluster{{
		Name: "c", Cluster: sharedproto.Wrap(base), ClusterVersion: 1, Fingerprint: fingerprint,
	}})
	deltaCol := krt.NewStaticCollection(nil, []backendClusterDeltaSet{{
		Name:               "c",
		BaseFingerprint:    fingerprint,
		ClientsFingerprint: emptySnapshot.Fingerprint,
		ResolvedClients:    emptySnapshot,
		Deltas: map[string]uccClusterDelta{
			ucc.ResourceName(): {
				Client:         ucc,
				Name:           "c",
				Cluster:        sharedproto.Wrap(delta),
				ClusterVersion: 2,
			},
		},
	}})
	clientCol := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc})
	pcc := PerClientEnvoyClusters{base: baseCol, deltas: deltaCol, clients: clientCol}
	waitSynced(t, pcc)

	got := pcc.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
	require.Len(t, got, 1)
	require.True(t, got[0].Cluster.Is(delta), "the exact materialized delta must win without a fleet-resolution match")
}

// TestFetchClustersForClient_WithholdsInlineCLABaseUntilDeltaArrives pins the
// publish-atomicity guard: a base whose CLA is built per client (nil
// LoadAssignment on an inline-CLA cluster type) must NOT be surfaced for a UCC
// that has no delta yet — base and deltas are separate KRT collections, so the
// base can be visible first. Publishing it would send Envoy a host-less
// STRICT_DNS/STATIC cluster (503s until the delta lands); withholding lets the
// snapshot's referenced-cluster deferral hold the publish, matching the
// pre-split behavior where the row was absent until fully translated.
func TestFetchClustersForClient_WithholdsInlineCLABaseUntilDeltaArrives(t *testing.T) {
	ucc := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{"k": "a"}, ir.PodLocality{})

	inlineBase := &envoyclusterv3.Cluster{
		Name:                 "inline",
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS},
	}
	us := ir.NewBackendObjectIR(ir.ObjectSource{Namespace: "ns", Name: "svc"}, 0, "", "")
	bases := []baseEnvoyCluster{{
		Name:           "inline",
		Cluster:        sharedproto.Wrap(inlineBase),
		ClusterVersion: 1,
		NeedsInlineCLA: true,
		Base: &irtranslator.BaseCluster{
			Cluster:           inlineBase,
			EndpointInputs:    &endpoints.EndpointsInputs{EndpointsForBackend: *ir.NewEndpointsForBackend(us)},
			SupportsInlineCLA: true,
		},
	}}

	// No delta yet: the incomplete base must be withheld entirely.
	pcc := newTestPerClientClustersRaw(bases, nil, ucc)
	waitSynced(t, pcc)
	got := pcc.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
	require.Empty(t, got, "a CLA-less inline-CLA base must be withheld until its per-client delta arrives")

	// Delta present: the merged per-client cluster is surfaced.
	withCLA := clusterNamed("inline")
	pcc = newTestPerClientClustersRaw(bases, []uccClusterDelta{
		{Client: ucc, Name: "inline", Cluster: sharedproto.Wrap(withCLA), ClusterVersion: 7},
	}, ucc)
	waitSynced(t, pcc)
	got = pcc.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
	require.Len(t, got, 1)
	require.True(t, got[0].Cluster.Is(withCLA), "the per-client delta must be surfaced once it arrives")
	require.Equal(t, uint64(7), got[0].ClusterVersion)
}

// TestFetchClustersForClient_RejectsStaleDeltaAfterBaseUpdate reproduces the
// reviewer-identified ordering window: the new base is visible while the old
// full-clone delta still exists. The stale delta must not override the new base;
// the whole merged view stays pending until a matching delta set arrives.
func TestFetchClustersForClient_RejectsStaleDeltaAfterBaseUpdate(t *testing.T) {
	ucc := ir.NewUniquelyConnectedClient("role", "ns", map[string]string{"k": "a"}, ir.PodLocality{})
	oldFingerprint := baseClusterFingerprint{ClusterVersion: 1}
	newFingerprint := baseClusterFingerprint{ClusterVersion: 2}
	newBase := clusterNamed("c")
	staleDelta := clusterNamed("c")
	clientSnapshot := newClientInputSnapshot([]ir.UniquelyConnectedClient{ucc})

	baseCol := krt.NewStaticCollection(nil, []baseEnvoyCluster{{
		Name:           "c",
		Cluster:        sharedproto.Wrap(newBase),
		ClusterVersion: 2,
		Fingerprint:    newFingerprint,
	}})
	deltaCol := krt.NewStaticCollection(nil, []backendClusterDeltaSet{{
		Name:               "c",
		BaseFingerprint:    oldFingerprint,
		ClientsFingerprint: clientSnapshot.Fingerprint,
		ResolvedClients:    clientSnapshot,
		Deltas: map[string]uccClusterDelta{
			ucc.ResourceName(): {
				Client:         ucc,
				Name:           "c",
				Cluster:        sharedproto.Wrap(staleDelta),
				ClusterVersion: 99,
			},
		},
	}})
	clientCol := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc})
	pcc := PerClientEnvoyClusters{base: baseCol, deltas: deltaCol, clients: clientCol}
	waitSynced(t, pcc)

	require.Empty(t, pcc.FetchClustersForClient(krt.TestingDummyContext{}, ucc),
		"a stale delta must make the generation pending, not override the newer base")

	// Overlay removal: a matching empty set explicitly resolves to the base.
	deltaCol.UpdateObject(backendClusterDeltaSet{
		Name:               "c",
		BaseFingerprint:    newFingerprint,
		ClientsFingerprint: clientSnapshot.Fingerprint,
		ResolvedClients:    clientSnapshot,
	})
	require.Eventually(t, func() bool {
		got := pcc.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
		return len(got) == 1 && got[0].Cluster.Is(newBase)
	}, time.Second, 10*time.Millisecond)

	// Overlay addition: the matching delta atomically replaces the base.
	newDelta := clusterNamed("c")
	deltaCol.UpdateObject(backendClusterDeltaSet{
		Name:               "c",
		BaseFingerprint:    newFingerprint,
		ClientsFingerprint: clientSnapshot.Fingerprint,
		ResolvedClients:    clientSnapshot,
		Deltas: map[string]uccClusterDelta{
			ucc.ResourceName(): {
				Client:         ucc,
				Name:           "c",
				Cluster:        sharedproto.Wrap(newDelta),
				ClusterVersion: 100,
			},
		},
	})
	require.Eventually(t, func() bool {
		got := pcc.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
		return len(got) == 1 && got[0].Cluster.Is(newDelta)
	}, time.Second, 10*time.Millisecond)
}

// TestFetchClustersForClient_WaitsForCurrentClientSet proves that an empty
// sparse result from before a client connected is pending, not an affirmative
// "no overlay" decision for that client.
func TestFetchClustersForClient_WaitsForCurrentClientSet(t *testing.T) {
	ucc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	base := clusterNamed("c")
	fingerprint := baseClusterFingerprint{ClusterVersion: 1}
	baseCol := krt.NewStaticCollection(nil, []baseEnvoyCluster{{
		Name: "c", Cluster: sharedproto.Wrap(base), ClusterVersion: 1, Fingerprint: fingerprint,
	}})
	emptySnapshot := newClientInputSnapshot(nil)
	deltaCol := krt.NewStaticCollection(nil, []backendClusterDeltaSet{{
		Name:               "c",
		BaseFingerprint:    fingerprint,
		ClientsFingerprint: emptySnapshot.Fingerprint,
		ResolvedClients:    emptySnapshot,
	}})
	clientCol := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc})
	pcc := PerClientEnvoyClusters{base: baseCol, deltas: deltaCol, clients: clientCol}
	waitSynced(t, pcc)

	require.Empty(t, pcc.FetchClustersForClient(krt.TestingDummyContext{}, ucc))
	clientSnapshot := newClientInputSnapshot([]ir.UniquelyConnectedClient{ucc})
	deltaCol.UpdateObject(backendClusterDeltaSet{
		Name:               "c",
		BaseFingerprint:    fingerprint,
		ClientsFingerprint: clientSnapshot.Fingerprint,
		ResolvedClients:    clientSnapshot,
	})
	require.Eventually(t, func() bool {
		got := pcc.FetchClustersForClient(krt.TestingDummyContext{}, ucc)
		return len(got) == 1 && got[0].Cluster.Is(base)
	}, time.Second, 10*time.Millisecond)
}

// TestFetchClustersForClient_UnrelatedClientChurnIsNotAReadinessBarrier proves
// that an established client can keep using a backend result evaluated before
// another client connected. The old snapshot is authoritative for the stable
// client because it contains that client's exact current inputs; only the new
// client must wait for this backend to evaluate it.
func TestFetchClustersForClient_UnrelatedClientChurnIsNotAReadinessBarrier(t *testing.T) {
	stable := ir.NewUniquelyConnectedClient("stable", "ns", nil, ir.PodLocality{})
	late := ir.NewUniquelyConnectedClient("late", "ns", nil, ir.PodLocality{})
	base := clusterNamed("c")
	fingerprint := baseClusterFingerprint{ClusterVersion: 1}
	stableSnapshot := newClientInputSnapshot([]ir.UniquelyConnectedClient{stable})

	baseCol := krt.NewStaticCollection(nil, []baseEnvoyCluster{{
		Name: "c", Cluster: sharedproto.Wrap(base), ClusterVersion: 1, Fingerprint: fingerprint,
	}})
	deltaCol := krt.NewStaticCollection(nil, []backendClusterDeltaSet{{
		Name:               "c",
		BaseFingerprint:    fingerprint,
		ClientsFingerprint: stableSnapshot.Fingerprint,
		ResolvedClients:    stableSnapshot,
	}})
	clientCol := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{stable, late})
	pcc := PerClientEnvoyClusters{base: baseCol, deltas: deltaCol, clients: clientCol}
	waitSynced(t, pcc)

	stableClusters := pcc.FetchClustersForClient(krt.TestingDummyContext{}, stable)
	require.Len(t, stableClusters, 1, "unrelated client addition must not defer the established client")
	require.True(t, stableClusters[0].Cluster.Is(base))
	require.Empty(t, pcc.FetchClustersForClient(krt.TestingDummyContext{}, late),
		"the newly connected client must wait until this backend evaluates it")
}

// TestFetchClustersForClient_WaitsForCurrentLocalClusterCapability proves that
// an empty sparse result from before a client advertised local-cluster support
// is pending, not an affirmative "no overlay" decision for the changed client.
func TestFetchClustersForClient_WaitsForCurrentLocalClusterCapability(t *testing.T) {
	oldUcc := ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{})
	currentUcc := oldUcc
	currentUcc.KnowsLocalCluster = true
	stable := ir.NewUniquelyConnectedClient("stable", "ns", nil, ir.PodLocality{})

	base := clusterNamed("c")
	fingerprint := baseClusterFingerprint{ClusterVersion: 1}
	baseCol := krt.NewStaticCollection(nil, []baseEnvoyCluster{{
		Name: "c", Cluster: sharedproto.Wrap(base), ClusterVersion: 1, Fingerprint: fingerprint,
	}})
	oldSnapshot := newClientInputSnapshot([]ir.UniquelyConnectedClient{oldUcc, stable})
	deltaCol := krt.NewStaticCollection(nil, []backendClusterDeltaSet{{
		Name:               "c",
		BaseFingerprint:    fingerprint,
		ClientsFingerprint: oldSnapshot.Fingerprint,
		ResolvedClients:    oldSnapshot,
	}})
	clientCol := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{currentUcc, stable})
	pcc := PerClientEnvoyClusters{base: baseCol, deltas: deltaCol, clients: clientCol}
	waitSynced(t, pcc)

	require.Empty(t, pcc.FetchClustersForClient(krt.TestingDummyContext{}, currentUcc))
	stableClusters := pcc.FetchClustersForClient(krt.TestingDummyContext{}, stable)
	require.Len(t, stableClusters, 1, "another client's capability transition must not defer the stable client")
	require.True(t, stableClusters[0].Cluster.Is(base))

	currentSnapshot := newClientInputSnapshot([]ir.UniquelyConnectedClient{currentUcc, stable})
	deltaCol.UpdateObject(backendClusterDeltaSet{
		Name:               "c",
		BaseFingerprint:    fingerprint,
		ClientsFingerprint: currentSnapshot.Fingerprint,
		ResolvedClients:    currentSnapshot,
	})
	require.Eventually(t, func() bool {
		got := pcc.FetchClustersForClient(krt.TestingDummyContext{}, currentUcc)
		return len(got) == 1 && got[0].Cluster.Is(base)
	}, time.Second, 10*time.Millisecond)
}
