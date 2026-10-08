package backendconfigpolicy

import (
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestTunnelHookAttributesErrorsToItsPolicy checks that a multi-host destination
// fails with an error attributed to its tunnel policy.
func TestTunnelHookAttributesErrorsToItsPolicy(t *testing.T) {
	gk := wellknown.BackendConfigPolicyGVK.GroupKind()
	backendGK := wellknown.BackendGVK.GroupKind()
	backend := ir.NewBackendObjectIR(ir.ObjectSource{Group: backendGK.Group, Kind: backendGK.Kind, Namespace: "default", Name: "external"}, 0, "", "backend")
	backend.Obj = &kgateway.Backend{
		Spec: kgateway.BackendSpec{
			Static: &kgateway.StaticBackend{
				Hosts: []kgateway.Host{{Host: "external.example.com", Port: 443}, {Host: "external-2.example.com", Port: 443}},
			},
		},
	}
	backend.AttachedPolicies = ir.AttachedPolicies{Policies: map[schema.GroupKind][]ir.PolicyAtt{
		gk: {{GroupKind: gk, PolicyRef: &ir.AttachedPolicyRef{Name: "tunnel"}, PolicyIr: &BackendConfigPolicyIR{tunnel: baseHarnessTunnel()}}},
	}}

	resources, err := newTunnelHook(&collections.CommonCollections{})(krt.TestingDummyContext{}, t.Context(), backend, &envoyclusterv3.Cluster{Name: backend.ClusterName()})

	require.Error(t, err, "a Static Backend with two hosts cannot be tunneled")
	assert.Equal(t, &ir.AttachedPolicyRef{
		Group:     tunnelPolicySource.Group,
		Kind:      tunnelPolicySource.Kind,
		Namespace: tunnelPolicySource.Namespace,
		Name:      tunnelPolicySource.Name,
	}, resources.Policy, "the error must be attributed to the tunnel's policy")
}

// TestTunnelSecretNames checks that a credential rotation keeps a header's
// secret name, while a new proxy or header binding renames it, so Envoy never
// reuses a value cached for another proxy or header.
func TestTunnelSecretNames(t *testing.T) {
	names := func(proxyCluster string, headers ...gwv1.HTTPHeader) []string {
		_, secrets := buildTunnelListener("connect_tunnel_test", proxyCluster, kgateway.Host{Host: "external.example.com", Port: 443}, headers)
		out := make([]string, 0, len(secrets))
		for _, s := range secrets {
			out = append(out, s.GetName())
		}
		return out
	}
	auth := gwv1.HTTPHeader{Name: "Proxy-Authorization", Value: "Basic dXNlcjpwYXNz"}
	client := gwv1.HTTPHeader{Name: "X-Proxy-Client", Value: "gateway"}
	base := names("proxy-a", auth, client)

	assert.Equal(t, base, names("proxy-a", gwv1.HTTPHeader{Name: "Proxy-Authorization", Value: "Basic cm90YXRlZA=="}, client), "a rotated credential must keep its secret name")
	for _, name := range names("proxy-b", auth, client) {
		assert.NotContains(t, base, name, "a new proxy must rename every secret")
	}
	for _, name := range names("proxy-a", client, auth) {
		assert.NotContains(t, base, name, "reordered headers must rename their secrets")
	}
}
