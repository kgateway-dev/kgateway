package plugins

import (
	"testing"

	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

func TestCreateCELMatcher(t *testing.T) {
	tests := []struct {
		name                string
		celExprs            []sharedv1alpha1.CELExpression
		action              sharedv1alpha1.AuthorizationPolicyAction
		expectError         bool
		expectSinglePred    bool
		expectOrMatcher     bool
		expectedPredicates  int
		expectedErrorString string
	}{
		{
			name: "single expression with allow action",
			celExprs: []sharedv1alpha1.CELExpression{
				`source.address.startsWith("10.0.0.")`,
			},
			action:           sharedv1alpha1.AuthorizationPolicyActionAllow,
			expectError:      false,
			expectSinglePred: true,
		},
		{
			name: "multiple expressions with deny action",
			celExprs: []sharedv1alpha1.CELExpression{
				`source.address.startsWith("10.0.0.")`,
				`source.address.startsWith("192.168.0.")`,
			},
			action:             sharedv1alpha1.AuthorizationPolicyActionDeny,
			expectError:        false,
			expectOrMatcher:    true,
			expectedPredicates: 2,
		},
		{
			name:                "empty expressions",
			celExprs:            []sharedv1alpha1.CELExpression{},
			action:              sharedv1alpha1.AuthorizationPolicyActionAllow,
			expectError:         true,
			expectedErrorString: "no CEL expressions provided",
		},
		{
			name: "invalid CEL expression",
			celExprs: []sharedv1alpha1.CELExpression{
				`this is not valid CEL!!!`,
			},
			action:      sharedv1alpha1.AuthorizationPolicyActionAllow,
			expectError: true,
		},
		{
			name: "multiple expressions with allow action",
			celExprs: []sharedv1alpha1.CELExpression{
				`request.auth.claims.groups == 'admin'`,
				`request.auth.claims.groups == 'developer'`,
			},
			action:             sharedv1alpha1.AuthorizationPolicyActionAllow,
			expectError:        false,
			expectOrMatcher:    true,
			expectedPredicates: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := CreateCELMatcher(tt.celExprs, tt.action)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, matcher)
				if tt.expectedErrorString != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorString)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, matcher)
			assert.NotNil(t, matcher.Predicate)
			assert.NotNil(t, matcher.OnMatch)

			if tt.expectSinglePred {
				singlePred := matcher.Predicate.GetSinglePredicate()
				assert.NotNil(t, singlePred, "Expected single predicate")
			}

			if tt.expectOrMatcher {
				orMatcher := matcher.Predicate.GetOrMatcher()
				assert.NotNil(t, orMatcher, "Expected OR matcher")
				if tt.expectedPredicates > 0 {
					assert.Len(t, orMatcher.Predicate, tt.expectedPredicates)
				}
			}
		})
	}
}

func TestCreateMatchAction(t *testing.T) {
	tests := []struct {
		name               string
		action             envoyrbacv3.RBAC_Action
		expectedName       string
		expectedConfigName string
	}{
		{
			name:               "allow action",
			action:             envoyrbacv3.RBAC_ALLOW,
			expectedName:       "allow-request",
			expectedConfigName: "envoy.filters.rbac.action",
		},
		{
			name:               "deny action",
			action:             envoyrbacv3.RBAC_DENY,
			expectedName:       "deny-request",
			expectedConfigName: "envoy.filters.rbac.action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := CreateMatchAction(tt.action)
			require.NotNil(t, action)

			actionConfig := action.GetAction()
			require.NotNil(t, actionConfig)
			assert.Equal(t, tt.expectedConfigName, actionConfig.Name)

			var rbacAction envoyrbacv3.Action
			err := actionConfig.TypedConfig.UnmarshalTo(&rbacAction)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedName, rbacAction.Name)
			assert.Equal(t, tt.action, rbacAction.Action)
		})
	}
}

func TestCreateDefaultAction(t *testing.T) {
	tests := []struct {
		name               string
		action             envoyrbacv3.RBAC_Action
		expectedName       string
		expectedConfigName string
	}{
		{
			name:               "allow action",
			action:             envoyrbacv3.RBAC_ALLOW,
			expectedName:       "allow-request",
			expectedConfigName: "action",
		},
		{
			name:               "deny action",
			action:             envoyrbacv3.RBAC_DENY,
			expectedName:       "deny-request",
			expectedConfigName: "action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := CreateDefaultAction(tt.action)
			require.NotNil(t, action)

			actionConfig := action.GetAction()
			require.NotNil(t, actionConfig)
			assert.Equal(t, tt.expectedConfigName, actionConfig.Name)

			var rbacAction envoyrbacv3.Action
			err := actionConfig.TypedConfig.UnmarshalTo(&rbacAction)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedName, rbacAction.Name)
			assert.Equal(t, tt.action, rbacAction.Action)
		})
	}
}

func TestParseCELExpression(t *testing.T) {
	tests := []struct {
		name        string
		expr        sharedv1alpha1.CELExpression
		nilEnv      bool
		expectError bool
		errorString string
	}{
		{
			name:        "simple string comparison",
			expr:        `source.address == "10.0.0.1"`,
			expectError: false,
		},
		{
			name:        "startsWith function",
			expr:        `source.address.startsWith("10.0.0.")`,
			expectError: false,
		},
		{
			name:        "boolean expression",
			expr:        `true`,
			expectError: false,
		},
		{
			name:        "request claims",
			expr:        `request.auth.claims.groups == 'admin'`,
			expectError: false,
		},
		{
			name:        "invalid CEL syntax",
			expr:        `this is not valid CEL`,
			expectError: true,
		},
		{
			name:        "incomplete expression",
			expr:        `source.address ==`,
			expectError: true,
		},
		{
			name:        "unclosed string literal",
			expr:        `unclosed string literal"`,
			expectError: true,
		},
		{
			name:        "nil environment",
			expr:        `source.address == "10.0.0.1"`,
			nilEnv:      true,
			expectError: true,
			errorString: "CEL environment is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var env *cel.Env
			var err error

			if !tt.nilEnv {
				env, err = cel.NewEnv()
				require.NoError(t, err, "Failed to create CEL environment")
			}

			parsed, err := ParseCELExpression(env, tt.expr)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, parsed)
				if tt.errorString != "" {
					assert.Contains(t, err.Error(), tt.errorString)
				}
				return
			}

			assert.NoError(t, err, "CEL expression should be valid: %s", tt.expr)
			assert.NotNil(t, parsed, "Parsed CEL expression should not be nil: %s", tt.expr)
		})
	}
}
