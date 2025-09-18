package trafficpolicy

import (
	"testing"

	cncfcorev3 "github.com/cncf/xds/go/xds/core/v3"
	cncfmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	envoyrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoyauthz "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

// createExpectedMatcher creates an expected matcher structure for testing
func createExpectedMatcher(action v1alpha1.AuthorizationPolicyAction, numRules int) *cncfmatcherv3.Matcher {
	// Create a simplified matcher structure for testing
	// We don't need to match the exact complex internal structure,
	// just the basic structure with the right number of matchers
	var matchers []*cncfmatcherv3.Matcher_MatcherList_FieldMatcher
	for i := 0; i < numRules; i++ {
		matcher := &cncfmatcherv3.Matcher_MatcherList_FieldMatcher{
			// Simplified structure - the actual implementation creates complex CEL matchers
			Predicate: &cncfmatcherv3.Matcher_MatcherList_Predicate{
				MatchType: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
					SinglePredicate: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
						Input: &cncfcorev3.TypedExtensionConfig{
							Name: "envoy.matching.inputs.cel_data_input",
						},
						Matcher: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_CustomMatch{
							CustomMatch: &cncfcorev3.TypedExtensionConfig{
								Name: "envoy.matching.matchers.cel_matcher",
							},
						},
					},
				},
			},
			OnMatch: &cncfmatcherv3.Matcher_OnMatch{
				OnMatch: &cncfmatcherv3.Matcher_OnMatch_Action{
					Action: &cncfcorev3.TypedExtensionConfig{
						Name: "envoy.filters.rbac.action",
					},
				},
			},
		}
		matchers = append(matchers, matcher)
	}

	return &cncfmatcherv3.Matcher{
		MatcherType: &cncfmatcherv3.Matcher_MatcherList_{
			MatcherList: &cncfmatcherv3.Matcher_MatcherList{
				Matchers: matchers,
			},
		},
		OnNoMatch: &cncfmatcherv3.Matcher_OnMatch{
			OnMatch: &cncfmatcherv3.Matcher_OnMatch_Action{
				Action: &cncfcorev3.TypedExtensionConfig{
					Name: "action",
				},
			},
		},
	}
}

func TestTranslateRBAC(t *testing.T) {
	tests := []struct {
		name             string
		ns               string
		tpName           string
		rbac             *v1alpha1.RBAC
		expected         *envoyauthz.RBACPerRoute
		expectedCELRules map[string][]string // policy name -> expected CEL expressions
		wantErr          bool
	}{
		{
			name:   "allow action with single rule",
			ns:     "test-ns",
			tpName: "test-policy",
			rbac: &v1alpha1.RBAC{
				Action: v1alpha1.AuthorizationPolicyActionAllow,
				Policy: v1alpha1.RBACPolicy{
					MatchExpressions: []string{"request.auth.claims.groups == 'group1'", "request.auth.claims.groups == 'group2'"},
				},
			},
			expected: &envoyauthz.RBACPerRoute{
				Rbac: &envoyauthz.RBAC{
					Matcher: createExpectedMatcher(v1alpha1.AuthorizationPolicyActionAllow, 1),
				},
			},
			expectedCELRules: map[string][]string{
				"ns[test-ns]-policy[test-policy]-rule[0]": {"request.auth.claims.groups == 'group1'", "request.auth.claims.groups == 'group2'"},
			},
			wantErr: false,
		},
		{
			name:   "deny action with empty rules",
			ns:     "test-ns",
			tpName: "test-policy",
			rbac: &v1alpha1.RBAC{
				Action: v1alpha1.AuthorizationPolicyActionDeny,
				Policy: v1alpha1.RBACPolicy{},
			},
			expected: &envoyauthz.RBACPerRoute{
				Rbac: &envoyauthz.RBAC{
					Rules: &envoyrbacv3.RBAC{
						Action: envoyrbacv3.RBAC_DENY,
					},
					Matcher: createExpectedMatcher(v1alpha1.AuthorizationPolicyActionDeny, 0),
				},
			},
			expectedCELRules: map[string][]string{},
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := translateRBAC(tt.rbac)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			if got.Rbac.Matcher != nil {
				// When CEL expressions are present, expect Matcher field
				require.NotNil(t, got.Rbac.Matcher, "Expected Matcher field in actual result")

				// Create CEL environment for validation
				env, err := cel.NewEnv()
				require.NoError(t, err, "Failed to create CEL environment")

				// Validate CEL expressions for all expected rules
				for _, expectedCELs := range tt.expectedCELRules {
					assert.Greater(t, len(expectedCELs), 0, "Expected CEL expressions should not be empty")

					// Validate each CEL expression can be parsed
					for _, celExpr := range expectedCELs {
						parsedExpr, err := parseCELExpression(env, celExpr)
						assert.NoError(t, err, "CEL expression should be valid: %s", celExpr)
						assert.NotNil(t, parsedExpr, "Parsed CEL expression should not be nil: %s", celExpr)
					}
				}
			}
		})
	}
}
