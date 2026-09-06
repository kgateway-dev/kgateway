package proxy_syncer

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// TestNewPerClientEnvoyClusters_SparseOverlayWiring exercises the real KRT
// wiring end-to-end (base collection -> per-client assembly) rather than the
// static-collection test helpers. It pins the headline behaviors of the
// base+overlay split:
//
//   - A UCC the overlay declines sees the shared base proto (nothing allocated).
//   - A UCC the overlay matches sees a distinct per-client proto carrying the
//     mutation, while the base proto stays pristine.
//   - Two matching UCCs receive independently owned protos; interning is
//     intentionally left to a later optimization.
func TestNewPerClientEnvoyClusters_SparseOverlayWiring(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	backendGK := schema.GroupKind{Group: "group", Kind: "kind"}
	overlayGK := schema.GroupKind{Group: "test", Kind: "Overlay"}

	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			backendGK: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			overlayGK: {
				// Self-gating overlay: only clients labeled match=yes get a
				// mutation; everyone else takes the fast path (nil => share base).
				PerClientClusterOverlay: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
					if ucc.Labels["match"] != "yes" {
						return nil
					}
					return &sdk.ClusterOverlay{
						Mutate: func(out *envoyclusterv3.Cluster) {
							out.OutlierDetection = &envoyclusterv3.OutlierDetection{}
						},
					}
				},
			},
		},
	}

	backend := ir.NewBackendObjectIR(ir.ObjectSource{Group: "group", Kind: "kind", Namespace: "ns", Name: "svc"}, 80, "", "")
	backend.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{}}
	finalBackends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{&backend}, krtopts.ToOptions("FinalBackends")...)

	// matchA and matchB produce byte-identical overlaid clusters; other is declined.
	matchA := ir.NewUniquelyConnectedClient("a", "ns", map[string]string{"match": "yes", "id": "a"}, ir.PodLocality{})
	matchB := ir.NewUniquelyConnectedClient("b", "ns", map[string]string{"match": "yes", "id": "b"}, ir.PodLocality{})
	other := ir.NewUniquelyConnectedClient("c", "ns", map[string]string{"match": "no"}, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{matchA, matchB, other}, krtopts.ToOptions("UCCs")...)

	pcc := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)
	require.Eventually(t, pcc.HasSynced, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		bases := krt.Fetch(krt.TestingDummyContext{}, pcc.base)
		return len(bases) == 1 && bases[0].Base != nil && bases[0].Base.Cluster == nil
	}, time.Second, 10*time.Millisecond,
		"the retained BaseCluster must not expose a raw alias to the shared proto")

	name := backend.ClusterName()
	var gotA, gotB, gotOther *envoyclusterv3.Cluster
	require.Eventually(t, func() bool {
		gotA = storedClustersForClient(pcc, matchA)[name]
		gotB = storedClustersForClient(pcc, matchB)[name]
		gotOther = storedClustersForClient(pcc, other)[name]
		return gotA != nil && gotB != nil && gotOther != nil
	}, 2*time.Second, 20*time.Millisecond)

	// Declined client: the shared base proto itself, no mutation, no copy.
	assert.Nil(t, gotOther.GetOutlierDetection(), "declined client must see the un-overlaid base")
	baseRow := pcc.base.GetKey(name)
	require.NotNil(t, baseRow)
	assert.True(t, baseRow.Cluster.Is(gotOther), "declined client must be served the base proto, not a copy")

	// Matched client: distinct proto carrying the overlay mutation.
	assert.NotNil(t, gotA.GetOutlierDetection(), "matched client must see the overlay mutation")
	assert.NotSame(t, gotOther, gotA, "matched client must not share the base proto")
	assert.NotSame(t, gotA, gotB, "clones are owned by their client; interning is a separate optimization")

	// The per-client transform lends the base proto to ApplyPerClient rather
	// than handing it a defensive copy, so the overlay passes above ran against
	// the very proto the declined client is served. That client's payload was
	// published through the snapshot sink, which re-verifies the wrap-time hash
	// (TestMain arms the tripwire); reaching this line means nothing on that
	// path mutated the borrowed base.
	require.NotPanics(t, func() { baseRow.Cluster.ResourceWithTTL() },
		"the shared base must survive the per-client overlay passes unmutated")
}

// TestNewPerClientEnvoyClusters_BackendMetadataUpdateRecomputesClients covers
// the waypoint ingress-use-waypoint failure mode: a metadata-only Service label
// update changes whether a per-client overlay applies, even though the shared
// base cluster is byte-identical. The base row compares its Backend through
// BackendObjectIR.Equals, which sees the object's version, so the row changes
// and every client's payload is rebuilt from the updated backend.
func TestNewPerClientEnvoyClusters_BackendMetadataUpdateRecomputesClients(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	backendGK := schema.GroupKind{Group: "", Kind: "Service"}
	overlayGK := schema.GroupKind{Group: "test", Kind: "Overlay"}
	const overlayLabel = "test-overlay"

	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			backendGK: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			overlayGK: {
				PerClientClusterOverlay: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
					if in.Obj.GetLabels()[overlayLabel] != "true" {
						return nil
					}
					return &sdk.ClusterOverlay{
						Mutate: func(out *envoyclusterv3.Cluster) {
							out.OutlierDetection = &envoyclusterv3.OutlierDetection{}
						},
					}
				},
			},
		},
	}

	backend := ir.NewBackendObjectIR(ir.ObjectSource{Group: "", Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")
	backend.Obj = &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace:       "ns",
		Name:            "svc",
		UID:             "svc-uid",
		ResourceVersion: "1",
		Generation:      1,
	}}
	finalBackends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{&backend}, krtopts.ToOptions("FinalBackends")...)
	ucc := ir.NewUniquelyConnectedClient("client", "ns", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc}, krtopts.ToOptions("UCCs")...)

	pcc := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)
	require.Eventually(t, pcc.HasSynced, time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		stored := storedClustersForClient(pcc, ucc)
		cluster := stored[backend.ClusterName()]
		return len(stored) == 1 && cluster != nil && cluster.GetOutlierDetection() == nil
	}, 2*time.Second, 20*time.Millisecond)

	updated := backend
	updated.Obj = &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace:       "ns",
		Name:            "svc",
		UID:             "svc-uid",
		ResourceVersion: "2",
		Generation:      1,
		Labels:          map[string]string{overlayLabel: "true"},
	}}
	finalBackends.UpdateObject(&updated)

	require.Eventually(t, func() bool {
		stored := storedClustersForClient(pcc, ucc)
		cluster := stored[backend.ClusterName()]
		return len(stored) == 1 && cluster != nil && cluster.GetOutlierDetection() != nil
	}, 2*time.Second, 20*time.Millisecond)

	removed := backend
	removed.Obj = &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace:       "ns",
		Name:            "svc",
		UID:             "svc-uid",
		ResourceVersion: "3",
		Generation:      1,
	}}
	finalBackends.UpdateObject(&removed)

	require.Eventually(t, func() bool {
		stored := storedClustersForClient(pcc, ucc)
		cluster := stored[backend.ClusterName()]
		return len(stored) == 1 && cluster != nil && cluster.GetOutlierDetection() == nil
	}, 2*time.Second, 20*time.Millisecond)
}

// TestNewPerClientEnvoyClusters_ArmedTripwireCatchesBaseMutation is the negative
// control for the immutability tripwire on a real collection row. TestMain arms
// sharedproto.AssertImmutability for this package, so the base transform captures
// each shared proto's content hash at wrap time; mutating the shared base through
// a borrowed pointer — the aliasing a buggy overlay or snapshot consumer would
// introduce — must make the publish path panic and name the cluster. Without this
// check a change that quietly stopped capturing hashes (wrapping with hash 0, or
// wrapping before the proto is final) would leave every NotPanics assertion in the
// package passing while the tripwire guarded nothing.
func TestNewPerClientEnvoyClusters_ArmedTripwireCatchesBaseMutation(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	require.True(t, sharedproto.AssertImmutability, "TestMain must arm the tripwire for this package")

	backendGK := schema.GroupKind{Group: "group", Kind: "kind"}
	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			backendGK: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{},
	}
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Group: "group", Kind: "kind", Namespace: "ns", Name: "svc"}, 80, "", "")
	finalBackends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{&backend}, krtopts.ToOptions("FinalBackends")...)
	ucc := ir.NewUniquelyConnectedClient("c", "ns", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc}, krtopts.ToOptions("UCCs")...)

	pcc := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)
	// Wait for the client's row to be stored, so no transform is publishing the
	// shared base (and re-hashing it) concurrently with the deliberate mutation
	// below; the mutation is the bug under test, not a data race with KRT.
	require.Eventually(t, func() bool {
		return pcc.HasSynced() && len(storedClustersForClient(pcc, ucc)) == 1
	}, 2*time.Second, 20*time.Millisecond)
	got := clustersForClient(krt.TestingDummyContext{}, ctx, translator, pcc.base, ucc)
	require.Len(t, got, 1)

	shared := got[0].Cluster
	require.NotPanics(t, func() { shared.ResourceWithTTL() }, "an unmutated shared base must publish")

	// Reach the shared proto the way an aliasing bug would: through the borrow,
	// without cloning. The wrapper exists to make exactly this loud.
	shared.BorrowForRead().AltStatName = "mutated-through-alias"

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		shared.ResourceWithTTL()
	}()
	require.NotNil(t, recovered, "publishing a shared base mutated after wrapping must panic")
	msg, ok := recovered.(string)
	require.True(t, ok, "the tripwire panics with a message, got %T", recovered)
	assert.Contains(t, msg, backend.ClusterName(), "the tripwire must name the mutated cluster")
	assert.Contains(t, msg, "mutated after creation")
}

// failingValidator rejects every overlaid cluster with the same message, standing
// in for a strict-mode validation failure that persists across backend
// generations. The un-overlaid base passes, so the failure is per client.
type failingValidator struct{ err error }

func (f failingValidator) Validate(_ context.Context, bootstrap *envoybootstrapv3.Bootstrap) error {
	for _, c := range bootstrap.GetStaticResources().GetClusters() {
		if c.GetOutlierDetection() != nil {
			return f.err
		}
	}
	return nil
}

// TestNewPerClientEnvoyClusters_PerClientErrorTracksBackendGeneration pins that
// a per-client error row moves with the backend's generation even when its
// message does not. Backend status filters cluster errors by generation, so if
// the client's row compared equal across generations KRT would retain the old
// error row and status would report the new generation as accepted while CDS
// still excludes the cluster.
func TestNewPerClientEnvoyClusters_PerClientErrorTracksBackendGeneration(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	backendGK := schema.GroupKind{Group: "", Kind: "Service"}
	overlayGK := schema.GroupKind{Group: "test", Kind: "Overlay"}
	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			backendGK: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS}
					return nil
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			overlayGK: {
				// Always applies, so every client materializes a cluster and
				// strict validation runs on it.
				PerClientClusterOverlay: func(kctx krt.HandlerContext, ctx context.Context, ucc ir.UniquelyConnectedClient, in ir.BackendObjectIR) *sdk.ClusterOverlay {
					return &sdk.ClusterOverlay{Mutate: func(out *envoyclusterv3.Cluster) {
						out.OutlierDetection = &envoyclusterv3.OutlierDetection{}
					}}
				},
			},
		},
		Mode:      apisettings.ValidationStrict,
		Validator: failingValidator{err: errors.New("rejected by envoy")},
	}

	serviceAt := func(generation int64) *corev1.Service {
		return &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns", Name: "svc", UID: "svc-uid",
			ResourceVersion: strconv.FormatInt(generation, 10), Generation: generation,
		}}
	}
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Group: "", Kind: "Service", Namespace: "ns", Name: "svc"}, 80, "", "")
	backend.Obj = serviceAt(1)
	finalBackends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{&backend}, krtopts.ToOptions("FinalBackends")...)
	ucc := ir.NewUniquelyConnectedClient("client", "ns", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc}, krtopts.ToOptions("UCCs")...)

	pcc := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)
	name := backend.ClusterName()
	statusKey := uccClusterResourceName(ucc, name)

	errorRowAtGeneration := func(generation int64) func() bool {
		return func() bool {
			row := pcc.StatusClusters().GetKey(statusKey)
			return row != nil && row.PerClientError && row.BackendGeneration == generation &&
				row.Error != nil && row.Error.Error() == "rejected by envoy"
		}
	}
	require.Eventually(t, errorRowAtGeneration(1), 2*time.Second, 20*time.Millisecond,
		"the per-client validation failure must reach status at generation 1")
	require.Eventually(t, func() bool {
		stored := storedClustersForClient(pcc, ucc)
		return stored != nil && stored[name] == nil
	}, 2*time.Second, 20*time.Millisecond, "an errored cluster must be excluded from the client's payload")

	// Same client, same error, next generation of the backend.
	updated := backend
	updated.Obj = serviceAt(2)
	finalBackends.UpdateObject(&updated)

	require.Eventually(t, errorRowAtGeneration(2), 2*time.Second, 20*time.Millisecond,
		"the error row must move to generation 2 even though its message is unchanged")
	assert.Nil(t, storedClustersForClient(pcc, ucc)[name], "the cluster must stay excluded at generation 2")
}

// TestNewPerClientEnvoyClusters_ClientIndependentInlineCLASharesBase pins the
// inline-CLA split through the real wiring. A STRICT_DNS backend whose CLA no
// client can influence is published to every client as the one shared base
// proto, CLA included; a sibling with a zone-preferring traffic distribution
// still gets an independently built cluster per client.
func TestNewPerClientEnvoyClusters_ClientIndependentInlineCLASharesBase(t *testing.T) {
	ctx := t.Context()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	backendGK := schema.GroupKind{Group: "group", Kind: "kind"}
	translator := &irtranslator.BackendTranslator{
		ContributedBackends: map[schema.GroupKind]ir.BackendInit{
			backendGK: {
				InitEnvoyBackend: func(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
					out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_STRICT_DNS}
					eps := ir.NewEndpointsForBackend(in)
					if in.GetName() == "zonal" {
						eps.TrafficDistribution = wellknown.TrafficDistributionPreferSameZone
					}
					eps.Add(ir.PodLocality{Region: "r1", Zone: "z1"}, ir.EndpointWithMd{
						LbEndpoint: lbEndpointPipe("z1"),
						EndpointMd: ir.EndpointMetadata{Labels: map[string]string{corev1.LabelTopologyZone: "z1", corev1.LabelZoneRegion: "r1"}},
					})
					eps.Add(ir.PodLocality{Region: "r1", Zone: "z2"}, ir.EndpointWithMd{
						LbEndpoint: lbEndpointPipe("z2"),
						EndpointMd: ir.EndpointMetadata{Labels: map[string]string{corev1.LabelTopologyZone: "z2", corev1.LabelZoneRegion: "r1"}},
					})
					return eps
				},
			},
		},
		ContributedPolicies: map[schema.GroupKind]sdk.PolicyPlugin{},
	}

	shared := ir.NewBackendObjectIR(ir.ObjectSource{Group: "group", Kind: "kind", Namespace: "ns", Name: "shared"}, 80, "", "")
	shared.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{}}
	zonal := ir.NewBackendObjectIR(ir.ObjectSource{Group: "group", Kind: "kind", Namespace: "ns", Name: "zonal"}, 80, "", "")
	zonal.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{}}
	finalBackends := krt.NewStaticCollection(nil, []*ir.BackendObjectIR{&shared, &zonal}, krtopts.ToOptions("FinalBackends")...)

	z1 := ir.NewUniquelyConnectedClient("a", "ns", map[string]string{corev1.LabelTopologyZone: "z1", corev1.LabelZoneRegion: "r1"}, ir.PodLocality{Region: "r1", Zone: "z1"})
	z2 := ir.NewUniquelyConnectedClient("b", "ns", map[string]string{corev1.LabelTopologyZone: "z2", corev1.LabelZoneRegion: "r1"}, ir.PodLocality{Region: "r1", Zone: "z2"})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{z1, z2}, krtopts.ToOptions("UCCs")...)

	pcc := NewPerClientEnvoyClusters(ctx, krtopts, translator, finalBackends, uccs)
	var gotZ1, gotZ2 map[string]*envoyclusterv3.Cluster
	require.Eventually(t, func() bool {
		gotZ1, gotZ2 = storedClustersForClient(pcc, z1), storedClustersForClient(pcc, z2)
		return len(gotZ1) == 2 && len(gotZ2) == 2
	}, time.Second, 10*time.Millisecond)

	sharedName, zonalName := shared.ClusterName(), zonal.ClusterName()
	require.NotNil(t, gotZ1[sharedName].GetLoadAssignment(), "the shared inline cluster must carry its CLA")
	assert.Same(t, gotZ1[sharedName], gotZ2[sharedName],
		"a client-independent inline cluster is one proto shared by every client")

	require.NotNil(t, gotZ1[zonalName].GetLoadAssignment())
	require.NotNil(t, gotZ2[zonalName].GetLoadAssignment())
	assert.NotSame(t, gotZ1[zonalName], gotZ2[zonalName],
		"a zone-preferring inline cluster is built per client")
	assert.NotEqual(t, gotZ1[zonalName].GetLoadAssignment().GetEndpoints(), gotZ2[zonalName].GetLoadAssignment().GetEndpoints(),
		"clients in different zones must see different endpoint priorities")

	bases := krt.Fetch(krt.TestingDummyContext{}, pcc.base)
	require.Len(t, bases, 2)
	for _, b := range bases {
		require.Nil(t, b.Base.Cluster, "the retained BaseCluster must not expose a raw alias to the shared proto")
	}
}
