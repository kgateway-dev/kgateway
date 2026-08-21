package endpoints

import (
	"errors"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"

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
		cloned := endpoint.Clone()
		cloned.EndpointMd.Labels["id"] = "modified"
		cloned.GetEndpoint().GetAddress().GetSocketAddress().Address = "127.0.0.1"
		replacement.Add(locality, cloned)
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
