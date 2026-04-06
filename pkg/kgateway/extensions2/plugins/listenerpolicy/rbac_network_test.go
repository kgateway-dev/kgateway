package listenerpolicy

import (
	"testing"

	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoyrbacnetwork "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestTranslateNetworkRbac_Nil(t *testing.T) {
	result, err := translateNetworkRbac(nil, ir.ObjectSource{})
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestTranslateNetworkRbac_EmptyMatchExpressions(t *testing.T) {
	rbac := &sharedv1alpha1.Authorization{
		Policy: sharedv1alpha1.AuthorizationPolicy{
			MatchExpressions: []sharedv1alpha1.CELExpression{},
		},
		Action: sharedv1alpha1.AuthorizationPolicyActionAllow,
	}

	objSrc := ir.ObjectSource{
		Namespace: "test-ns",
		Name:      "test-policy",
	}

	result, err := translateNetworkRbac(rbac, objSrc)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Unmarshal to verify it's a valid RBAC config
	var rbacConfig envoyrbacnetwork.RBAC
	err = result.UnmarshalTo(&rbacConfig)
	require.NoError(t, err)

	// Should have deny-all rules when no matchers
	assert.Equal(t, "test-ns_test-policy_network_rbac", rbacConfig.StatPrefix)
	assert.NotNil(t, rbacConfig.Rules)
	assert.Equal(t, envoyrbacv3.RBAC_DENY, rbacConfig.Rules.Action)
}

func TestTranslateNetworkRbac_SingleExpression_Allow(t *testing.T) {
	rbac := &sharedv1alpha1.Authorization{
		Policy: sharedv1alpha1.AuthorizationPolicy{
			MatchExpressions: []sharedv1alpha1.CELExpression{
				`source.address.startsWith("10.0.0.")`,
			},
		},
		Action: sharedv1alpha1.AuthorizationPolicyActionAllow,
	}

	objSrc := ir.ObjectSource{
		Namespace: "test-ns",
		Name:      "test-policy",
	}

	result, err := translateNetworkRbac(rbac, objSrc)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Unmarshal to verify structure
	var rbacConfig envoyrbacnetwork.RBAC
	err = result.UnmarshalTo(&rbacConfig)
	require.NoError(t, err)

	assert.Equal(t, "test-ns_test-policy_network_rbac", rbacConfig.StatPrefix)
	assert.NotNil(t, rbacConfig.Matcher)
	assert.NotNil(t, rbacConfig.Matcher.OnNoMatch)
}

func TestTranslateNetworkRbac_SingleExpression_Deny(t *testing.T) {
	rbac := &sharedv1alpha1.Authorization{
		Policy: sharedv1alpha1.AuthorizationPolicy{
			MatchExpressions: []sharedv1alpha1.CELExpression{
				`source.address.startsWith("192.168.0.")`,
			},
		},
		Action: sharedv1alpha1.AuthorizationPolicyActionDeny,
	}

	objSrc := ir.ObjectSource{
		Namespace: "prod-ns",
		Name:      "deny-policy",
	}

	result, err := translateNetworkRbac(rbac, objSrc)
	require.NoError(t, err)
	require.NotNil(t, result)

	var rbacConfig envoyrbacnetwork.RBAC
	err = result.UnmarshalTo(&rbacConfig)
	require.NoError(t, err)

	assert.Equal(t, "prod-ns_deny-policy_network_rbac", rbacConfig.StatPrefix)
	assert.NotNil(t, rbacConfig.Matcher)
}

func TestTranslateNetworkRbac_MultipleExpressions(t *testing.T) {
	rbac := &sharedv1alpha1.Authorization{
		Policy: sharedv1alpha1.AuthorizationPolicy{
			MatchExpressions: []sharedv1alpha1.CELExpression{
				`source.address.startsWith("10.0.0.")`,
				`source.address.startsWith("192.168.0.")`,
				`source.address.startsWith("172.16.0.")`,
			},
		},
		Action: sharedv1alpha1.AuthorizationPolicyActionAllow,
	}

	objSrc := ir.ObjectSource{
		Namespace: "multi-ns",
		Name:      "multi-policy",
	}

	result, err := translateNetworkRbac(rbac, objSrc)
	require.NoError(t, err)
	require.NotNil(t, result)

	var rbacConfig envoyrbacnetwork.RBAC
	err = result.UnmarshalTo(&rbacConfig)
	require.NoError(t, err)

	assert.Equal(t, "multi-ns_multi-policy_network_rbac", rbacConfig.StatPrefix)
	assert.NotNil(t, rbacConfig.Matcher)

	// Verify matcher list structure
	matcherList := rbacConfig.Matcher.GetMatcherList()
	require.NotNil(t, matcherList)
	assert.Len(t, matcherList.Matchers, 1) // One field matcher containing OR predicate
}

func TestTranslateNetworkRbac_InvalidCELExpression(t *testing.T) {
	rbac := &sharedv1alpha1.Authorization{
		Policy: sharedv1alpha1.AuthorizationPolicy{
			MatchExpressions: []sharedv1alpha1.CELExpression{
				`invalid CEL syntax!!!`,
			},
		},
		Action: sharedv1alpha1.AuthorizationPolicyActionAllow,
	}

	objSrc := ir.ObjectSource{
		Namespace: "test-ns",
		Name:      "invalid-policy",
	}

	_, err := translateNetworkRbac(rbac, objSrc)

	// Should return error but might still return partial config
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CEL matcher errors")
}
