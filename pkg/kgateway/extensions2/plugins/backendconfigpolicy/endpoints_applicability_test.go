package backendconfigpolicy

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/overlaytest"
)

// TestEndpointsMayApplyCoversProcessEndpoints checks the promise this plugin
// makes to the framework: a backend its PerClientEndpointsMayApply rules out is
// one processEndpoints would not have touched, for any client. The framework
// does not invoke the hook for such a backend and builds one inline
// ClusterLoadAssignment shared by every client, so a predicate that rules out
// too much loses this plugin's zone-aware configuration silently.
//
// The pairing under test is the registration's, not a restatement of it: both
// fields are taken from the same values NewPlugin registers.
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
			// hook would not touch costs a per-client build and is sound.
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
