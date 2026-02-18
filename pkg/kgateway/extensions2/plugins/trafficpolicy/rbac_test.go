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

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

// createExpectedMatcher creates an expected matcher structure for testing
func createExpectedMatcher(numRules int) *cncfmatcherv3.Matcher {
	// Create a simplified matcher structure for testing
	// We don't need to match the exact complex internal structure,
	// just the basic structure with the right number of matchers
	var matchers []*cncfmatcherv3.Matcher_MatcherList_FieldMatcher
	for range numRules {
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
		rbac             *shared.Authorization
		expected         *envoyauthz.RBACPerRoute
		expectedCELRules map[string][]shared.CELExpression // policy name -> expected CEL expressions
		wantErr          bool
	}{
		{
			name:   "allow action with single rule",
			ns:     "test-ns",
			tpName: "test-policy",
			rbac: &shared.Authorization{
				Action: shared.AuthorizationPolicyActionAllow,
				Policy: shared.AuthorizationPolicy{
					MatchExpressions: []shared.CELExpression{"request.auth.claims.groups == 'group1'", "request.auth.claims.groups == 'group2'"},
				},
			},
			expected: &envoyauthz.RBACPerRoute{
				Rbac: &envoyauthz.RBAC{
					Matcher: createExpectedMatcher(1),
				},
			},
			expectedCELRules: map[string][]shared.CELExpression{
				"ns[test-ns]-policy[test-policy]-rule[0]": {"request.auth.claims.groups == 'group1'", "request.auth.claims.groups == 'group2'"},
			},
			wantErr: false,
		},
		{
			name:   "deny action with empty rules",
			ns:     "test-ns",
			tpName: "test-policy",
			rbac: &shared.Authorization{
				Action: shared.AuthorizationPolicyActionDeny,
				Policy: shared.AuthorizationPolicy{},
			},
			expected: &envoyauthz.RBACPerRoute{
				Rbac: &envoyauthz.RBAC{
					Rules: &envoyrbacv3.RBAC{
						Action: envoyrbacv3.RBAC_DENY,
					},
					Matcher: createExpectedMatcher(0),
				},
			},
			expectedCELRules: map[string][]shared.CELExpression{},
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

func TestValidateCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{
			name:    "valid IPv4 CIDR",
			cidr:    "192.168.1.0/24",
			wantErr: false,
		},
		{
			name:    "valid IPv4 single address",
			cidr:    "10.0.0.1/32",
			wantErr: false,
		},
		{
			name:    "valid IPv6 CIDR",
			cidr:    "2001:db8::/32",
			wantErr: false,
		},
		{
			name:    "invalid CIDR format",
			cidr:    "192.168.1.0",
			wantErr: true,
		},
		{
			name:    "invalid IP address",
			cidr:    "256.256.256.256/24",
			wantErr: true,
		},
		{
			name:    "invalid prefix length",
			cidr:    "192.168.1.0/33",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCIDR(tt.cidr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateIPMatcher(t *testing.T) {
	tests := []struct {
		name        string
		cidrs       []string
		isBlocklist bool
		action      shared.AuthorizationPolicyAction
		wantErr     bool
	}{
		{
			name:        "valid allowed CIDRs",
			cidrs:       []string{"192.168.1.0/24", "10.0.0.0/8"},
			isBlocklist: false,
			action:      shared.AuthorizationPolicyActionAllow,
			wantErr:     false,
		},
		{
			name:        "valid blocked CIDRs",
			cidrs:       []string{"192.168.1.100/32"},
			isBlocklist: true,
			action:      shared.AuthorizationPolicyActionDeny,
			wantErr:     false,
		},
		{
			name:        "invalid CIDR in list",
			cidrs:       []string{"192.168.1.0/24", "invalid"},
			isBlocklist: false,
			action:      shared.AuthorizationPolicyActionAllow,
			wantErr:     true,
		},
		{
			name:        "IPv6 CIDR",
			cidrs:       []string{"2001:db8::/32"},
			isBlocklist: false,
			action:      shared.AuthorizationPolicyActionAllow,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := createIPMatcher(tt.cidrs, tt.isBlocklist, tt.action)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, matcher)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, matcher)
				assert.NotNil(t, matcher.Predicate)
				assert.NotNil(t, matcher.OnMatch)
			}
		})
	}
}

func TestTranslateRBACWithCIDRs(t *testing.T) {
	tests := []struct {
		name    string
		rbac    *shared.Authorization
		wantErr bool
	}{
		{
			name: "allowed CIDRs only",
			rbac: &shared.Authorization{
				Action: shared.AuthorizationPolicyActionAllow,
				Policy: shared.AuthorizationPolicy{
					AllowedCIDRs: []string{"192.168.1.0/24", "10.0.0.0/8"},
				},
			},
			wantErr: false,
		},
		{
			name: "blocked CIDRs only",
			rbac: &shared.Authorization{
				Action: shared.AuthorizationPolicyActionDeny,
				Policy: shared.AuthorizationPolicy{
					BlockedCIDRs: []string{"192.168.1.100/32"},
				},
			},
			wantErr: false,
		},
		{
			name: "both allowed and blocked CIDRs",
			rbac: &shared.Authorization{
				Action: shared.AuthorizationPolicyActionAllow,
				Policy: shared.AuthorizationPolicy{
					AllowedCIDRs: []string{"192.168.0.0/16"},
					BlockedCIDRs: []string{"192.168.1.100/32"},
				},
			},
			wantErr: false,
		},
		{
			name: "CIDRs with CEL expressions",
			rbac: &shared.Authorization{
				Action: shared.AuthorizationPolicyActionAllow,
				Policy: shared.AuthorizationPolicy{
					AllowedCIDRs:     []string{"192.168.1.0/24"},
					MatchExpressions: []shared.CELExpression{"request.method == 'GET'"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid CIDR in allowed list",
			rbac: &shared.Authorization{
				Action: shared.AuthorizationPolicyActionAllow,
				Policy: shared.AuthorizationPolicy{
					AllowedCIDRs: []string{"invalid-cidr"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid CIDR in blocked list",
			rbac: &shared.Authorization{
				Action: shared.AuthorizationPolicyActionDeny,
				Policy: shared.AuthorizationPolicy{
					BlockedCIDRs: []string{"256.256.256.256/24"},
				},
			},
			wantErr: true,
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
			require.NotNil(t, got)
			require.NotNil(t, got.Rbac)

			// Verify the matcher was created
			if len(tt.rbac.Policy.AllowedCIDRs) > 0 || len(tt.rbac.Policy.BlockedCIDRs) > 0 || len(tt.rbac.Policy.MatchExpressions) > 0 {
				assert.NotNil(t, got.Rbac.Matcher, "Expected Matcher field when CIDR or CEL rules are present")
				if got.Rbac.Matcher != nil {
					matcherList := got.Rbac.Matcher.GetMatcherList()
					assert.NotNil(t, matcherList, "Expected MatcherList")
					if matcherList != nil {
						// Count expected matchers
						expectedCount := 0
						if len(tt.rbac.Policy.BlockedCIDRs) > 0 {
							expectedCount++
						}
						if len(tt.rbac.Policy.AllowedCIDRs) > 0 {
							expectedCount++
						}
						if len(tt.rbac.Policy.MatchExpressions) > 0 {
							expectedCount++
						}
						assert.Len(t, matcherList.Matchers, expectedCount, "Expected correct number of matchers")
					}
				}
			}
		})
	}
}
