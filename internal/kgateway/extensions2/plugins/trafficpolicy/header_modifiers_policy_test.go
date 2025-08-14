package trafficpolicy

import (
	"context"
	"testing"

	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
)

// Helper to create a simple header mutations filter for testing.
func testHeaderMutation(isAppend bool) *header_mutationv3.HeaderMutationPerRoute {
	appendAction := envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD
	if isAppend {
		appendAction = envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD
	}

	return &header_mutationv3.HeaderMutationPerRoute{
		Mutations: &header_mutationv3.Mutations{
			RequestMutations: []*mutation_rulesv3.HeaderMutation{{
				Action: &mutation_rulesv3.HeaderMutation_Append{
					Append: &envoycorev3.HeaderValueOption{
						Header: &envoycorev3.HeaderValue{
							Key:   "x-test-request",
							Value: "test-request",
						},
						AppendAction: appendAction,
					},
				},
			}},
			ResponseMutations: []*mutation_rulesv3.HeaderMutation{{
				Action: &mutation_rulesv3.HeaderMutation_Append{
					Append: &envoycorev3.HeaderValueOption{
						Header: &envoycorev3.HeaderValue{
							Key:   "x-test-response",
							Value: "test-response",
						},
						AppendAction: appendAction,
					},
				},
			}},
		},
	}
}

func TestHeaderModifiersIREquals(t *testing.T) {
	tests := []struct {
		name             string
		headerModifiers1 *headerModifiersIR
		headerModifiers2 *headerModifiersIR
		expected         bool
	}{
		{
			name:             "both nil are equal",
			headerModifiers1: nil,
			headerModifiers2: nil,
			expected:         true,
		},
		{
			name:             "nil vs non-nil are not equal",
			headerModifiers1: nil,
			headerModifiers2: &headerModifiersIR{policy: testHeaderMutation(false)},
			expected:         false,
		},
		{
			name:             "non-nil vs nil are not equal",
			headerModifiers1: &headerModifiersIR{policy: testHeaderMutation(false)},
			headerModifiers2: nil,
			expected:         false,
		},
		{
			name:             "same instance is equal",
			headerModifiers1: &headerModifiersIR{policy: testHeaderMutation(false)},
			headerModifiers2: &headerModifiersIR{policy: testHeaderMutation(false)},
			expected:         true,
		},
		{
			name:             "different append settings are not equal",
			headerModifiers1: &headerModifiersIR{policy: testHeaderMutation(true)},
			headerModifiers2: &headerModifiersIR{policy: testHeaderMutation(false)},
			expected:         false,
		},
		{
			name:             "nil HeaderModifiers fields are equal",
			headerModifiers1: &headerModifiersIR{policy: nil},
			headerModifiers2: &headerModifiersIR{policy: nil},
			expected:         true,
		},
		{
			name:             "nil vs non-nil HeaderModifiers fields are not equal",
			headerModifiers1: &headerModifiersIR{policy: nil},
			headerModifiers2: &headerModifiersIR{policy: testHeaderMutation(false)},
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.headerModifiers1.Equals(tt.headerModifiers2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.headerModifiers2.Equals(tt.headerModifiers1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values.
	t.Run("reflexivity", func(t *testing.T) {
		HeaderModifiers := &headerModifiersIR{policy: testHeaderMutation(false)}
		assert.True(t, HeaderModifiers.Equals(HeaderModifiers), "HeaderModifiers should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c).
	t.Run("transitivity", func(t *testing.T) {
		createSameHeaderModifiers := func() *headerModifiersIR {
			return &headerModifiersIR{policy: testHeaderMutation(true)}
		}

		a := createSameHeaderModifiers()
		b := createSameHeaderModifiers()
		c := createSameHeaderModifiers()

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}

func TestHeaderModifiersIRValidate(t *testing.T) {
	tests := []struct {
		name            string
		headerModifiers *headerModifiersIR
		wantErr         bool
	}{
		{
			name:            "nil headerModifiers is valid",
			headerModifiers: nil,
			wantErr:         false,
		},
		{
			name:            "headerModifiers with nil config is valid",
			headerModifiers: &headerModifiersIR{policy: nil},
			wantErr:         false,
		},
		{
			name: "valid headerModifiers config passes validation",
			headerModifiers: &headerModifiersIR{
				policy: testHeaderMutation(false),
			},
			wantErr: false,
		},
		{
			name: "invalid headerModifiers config fails validation",
			headerModifiers: &headerModifiersIR{
				policy: &header_mutationv3.HeaderMutationPerRoute{
					Mutations: &header_mutationv3.Mutations{
						RequestMutations: []*mutation_rulesv3.HeaderMutation{{
							Action: &mutation_rulesv3.HeaderMutation_Append{
								Append: &envoycorev3.HeaderValueOption{},
							},
						}},
						ResponseMutations: []*mutation_rulesv3.HeaderMutation{},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.headerModifiers.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHeaderModifiersApplyForRoute(t *testing.T) {
	t.Run("applies header modifiers configuration to route", func(t *testing.T) {
		// Setup.
		plugin := &trafficPolicyPluginGwPass{}

		ctx := context.Background()

		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				headerModifiers: &headerModifiersIR{
					policy: testHeaderMutation(false),
				},
			},
		}

		pCtx := &ir.RouteContext{
			Policy: policy,
		}

		outputRoute := &envoyroutev3.Route{}

		// Execute.
		err := plugin.ApplyForRoute(ctx, pCtx, outputRoute)

		// Verify.
		require.NoError(t, err)
		require.NotNil(t, pCtx.TypedFilterConfig)
		headerModifiersConfig, ok := pCtx.TypedFilterConfig[headerMutationFilterName]
		assert.True(t, ok)
		assert.NotNil(t, headerModifiersConfig)
	})

	t.Run("handles nil header modifiers configuration", func(t *testing.T) {
		// Setup.
		plugin := &trafficPolicyPluginGwPass{}

		ctx := context.Background()

		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				headerModifiers: nil,
			},
		}

		pCtx := &ir.RouteContext{
			Policy: policy,
		}

		outputRoute := &envoyroutev3.Route{}

		// Execute.
		err := plugin.ApplyForRoute(ctx, pCtx, outputRoute)

		// Verify.
		require.NoError(t, err)
		assert.Nil(t, pCtx.TypedFilterConfig)
	})
}

func TestHeaderModifiersHttpFilters(t *testing.T) {
	t.Run("adds header modifiers filter to filter chain", func(t *testing.T) {
		// Setup.
		plugin := &trafficPolicyPluginGwPass{
			headerMutationInChain: map[string]*header_mutationv3.HeaderMutationPerRoute{
				"test-filter-chain": testHeaderMutation(false),
			},
		}

		ctx := context.Background()

		fcc := ir.FilterChainCommon{
			FilterChainName: "test-filter-chain",
		}

		// Execute.
		filters, err := plugin.HttpFilters(ctx, fcc)

		// Verify.
		require.NoError(t, err)
		require.NotNil(t, filters)
		assert.Equal(t, 1, len(filters))
		assert.Equal(t, plugins.DuringStage(plugins.RouteStage), filters[0].Stage)
	})
}

func TestHeaderModifiersPolicyPlugin(t *testing.T) {
	t.Run("applies header modifiers configuration to route", func(t *testing.T) {
		// Setup.
		plugin := &trafficPolicyPluginGwPass{}

		ctx := context.Background()

		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				headerModifiers: &headerModifiersIR{
					policy: testHeaderMutation(false),
				},
			},
		}

		pCtx := &ir.RouteContext{
			Policy: policy,
		}

		outputRoute := &envoyroutev3.Route{}

		// Execute.
		err := plugin.ApplyForRoute(ctx, pCtx, outputRoute)

		// Verify.
		require.NoError(t, err)
		require.NotNil(t, pCtx.TypedFilterConfig)

		headerModifiersConfig, ok := pCtx.TypedFilterConfig[headerMutationFilterName]
		assert.True(t, ok)
		assert.NotNil(t, headerModifiersConfig)
	})
}
