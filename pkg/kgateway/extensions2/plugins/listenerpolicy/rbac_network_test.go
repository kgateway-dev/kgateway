package listenerpolicy

import (
	"testing"

	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoyrbacnetwork "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	"github.com/google/cel-go/cel"
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

func TestCreateNetworkCELMatcher_SingleExpression_Allow(t *testing.T) {
	celExprs := []sharedv1alpha1.CELExpression{
		`source.address.startsWith("10.0.0.")`,
	}

	matcher, err := createNetworkCELMatcher(celExprs, sharedv1alpha1.AuthorizationPolicyActionAllow)
	require.NoError(t, err)
	require.NotNil(t, matcher)

	assert.NotNil(t, matcher.Predicate)
	assert.NotNil(t, matcher.OnMatch)

	// Verify it's a single predicate (not OR)
	singlePred := matcher.Predicate.GetSinglePredicate()
	assert.NotNil(t, singlePred)
}

func TestCreateNetworkCELMatcher_MultipleExpressions_Deny(t *testing.T) {
	celExprs := []sharedv1alpha1.CELExpression{
		`source.address.startsWith("10.0.0.")`,
		`source.address.startsWith("192.168.0.")`,
	}

	matcher, err := createNetworkCELMatcher(celExprs, sharedv1alpha1.AuthorizationPolicyActionDeny)
	require.NoError(t, err)
	require.NotNil(t, matcher)

	assert.NotNil(t, matcher.Predicate)
	assert.NotNil(t, matcher.OnMatch)

	// Verify it's an OR matcher
	orMatcher := matcher.Predicate.GetOrMatcher()
	assert.NotNil(t, orMatcher)
	assert.Len(t, orMatcher.Predicate, 2)
}

func TestCreateNetworkCELMatcher_EmptyExpressions(t *testing.T) {
	celExprs := []sharedv1alpha1.CELExpression{}

	matcher, err := createNetworkCELMatcher(celExprs, sharedv1alpha1.AuthorizationPolicyActionAllow)
	assert.Error(t, err)
	assert.Nil(t, matcher)
	assert.Contains(t, err.Error(), "no CEL expressions provided")
}

func TestCreateNetworkCELMatcher_InvalidExpression(t *testing.T) {
	celExprs := []sharedv1alpha1.CELExpression{
		`this is not valid CEL!!!`,
	}

	matcher, err := createNetworkCELMatcher(celExprs, sharedv1alpha1.AuthorizationPolicyActionAllow)
	assert.Error(t, err)
	assert.Nil(t, matcher)
}

func TestCreateMatchAction_Allow(t *testing.T) {
	action := createMatchAction(envoyrbacv3.RBAC_ALLOW)
	require.NotNil(t, action)

	actionConfig := action.GetAction()
	require.NotNil(t, actionConfig)
	assert.Equal(t, "envoy.filters.rbac.action", actionConfig.Name)

	// Unmarshal to verify it's an allow action
	var rbacAction envoyrbacv3.Action
	err := actionConfig.TypedConfig.UnmarshalTo(&rbacAction)
	require.NoError(t, err)
	assert.Equal(t, "allow-request", rbacAction.Name)
	assert.Equal(t, envoyrbacv3.RBAC_ALLOW, rbacAction.Action)
}

func TestCreateMatchAction_Deny(t *testing.T) {
	action := createMatchAction(envoyrbacv3.RBAC_DENY)
	require.NotNil(t, action)

	actionConfig := action.GetAction()
	require.NotNil(t, actionConfig)

	var rbacAction envoyrbacv3.Action
	err := actionConfig.TypedConfig.UnmarshalTo(&rbacAction)
	require.NoError(t, err)
	assert.Equal(t, "deny-request", rbacAction.Name)
	assert.Equal(t, envoyrbacv3.RBAC_DENY, rbacAction.Action)
}

func TestCreateDefaultAction(t *testing.T) {
	action := createDefaultAction(envoyrbacv3.RBAC_DENY)
	require.NotNil(t, action)

	actionConfig := action.GetAction()
	require.NotNil(t, actionConfig)
	assert.Equal(t, "action", actionConfig.Name)

	var rbacAction envoyrbacv3.Action
	err := actionConfig.TypedConfig.UnmarshalTo(&rbacAction)
	require.NoError(t, err)
	assert.Equal(t, envoyrbacv3.RBAC_DENY, rbacAction.Action)
}

func TestParseCELExpression_Valid(t *testing.T) {
	// Note: This test requires a CEL environment
	// We'll test with a simple valid expression
	testCases := []struct {
		name string
		expr sharedv1alpha1.CELExpression
	}{
		{
			name: "simple string comparison",
			expr: `source.address == "10.0.0.1"`,
		},
		{
			name: "startsWith function",
			expr: `source.address.startsWith("10.0.0.")`,
		},
		{
			name: "boolean expression",
			expr: `true`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a basic CEL environment
			env, err := createTestCELEnv()
			require.NoError(t, err)

			parsed, err := parseCELExpression(env, tc.expr)
			assert.NoError(t, err)
			assert.NotNil(t, parsed)
		})
	}
}

func TestParseCELExpression_Invalid(t *testing.T) {
	env, err := createTestCELEnv()
	require.NoError(t, err)

	invalidExprs := []sharedv1alpha1.CELExpression{
		`this is not valid CEL`,
		`source.address.invalidFunction()`,
		`unclosed string literal"`,
	}

	for _, expr := range invalidExprs {
		parsed, err := parseCELExpression(env, expr)
		assert.Error(t, err)
		assert.Nil(t, parsed)
	}
}

func TestParseCELExpression_NilEnv(t *testing.T) {
	parsed, err := parseCELExpression(nil, `source.address == "10.0.0.1"`)
	assert.Error(t, err)
	assert.Nil(t, parsed)
	assert.Contains(t, err.Error(), "CEL environment is nil")
}

// Helper function to create a test CEL environment
func createTestCELEnv() (*cel.Env, error) {
	return cel.NewEnv()
}

// Integration test: Full translation pipeline
func TestNetworkRbacIntegration(t *testing.T) {
	testCases := []struct {
		name      string
		rbac      *sharedv1alpha1.Authorization
		objSrc    ir.ObjectSource
		wantError bool
	}{
		{
			name: "corporate network allow",
			rbac: &sharedv1alpha1.Authorization{
				Policy: sharedv1alpha1.AuthorizationPolicy{
					MatchExpressions: []sharedv1alpha1.CELExpression{
						`source.address.startsWith("10.0.0.")`,
						`source.address.startsWith("192.168.0.")`,
					},
				},
				Action: sharedv1alpha1.AuthorizationPolicyActionAllow,
			},
			objSrc: ir.ObjectSource{
				Namespace: "prod",
				Name:      "corporate-network",
			},
			wantError: false,
		},
		{
			name: "deny specific IPs",
			rbac: &sharedv1alpha1.Authorization{
				Policy: sharedv1alpha1.AuthorizationPolicy{
					MatchExpressions: []sharedv1alpha1.CELExpression{
						`source.address.startsWith("192.0.2.")`,
					},
				},
				Action: sharedv1alpha1.AuthorizationPolicyActionDeny,
			},
			objSrc: ir.ObjectSource{
				Namespace: "security",
				Name:      "blocklist",
			},
			wantError: false,
		},
		{
			name: "nil rbac",
			rbac: nil,
			objSrc: ir.ObjectSource{
				Namespace: "test",
				Name:      "test",
			},
			wantError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := translateNetworkRbac(tc.rbac, tc.objSrc)

			if tc.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tc.rbac == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)

				// Verify it's a valid protobuf message
				var rbacConfig envoyrbacnetwork.RBAC
				err = result.UnmarshalTo(&rbacConfig)
				require.NoError(t, err)

				// Verify stat prefix
				expectedPrefix := tc.objSrc.Namespace + "_" + tc.objSrc.Name + "_network_rbac"
				assert.Equal(t, expectedPrefix, rbacConfig.StatPrefix)
			}
		})
	}
}
