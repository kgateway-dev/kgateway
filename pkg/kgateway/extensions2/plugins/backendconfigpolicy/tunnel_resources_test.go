package backendconfigpolicy

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func tunnelTestBackend() ir.BackendObjectIR {
	gk := wellknown.BackendGVK.GroupKind()
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Group: gk.Group, Kind: gk.Kind, Namespace: "default", Name: "external"}, 0, "", "backend")
	backend.Obj = &kgateway.Backend{
		Spec: kgateway.BackendSpec{
			Static: &kgateway.StaticBackend{
				Hosts: []kgateway.Host{{Host: "external.example.com", Port: 443}},
			},
		},
	}
	return backend
}

// TestTunnelHookAttributesErrorsToItsPolicy checks that a multi-host destination
// fails with an error attributed to its tunnel policy.
func TestTunnelHookAttributesErrorsToItsPolicy(t *testing.T) {
	gk := wellknown.BackendConfigPolicyGVK.GroupKind()
	backend := tunnelTestBackend()
	static := backend.Obj.(*kgateway.Backend).Spec.Static
	static.Hosts = append(static.Hosts, kgateway.Host{Host: "external-2.example.com", Port: 443})
	backend.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
		gk: {{GroupKind: gk, PolicyRef: &ir.AttachedPolicyRef{Name: "tunnel"}, PolicyIr: &BackendConfigPolicyIR{tunnel: baseHarnessTunnel()}}},
	}}

	_, err := newTunnelHook(&collections.CommonCollections{})(krt.TestingDummyContext{}, t.Context(), backend, &envoyclusterv3.Cluster{Name: backend.ClusterName()})

	require.Error(t, err, "a Static Backend with two hosts cannot be tunneled")
	var policyErr *ir.PolicyError
	require.ErrorAs(t, err, &policyErr, "the error must name the tunnel's policy")
	assert.Equal(t, &ir.AttachedPolicyRef{
		Group:     tunnelPolicySource.Group,
		Kind:      tunnelPolicySource.Kind,
		Namespace: tunnelPolicySource.Namespace,
		Name:      tunnelPolicySource.Name,
	}, policyErr.Ref)
}
