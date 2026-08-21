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
