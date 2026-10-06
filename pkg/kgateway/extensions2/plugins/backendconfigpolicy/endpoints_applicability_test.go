package backendconfigpolicy

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/overlaytest"
)

// TestEndpointsMayApplyCoversProcessEndpoints checks that processEndpoints
// contributes nothing, for any client, to a backend the plugin's
// PerClientEndpointsMayApply rules out. The framework skips the hook for such
// backends, so a predicate that rules out too much drops zone-aware endpoints
// silently. See overlaytest.AssertMayApplyCoversEndpointHook.
func TestEndpointsMayApplyCoversProcessEndpoints(t *testing.T) {
	groupKind := wellknown.BackendConfigPolicyGVK.GroupKind()
	endpointPlugin := &backendConfigEndpointPlugin{}
	plugin := sdk.PolicyPlugin{
		Name:                       "BackendConfigPolicy",
		PerClientEditEndpoints:     endpointPlugin.processEndpoints,
		PerClientEndpointsMayApply: sdk.AttachedPolicyEndpointsMayApply(groupKind),
	}

	backend := func(name string, attached map[schema.GroupKind][]ir.PolicyAtt) ir.BackendObjectIR {
		b := ir.NewBackendObjectIR(ir.ObjectSource{
			Group: "", Kind: "Service", Namespace: "ns", Name: name,
		}, 80, "", "")
		b.AttachedPolicies = ir.AttachedPolicies{Policies: attached}
		return b
	}

	overlaytest.AssertMayApplyCoversEndpointHook(t, overlaytest.EndpointsCase{
		Plugin: plugin,
		Backends: []ir.BackendObjectIR{
			// Ruled out: nothing of this kind is attached, so PoliciesFor comes
			// back empty and the hook has nothing to select.
			backend("no-policies", nil),
			backend("other-kind-only", map[schema.GroupKind][]ir.PolicyAtt{
				{Group: "test", Kind: "OtherPolicy"}: {{GroupKind: schema.GroupKind{Group: "test", Kind: "OtherPolicy"}}},
			}),
			// Admitted. The hook may still decline this one, because the
			// attached policy need not be zone-aware; admitting a backend the
			// hook would not touch is safe, but costs a per-client build.
			backend("attached", map[schema.GroupKind][]ir.PolicyAtt{
				groupKind: {{GroupKind: groupKind}},
			}),
		},
		Clients: []ir.UniquelyConnectedClient{
			ir.NewUniquelyConnectedClient("role", "ns", nil, ir.PodLocality{Region: "r1", Zone: "z1"}),
			ir.NewUniquelyConnectedClient("role", "other-ns", map[string]string{"topology.kubernetes.io/zone": "z2"}, ir.PodLocality{Region: "r1", Zone: "z2"}),
		},
	})
}
