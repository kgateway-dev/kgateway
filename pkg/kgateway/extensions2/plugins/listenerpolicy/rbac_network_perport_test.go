package listenerpolicy

import (
	"testing"
	"time"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyrbacnetwork "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// TestPerPortRBAC_DefaultOnly tests that default RBAC is applied when no per-port config exists
func TestPerPortRBAC_DefaultOnly(t *testing.T) {
	spec := &kgateway.ListenerPolicySpec{
		Default: &kgateway.ListenerDefaultConfig{
			ListenerConfig: kgateway.ListenerConfig{
				RBAC: &sharedv1alpha1.Authorization{
					Policy: sharedv1alpha1.AuthorizationPolicy{
						MatchExpressions: []sharedv1alpha1.CELExpression{
							`source.address.startsWith("10.0.0.")`,
						},
					},
					Action: sharedv1alpha1.AuthorizationPolicyActionAllow,
				},
			},
		},
	}

	objSrc := ir.ObjectSource{
		Namespace: "test-ns",
		Name:      "default-policy",
	}

	policyIR, errs := NewListenerPolicyIR(nil, nil, time.Now(), spec, objSrc)
	require.Empty(t, errs)
	require.NotNil(t, policyIR)

	// Verify default policy has RBAC
	assert.NotNil(t, policyIR.defaultPolicy.rbacNetworkFilter)

	// Verify per-port map is empty
	assert.Empty(t, policyIR.perPortPolicy)

	// Test getPolicy for any port returns default
	pass := &listenerPolicyPluginGwPass{}
	policy := pass.getPolicy(policyIR, 8080)
	assert.NotNil(t, policy.rbacNetworkFilter)

	policy = pass.getPolicy(policyIR, 8443)
	assert.NotNil(t, policy.rbacNetworkFilter)
}

// TestPerPortRBAC_PerPortOverride tests that per-port RBAC overrides default
func TestPerPortRBAC_PerPortOverride(t *testing.T) {
	spec := &kgateway.ListenerPolicySpec{
		Default: &kgateway.ListenerDefaultConfig{
			ListenerConfig: kgateway.ListenerConfig{
				RBAC: &sharedv1alpha1.Authorization{
					Policy: sharedv1alpha1.AuthorizationPolicy{
						MatchExpressions: []sharedv1alpha1.CELExpression{
							`source.address.startsWith("10.0.0.")`,
						},
					},
					Action: sharedv1alpha1.AuthorizationPolicyActionAllow,
				},
			},
		},
		PerPort: []kgateway.ListenerPortConfig{
			{
				Port: 8443,
				Listener: kgateway.ListenerConfig{
					RBAC: &sharedv1alpha1.Authorization{
						Policy: sharedv1alpha1.AuthorizationPolicy{
							MatchExpressions: []sharedv1alpha1.CELExpression{
								`source.address.startsWith("192.168.0.")`,
							},
						},
						Action: sharedv1alpha1.AuthorizationPolicyActionDeny,
					},
				},
			},
		},
	}

	objSrc := ir.ObjectSource{
		Namespace: "test-ns",
		Name:      "override-policy",
	}

	policyIR, errs := NewListenerPolicyIR(nil, nil, time.Now(), spec, objSrc)
	require.Empty(t, errs)
	require.NotNil(t, policyIR)

	// Verify default policy has RBAC
	assert.NotNil(t, policyIR.defaultPolicy.rbacNetworkFilter)

	// Verify per-port policy exists for 8443
	assert.Len(t, policyIR.perPortPolicy, 1)
	assert.NotNil(t, policyIR.perPortPolicy[8443].rbacNetworkFilter)

	// Test getPolicy returns correct policy per port
	pass := &listenerPolicyPluginGwPass{}

	// Port 8080 should get default
	policy8080 := pass.getPolicy(policyIR, 8080)
	assert.NotNil(t, policy8080.rbacNetworkFilter)

	// Port 8443 should get per-port override
	policy8443 := pass.getPolicy(policyIR, 8443)
	assert.NotNil(t, policy8443.rbacNetworkFilter)

	// Verify they're different filters
	assert.NotEqual(t, policy8080.rbacNetworkFilter, policy8443.rbacNetworkFilter)

	// Unmarshal and verify stat prefixes are different
	var rbac8080 envoyrbacnetwork.RBAC
	err := policy8080.rbacNetworkFilter.UnmarshalTo(&rbac8080)
	require.NoError(t, err)

	var rbac8443 envoyrbacnetwork.RBAC
	err = policy8443.rbacNetworkFilter.UnmarshalTo(&rbac8443)
	require.NoError(t, err)

	// Both should have the same stat prefix (from same policy object)
	assert.Equal(t, "test-ns_override-policy_network_rbac", rbac8080.StatPrefix)
	assert.Equal(t, "test-ns_override-policy_network_rbac", rbac8443.StatPrefix)
}

// TestPerPortRBAC_MultiplePorts tests multiple ports with different RBAC configs
func TestPerPortRBAC_MultiplePorts(t *testing.T) {
	spec := &kgateway.ListenerPolicySpec{
		PerPort: []kgateway.ListenerPortConfig{
			{
				Port: 8080,
				Listener: kgateway.ListenerConfig{
					RBAC: &sharedv1alpha1.Authorization{
						Policy: sharedv1alpha1.AuthorizationPolicy{
							MatchExpressions: []sharedv1alpha1.CELExpression{
								`source.address.startsWith("10.0.0.")`,
							},
						},
						Action: sharedv1alpha1.AuthorizationPolicyActionAllow,
					},
				},
			},
			{
				Port: 8443,
				Listener: kgateway.ListenerConfig{
					RBAC: &sharedv1alpha1.Authorization{
						Policy: sharedv1alpha1.AuthorizationPolicy{
							MatchExpressions: []sharedv1alpha1.CELExpression{
								`source.address.startsWith("192.168.0.")`,
							},
						},
						Action: sharedv1alpha1.AuthorizationPolicyActionDeny,
					},
				},
			},
			{
				Port: 9090,
				Listener: kgateway.ListenerConfig{
					RBAC: &sharedv1alpha1.Authorization{
						Policy: sharedv1alpha1.AuthorizationPolicy{
							MatchExpressions: []sharedv1alpha1.CELExpression{
								`source.address == "10.1.1.1"`,
							},
						},
						Action: sharedv1alpha1.AuthorizationPolicyActionAllow,
					},
				},
			},
		},
	}

	objSrc := ir.ObjectSource{
		Namespace: "test-ns",
		Name:      "multi-port-policy",
	}

	policyIR, errs := NewListenerPolicyIR(nil, nil, time.Now(), spec, objSrc)
	require.Empty(t, errs)
	require.NotNil(t, policyIR)

	// Verify all three per-port policies exist
	assert.Len(t, policyIR.perPortPolicy, 3)
	assert.NotNil(t, policyIR.perPortPolicy[8080].rbacNetworkFilter)
	assert.NotNil(t, policyIR.perPortPolicy[8443].rbacNetworkFilter)
	assert.NotNil(t, policyIR.perPortPolicy[9090].rbacNetworkFilter)

	// Test getPolicy returns correct policy per port
	pass := &listenerPolicyPluginGwPass{}

	policy8080 := pass.getPolicy(policyIR, 8080)
	policy8443 := pass.getPolicy(policyIR, 8443)
	policy9090 := pass.getPolicy(policyIR, 9090)

	// All should have RBAC filters
	assert.NotNil(t, policy8080.rbacNetworkFilter)
	assert.NotNil(t, policy8443.rbacNetworkFilter)
	assert.NotNil(t, policy9090.rbacNetworkFilter)

	// All should be different
	assert.NotEqual(t, policy8080.rbacNetworkFilter, policy8443.rbacNetworkFilter)
	assert.NotEqual(t, policy8080.rbacNetworkFilter, policy9090.rbacNetworkFilter)
	assert.NotEqual(t, policy8443.rbacNetworkFilter, policy9090.rbacNetworkFilter)
}

// TestPerPortRBAC_FilterApplication tests that filters are correctly applied to listeners
func TestPerPortRBAC_FilterApplication(t *testing.T) {
	spec := &kgateway.ListenerPolicySpec{
		Default: &kgateway.ListenerDefaultConfig{
			ListenerConfig: kgateway.ListenerConfig{
				RBAC: &sharedv1alpha1.Authorization{
					Policy: sharedv1alpha1.AuthorizationPolicy{
						MatchExpressions: []sharedv1alpha1.CELExpression{
							`source.address.startsWith("10.0.0.")`,
						},
					},
					Action: sharedv1alpha1.AuthorizationPolicyActionAllow,
				},
			},
		},
	}

	objSrc := ir.ObjectSource{
		Namespace: "test-ns",
		Name:      "filter-test",
	}

	policyIR, errs := NewListenerPolicyIR(nil, nil, time.Now(), spec, objSrc)
	require.Empty(t, errs)
	require.NotNil(t, policyIR)

	// Create a mock listener
	listener := &envoylistenerv3.Listener{
		Name: "test-listener",
		FilterChains: []*envoylistenerv3.FilterChain{
			{
				Filters: []*envoylistenerv3.Filter{
					{Name: "envoy.filters.network.http_connection_manager"},
				},
			},
		},
	}

	// Create plugin pass and apply
	pass := &listenerPolicyPluginGwPass{
		rbacNetworkFilters: map[uint32]*anypb.Any{},
	}

	pCtx := &ir.ListenerContext{
		Policy: policyIR,
		Port:   8080,
	}

	pass.ApplyListenerPlugin(pCtx, listener)

	// Verify RBAC filter is returned by NetworkFilters
	networkFilters, err := pass.NetworkFilters()
	require.NoError(t, err)
	require.Len(t, networkFilters, 1)

	// Filter should be RBAC
	assert.Equal(t, "envoy.filters.network.rbac", networkFilters[0].Filter.Name)
	assert.Equal(t, filters.BeforeStage(filters.AuthZStage), networkFilters[0].Stage)

	// Verify filter was tracked in map
	assert.NotNil(t, pass.rbacNetworkFilters[8080])
}

// TestPerPortRBAC_NoRBAC tests that listeners without RBAC don't get filters
func TestPerPortRBAC_NoRBAC(t *testing.T) {
	spec := &kgateway.ListenerPolicySpec{
		Default: &kgateway.ListenerDefaultConfig{
			// No RBAC configured
		},
	}

	objSrc := ir.ObjectSource{
		Namespace: "test-ns",
		Name:      "no-rbac",
	}

	policyIR, errs := NewListenerPolicyIR(nil, nil, time.Now(), spec, objSrc)
	require.Empty(t, errs)
	require.NotNil(t, policyIR)

	// Verify no RBAC filter
	assert.Nil(t, policyIR.defaultPolicy.rbacNetworkFilter)

	// Create a mock listener
	listener := &envoylistenerv3.Listener{
		Name: "test-listener",
		FilterChains: []*envoylistenerv3.FilterChain{
			{
				Filters: []*envoylistenerv3.Filter{
					{Name: "envoy.filters.network.http_connection_manager"},
				},
			},
		},
	}

	// Create plugin pass and apply
	pass := &listenerPolicyPluginGwPass{
		rbacNetworkFilters: map[uint32]*anypb.Any{},
	}

	pCtx := &ir.ListenerContext{
		Policy: policyIR,
		Port:   8080,
	}

	pass.ApplyListenerPlugin(pCtx, listener)

	// Verify no RBAC filter is returned by NetworkFilters
	networkFilters, err := pass.NetworkFilters()
	require.NoError(t, err)
	require.Empty(t, networkFilters)

	// Verify no filter in map
	assert.Nil(t, pass.rbacNetworkFilters[8080])
}
