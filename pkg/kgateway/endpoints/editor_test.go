package endpoints

import (
	"errors"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/api/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestEndpointInputsResolverBuildsReplacementWithStructuralSharing(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	backend.Obj = &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"scope": "peered"}}}
	baseEndpoints := ir.NewEndpointsForBackend(backend)
	locality := ir.PodLocality{Region: "r", Zone: "z"}
	baseEndpoints.Add(locality, editorTestEndpoint("10.0.0.1", "unchanged"))
	baseEndpoints.Add(locality, editorTestEndpoint("10.0.0.2", "changed"))

	base := EndpointsInputs{EndpointsForBackend: *baseEndpoints}
	resolver := NewEndpointInputsResolver(base)
	labels := resolver.BackendLabels()
	labels["scope"] = "mutated"
	require.Equal(t, "peered", base.EndpointsForBackend.BackendLabels["scope"], "read access must not expose the shared label map")

	replacement := resolver.NewEndpointSet()
	resolver.ForEachEndpoint(func(locality ir.PodLocality, endpoint EndpointView) bool {
		if endpoint.Label("id") == "unchanged" {
			replacement.AddUnchanged(locality, endpoint)
			return true
		}
		replacement.AddModified(locality, endpoint, func(ep *ir.EndpointWithMd) {
			ep.EndpointMd.Labels["id"] = "modified"
			ep.GetEndpoint().GetAddress().GetSocketAddress().Address = "127.0.0.1"
		})
		return true
	})
	resolver.ReplaceEndpoints(replacement)

	resolved := resolver.Inputs()
	require.Len(t, resolved.EndpointsForBackend.LbEps[locality], 2)
	unchanged := resolved.EndpointsForBackend.LbEps[locality][0]
	modified := resolved.EndpointsForBackend.LbEps[locality][1]
	require.Same(t, base.EndpointsForBackend.LbEps[locality][0].LbEndpoint, unchanged.LbEndpoint,
		"unchanged endpoints should retain their immutable shared proto")
	require.NotSame(t, base.EndpointsForBackend.LbEps[locality][1].LbEndpoint, modified.LbEndpoint,
		"modified endpoints must use an isolated clone")
	assert.Equal(t, "changed", base.EndpointsForBackend.LbEps[locality][1].EndpointMd.Labels["id"])
	assert.Equal(t, "10.0.0.2", base.EndpointsForBackend.LbEps[locality][1].GetEndpoint().GetAddress().GetSocketAddress().GetAddress())
	assert.Equal(t, "modified", modified.EndpointMd.Labels["id"])
	assert.Equal(t, "127.0.0.1", modified.GetEndpoint().GetAddress().GetSocketAddress().GetAddress())
	assert.NotEqual(t, base.EndpointsForBackend.LbEpsEqualityHash, resolved.EndpointsForBackend.LbEpsEqualityHash,
		"the replacement builder must hash its resolved endpoint content")
}

func TestEndpointSetBuilderRehashesWhenLocalityChanges(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	sourceLocality := ir.PodLocality{Region: "r1", Zone: "z1"}
	targetLocality := ir.PodLocality{Region: "r2", Zone: "z2"}
	source := ir.NewEndpointsForBackend(backend)
	source.Add(sourceLocality, editorTestEndpoint("10.0.0.1", "moved"))

	resolver := NewEndpointInputsResolver(EndpointsInputs{EndpointsForBackend: *source})
	replacement := resolver.NewEndpointSet()
	resolver.ForEachEndpoint(func(_ ir.PodLocality, endpoint EndpointView) bool {
		replacement.AddUnchanged(targetLocality, endpoint)
		return true
	})

	want := ir.NewEndpointsForBackend(backend)
	want.Add(targetLocality, source.LbEps[sourceLocality][0])
	assert.Equal(t, want.LbEpsEqualityHash, replacement.state.endpoints.LbEpsEqualityHash,
		"moving an endpoint must hash its new locality")
}

func TestEndpointInputsResolverDeepCopiesLegacyMutableInputs(t *testing.T) {
	groupKind := schema.GroupKind{Group: "example.io", Kind: "Policy"}
	policyRef := &ir.AttachedPolicyRef{Name: "policy", Namespace: "ns"}
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	backend.Obj = &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"scope": "peered"}}}
	backend.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
		groupKind: {{
			PolicyRef:    policyRef,
			Errors:       []error{errors.New("base")},
			MergeOrigins: ir.MergeOrigins{"field": sets.New("origin")},
		}},
	}}
	baseEndpoints := ir.NewEndpointsForBackend(backend)
	locality := ir.PodLocality{Region: "r", Zone: "z"}
	baseEndpoints.Add(locality, editorTestEndpoint("10.0.0.1", "base"))
	base := EndpointsInputs{
		EndpointsForBackend: *baseEndpoints,
		PriorityInfo: &PriorityInfo{
			FailoverPriority: NewPriorities([]string{"topology.kubernetes.io/zone"}),
		},
	}

	resolver := NewEndpointInputsResolver(base)
	legacy := resolver.LegacyMutableInputs()
	legacy.EndpointsForBackend.BackendLabels["scope"] = "mutated"
	legacy.EndpointsForBackend.AttachedPolicies.Policies[groupKind][0].PolicyRef.Name = "mutated"
	legacy.EndpointsForBackend.AttachedPolicies.Policies[groupKind][0].Errors[0] = errors.New("mutated")
	legacy.EndpointsForBackend.AttachedPolicies.Policies[groupKind][0].MergeOrigins["field"].Insert("mutated")
	legacy.EndpointsForBackend.LbEps[locality][0].EndpointMd.Labels["id"] = "mutated"
	legacy.EndpointsForBackend.LbEps[locality][0].GetEndpoint().GetAddress().GetSocketAddress().Address = "127.0.0.1"
	legacy.PriorityInfo.FailoverPriority.priorityLabels[0] = "mutated"

	assert.Equal(t, "peered", base.EndpointsForBackend.BackendLabels["scope"])
	assert.Equal(t, "policy", base.EndpointsForBackend.AttachedPolicies.Policies[groupKind][0].PolicyRef.Name)
	assert.EqualError(t, base.EndpointsForBackend.AttachedPolicies.Policies[groupKind][0].Errors[0], "base")
	assert.False(t, base.EndpointsForBackend.AttachedPolicies.Policies[groupKind][0].MergeOrigins["field"].Has("mutated"))
	assert.Equal(t, "base", base.EndpointsForBackend.LbEps[locality][0].EndpointMd.Labels["id"])
	assert.Equal(t, "10.0.0.1", base.EndpointsForBackend.LbEps[locality][0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress())
	assert.Equal(t, "topology.kubernetes.io/zone", base.PriorityInfo.FailoverPriority.priorityLabels[0])
	require.Same(t, legacy, resolver.LegacyMutableInputs(), "legacy inputs should be cloned only once per client")

	// A later editor plugin must not reuse the contribution cached before the
	// legacy mutation. AddUnchanged falls back to hashing the now-mutated proto.
	replacement := resolver.NewEndpointSet()
	resolver.ForEachEndpoint(func(locality ir.PodLocality, endpoint EndpointView) bool {
		replacement.AddUnchanged(locality, endpoint)
		return true
	})
	want := ir.NewEndpointsForBackend(backend)
	want.Add(locality, legacy.EndpointsForBackend.LbEps[locality][0])
	assert.Equal(t, want.LbEpsEqualityHash, replacement.state.endpoints.LbEpsEqualityHash,
		"legacy mutation must invalidate the cached endpoint contribution")
}

func TestEndpointInputsResolverSetPriorityInfoClonesInput(t *testing.T) {
	resolver := NewEndpointInputsResolver(EndpointsInputs{})
	priorityInfo := &PriorityInfo{
		FailoverPriority: NewPriorities([]string{"region=west", "zone"}),
		Failover: []*v1alpha3.LocalityLoadBalancerSetting_Failover{
			{From: "west", To: "east"},
			nil,
		},
	}

	resolver.SetPriorityInfo(priorityInfo)
	installed := resolver.Inputs().PriorityInfo
	require.NotSame(t, priorityInfo, installed)
	require.NotSame(t, priorityInfo.FailoverPriority, installed.FailoverPriority)
	require.NotSame(t, priorityInfo.Failover[0], installed.Failover[0])

	priorityInfo.FailoverPriority.priorityLabels[0] = "mutated"
	priorityInfo.FailoverPriority.priorityLabelOverrides["region"] = "mutated"
	priorityInfo.Failover[0].From = "mutated"
	priorityInfo.Failover = nil
	priorityInfo.FailoverPriority = nil

	assert.Equal(t, []string{"region", "zone"}, installed.FailoverPriority.priorityLabels)
	assert.Equal(t, map[string]string{"region": "west"}, installed.FailoverPriority.priorityLabelOverrides)
	require.Len(t, installed.Failover, 2)
	assert.Equal(t, "west", installed.Failover[0].GetFrom())
	assert.Equal(t, "east", installed.Failover[0].GetTo())
	assert.Nil(t, installed.Failover[1])
}

func TestEndpointSetBuilderIsConsumedByReplace(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	resolver := NewEndpointInputsResolver(EndpointsInputs{EndpointsForBackend: *ir.NewEndpointsForBackend(backend)})
	builder := resolver.NewEndpointSet()
	builderCopy := *builder
	builder.Add(ir.PodLocality{}, editorTestEndpoint("10.0.0.1", "installed"))

	resolver.ReplaceEndpoints(builder)
	installed := resolver.Inputs().EndpointsForBackend
	installedHash := installed.LbEpsEqualityHash

	for name, candidate := range map[string]*EndpointSetBuilder{
		"original":     builder,
		"copied value": &builderCopy,
	} {
		t.Run(name, func(t *testing.T) {
			assert.PanicsWithValue(t, "endpoint replacement builder has already been consumed", func() {
				candidate.Add(ir.PodLocality{}, editorTestEndpoint("10.0.0.2", "late"))
			})
		})
	}
	assert.PanicsWithValue(t, "endpoint replacement builder has already been consumed", func() {
		resolver.ReplaceEndpoints(builder)
	})

	got := resolver.Inputs().EndpointsForBackend
	assert.Equal(t, installedHash, got.LbEpsEqualityHash, "rejected writes must not skew the installed hash")
	assert.Len(t, got.LbEps[ir.PodLocality{}], 1, "rejected writes must not mutate installed endpoint content")
}

func TestEndpointSetBuilderRejectsWrongOwnerAndNil(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	inputs := EndpointsInputs{EndpointsForBackend: *ir.NewEndpointsForBackend(backend)}
	owner := NewEndpointInputsResolver(inputs)
	other := NewEndpointInputsResolver(inputs)
	builder := owner.NewEndpointSet()

	assert.PanicsWithValue(t, "endpoint replacement builder belongs to a different resolver", func() {
		other.ReplaceEndpoints(builder)
	})
	// Rejection by the wrong owner does not consume the builder; its owner can
	// still finish and install it.
	builder.Add(ir.PodLocality{}, editorTestEndpoint("10.0.0.1", "owned"))
	owner.ReplaceEndpoints(builder)

	assert.PanicsWithValue(t, "endpoint replacement builder is nil", func() {
		other.ReplaceEndpoints(nil)
	})
	assert.PanicsWithValue(t, "endpoint replacement builder is uninitialized", func() {
		new(EndpointSetBuilder).Add(ir.PodLocality{}, editorTestEndpoint("10.0.0.2", "uninitialized"))
	})
}

// PoliciesFor hands out views rather than copies, so the mutable attachment
// metadata is unreachable by construction instead of defensively cloned on a
// per-client-per-backend path. What the views must still do is answer every
// question the endpoint plugins ask, including for an attachment that failed IR
// construction.
func TestEndpointInputsResolverPoliciesForExposesReadsOnly(t *testing.T) {
	groupKind := schema.GroupKind{Group: "example.io", Kind: "Policy"}
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	backend.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
		groupKind: {
			{
				PolicyRef:  &ir.AttachedPolicyRef{Group: "example.io", Kind: "Policy", Name: "good", Namespace: "ns"},
				Generation: 7,
			},
			{
				PolicyRef:    &ir.AttachedPolicyRef{Group: "example.io", Kind: "Policy", Name: "broken", Namespace: "ns"},
				Errors:       []error{errors.New("bad config")},
				MergeOrigins: ir.MergeOrigins{"field": sets.New("origin")},
			},
		},
	}}
	base := EndpointsInputs{EndpointsForBackend: *ir.NewEndpointsForBackend(backend)}

	policies := NewEndpointInputsResolver(base).PoliciesFor(groupKind)
	require.Len(t, policies, 2)

	assert.False(t, policies[0].HasErrors())
	assert.Equal(t, "example.io/Policy/ns/good/", policies[0].RefString())
	assert.Equal(t, int64(7), policies[0].Generation())

	assert.True(t, policies[1].HasErrors(), "an attachment that failed IR construction must report it")
	assert.Equal(t, "example.io/Policy/ns/broken/", policies[1].RefString())

	assert.Nil(t, NewEndpointInputsResolver(base).PoliciesFor(schema.GroupKind{Kind: "Absent"}),
		"a kind with no attachments should allocate nothing")
}

// TestReplaceEndpointsKeepsFoldedVersion covers the trap in building a
// replacement endpoint set: EmptyCopy reseeds the equality hash from backend
// identity, so anything the row's owner folded in afterwards — the
// attached-policy hash, in production — would vanish and stop distinguishing
// the states it was folded in to distinguish. The resolved hash keys CLA
// interning, so two policy states must not resolve alike.
func TestReplaceEndpointsKeepsFoldedVersion(t *testing.T) {
	locality := ir.PodLocality{Region: "r", Zone: "z"}

	resolvedHashFor := func(policyVersion uint64) uint64 {
		backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
		source := ir.NewEndpointsForBackend(backend)
		source.Add(locality, editorTestEndpoint("10.0.0.1", "ep"))
		// Mirrors newFinalBackendEndpoints folding the attached-policy hash into
		// the row it publishes.
		source.FoldVersion(policyVersion)

		resolver := NewEndpointInputsResolver(EndpointsInputs{EndpointsForBackend: *source})
		// A plugin that rebuilds the set without changing any endpoint.
		replacement := resolver.NewEndpointSet()
		resolver.ForEachEndpoint(func(locality ir.PodLocality, endpoint EndpointView) bool {
			replacement.AddUnchanged(locality, endpoint)
			return true
		})
		resolver.ReplaceEndpoints(replacement)
		return resolver.Inputs().EndpointsForBackend.LbEpsEqualityHash
	}

	assert.NotEqual(t, resolvedHashFor(1), resolvedHashFor(2),
		"a policy-only change must survive the replacement path")
	assert.Equal(t, resolvedHashFor(1), resolvedHashFor(1),
		"the replacement path must stay deterministic for equal inputs")
}

// A plugin that rebuilds the endpoint set without changing anything must land
// on the source hash exactly, not merely on a stable one. The resolved hash is
// this client's EDS resource version, so anything else republishes an identical
// CLA to every connected Envoy on every recompute.
func TestNoOpRebuildReproducesTheSourceHash(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	backend.Obj = &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"scope": "peered"}}}
	source := ir.NewEndpointsForBackend(backend)
	source.Add(ir.PodLocality{Region: "r", Zone: "z1"}, editorTestEndpoint("10.0.0.1", "ep-1"))
	source.Add(ir.PodLocality{Region: "r", Zone: "z2"}, editorTestEndpoint("10.0.0.2", "ep-2"))
	source.Add(ir.PodLocality{}, editorTestEndpoint("10.0.0.3", "ep-3"))
	source.FoldVersion(7)

	resolver := NewEndpointInputsResolver(EndpointsInputs{EndpointsForBackend: *source})
	replacement := resolver.NewEndpointSet()
	resolver.ForEachEndpoint(func(locality ir.PodLocality, endpoint EndpointView) bool {
		replacement.AddUnchanged(locality, endpoint)
		return true
	})
	resolver.ReplaceEndpoints(replacement)

	assert.Equal(t, source.LbEpsEqualityHash, resolver.Inputs().EndpointsForBackend.LbEpsEqualityHash,
		"a no-op rebuild must not bump the EDS version")
	assert.True(t, source.Equals(resolver.Inputs().EndpointsForBackend),
		"a no-op rebuild must be indistinguishable from its source to KRT")
}

// Add is the one entry point that takes a caller-owned mutable graph, so it is
// the one that has to take ownership of it. A plugin that keeps the value it
// contributed must not be able to reach into the resolved set — either to
// change content the version hash was already taken over, or to reach a client
// that has already been resolved.
func TestEndpointSetBuilderAddCopiesCallerOwnedEndpoint(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	base := EndpointsInputs{EndpointsForBackend: *ir.NewEndpointsForBackend(backend)}
	locality := ir.PodLocality{Region: "r", Zone: "z"}
	retained := editorTestEndpoint("10.0.0.1", "original")

	first := NewEndpointInputsResolver(base)
	firstBuilder := first.NewEndpointSet()
	firstBuilder.Add(locality, retained)
	first.ReplaceEndpoints(firstBuilder)
	firstResolved := first.Inputs().EndpointsForBackend
	firstHash := firstResolved.LbEpsEqualityHash

	// The plugin reuses the same endpoint value for the next client, mutating it
	// on the way.
	second := NewEndpointInputsResolver(base)
	secondBuilder := second.NewEndpointSet()
	retained.GetEndpoint().GetAddress().GetSocketAddress().Address = "10.0.0.2"
	retained.EndpointMd.Labels["id"] = "reused"
	secondBuilder.Add(locality, retained)
	second.ReplaceEndpoints(secondBuilder)
	secondResolved := second.Inputs().EndpointsForBackend

	firstInstalled := firstResolved.LbEps[locality][0]
	assert.Equal(t, "10.0.0.1", firstInstalled.GetEndpoint().GetAddress().GetSocketAddress().GetAddress(),
		"a later client's pass must not reach an already resolved client")
	assert.Equal(t, "original", firstInstalled.EndpointMd.Labels["id"])
	require.NotSame(t, firstInstalled.LbEndpoint, secondResolved.LbEps[locality][0].LbEndpoint,
		"two clients must not share one endpoint proto")
	assert.Equal(t, firstHash, firstResolved.LbEpsEqualityHash)
	assert.NotEqual(t, firstHash, secondResolved.LbEpsEqualityHash,
		"different resolved content must resolve to different versions")

	// The same aliasing within one client shows up as a version that no longer
	// describes the installed content.
	third := NewEndpointInputsResolver(base)
	thirdBuilder := third.NewEndpointSet()
	own := editorTestEndpoint("10.0.0.3", "own")
	thirdBuilder.Add(locality, own)
	own.GetEndpoint().GetAddress().GetSocketAddress().Address = "10.0.0.4"
	third.ReplaceEndpoints(thirdBuilder)
	assert.Equal(t, "10.0.0.3", third.Inputs().EndpointsForBackend.LbEps[locality][0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress(),
		"mutating after Add must not change installed content the hash was taken over")
}

func TestEndpointSetBuilderAddModifiedClonesAndRehashes(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	locality := ir.PodLocality{Region: "r", Zone: "z"}
	source := ir.NewEndpointsForBackend(backend)
	source.Add(locality, editorTestEndpoint("10.0.0.1", "source"))
	base := EndpointsInputs{EndpointsForBackend: *source}

	resolver := NewEndpointInputsResolver(base)
	builder := resolver.NewEndpointSet()
	resolver.ForEachEndpoint(func(locality ir.PodLocality, endpoint EndpointView) bool {
		builder.AddModified(locality, endpoint, func(ep *ir.EndpointWithMd) {
			ep.EndpointMd.Labels["id"] = "derived"
			ep.GetEndpoint().GetAddress().GetSocketAddress().Address = "10.0.0.9"
		})
		return true
	})
	resolver.ReplaceEndpoints(builder)

	resolved := resolver.Inputs().EndpointsForBackend.LbEps[locality][0]
	assert.Equal(t, "derived", resolved.EndpointMd.Labels["id"])
	assert.Equal(t, "10.0.0.9", resolved.GetEndpoint().GetAddress().GetSocketAddress().GetAddress())
	assert.Equal(t, "source", base.EndpointsForBackend.LbEps[locality][0].EndpointMd.Labels["id"],
		"the immutable source must be untouched")
	assert.Equal(t, "10.0.0.1", base.EndpointsForBackend.LbEps[locality][0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress())
	require.NotSame(t, base.EndpointsForBackend.LbEps[locality][0].LbEndpoint, resolved.LbEndpoint)

	want := ir.NewEndpointsForBackend(backend)
	want.Add(locality, resolved)
	assert.Equal(t, want.LbEpsEqualityHash, resolver.Inputs().EndpointsForBackend.LbEpsEqualityHash,
		"the modified endpoint must be hashed as it was installed")

	assert.PanicsWithValue(t, "endpoint modifier is nil", func() {
		other := NewEndpointInputsResolver(base)
		otherBuilder := other.NewEndpointSet()
		other.ForEachEndpoint(func(locality ir.PodLocality, endpoint EndpointView) bool {
			otherBuilder.AddModified(locality, endpoint, nil)
			return true
		})
	})
}

// A builder is seeded from the inputs as they stood when it was created, so
// installing it wholesale would roll back anything a setter wrote while it was
// being populated. That failure is silent: the resolved hash faithfully
// versions the reverted state, so nothing downstream notices.
func TestReplaceEndpointsKeepsSettersWrittenWhileBuilding(t *testing.T) {
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "")
	locality := ir.PodLocality{Region: "r", Zone: "z"}
	resolver := NewEndpointInputsResolver(EndpointsInputs{EndpointsForBackend: *ir.NewEndpointsForBackend(backend)})

	builder := resolver.NewEndpointSet()
	// BackendConfigPolicy's zone-aware hook writes exactly this, and a plugin
	// that also rewrote endpoints would build the replacement around it.
	resolver.SetTrafficDistribution(wellknown.TrafficDistributionPreferSameZone)
	builder.Add(locality, editorTestEndpoint("10.0.0.1", "ep"))
	resolver.ReplaceEndpoints(builder)

	assert.Equal(t, wellknown.TrafficDistributionPreferSameZone, resolver.Inputs().EndpointsForBackend.TrafficDistribution,
		"installing a replacement must not revert the resolver's own fields")
	assert.Len(t, resolver.Inputs().EndpointsForBackend.LbEps[locality], 1)
}

func editorTestEndpoint(address, id string) ir.EndpointWithMd {
	return ir.EndpointWithMd{
		LbEndpoint: &envoyendpointv3.LbEndpoint{
			HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{Endpoint: &envoyendpointv3.Endpoint{
				Address: &envoycorev3.Address{Address: &envoycorev3.Address_SocketAddress{SocketAddress: &envoycorev3.SocketAddress{
					Address: address,
				}}},
			}},
		},
		EndpointMd: ir.EndpointMetadata{Labels: map[string]string{"id": id}},
	}
}
