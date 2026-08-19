package trafficpolicy

import (
	"sort"
	"testing"

	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyapikeyauthv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/api_key_auth/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
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
			name:             "identical instance is equal",
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

			reverseResult := tt.headerModifiers2.Equals(tt.headerModifiers1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}
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

func TestConvertHeaderMutationFilterStage(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *shared.FilterStageSpec
		expected filters.FilterStage[filters.WellKnownFilterStage]
	}{
		{
			name:     "nil config returns default",
			cfg:      nil,
			expected: defaultHeaderMutationFilterStage,
		},
		{
			name:     "before AuthN",
			cfg:      &shared.FilterStageSpec{Stage: shared.FilterStageAuthN, Predicate: shared.FilterStagePredicateBefore},
			expected: filters.BeforeStage(filters.AuthNStage),
		},
		{
			name:     "during AuthZ",
			cfg:      &shared.FilterStageSpec{Stage: shared.FilterStageAuthZ, Predicate: shared.FilterStagePredicateDuring},
			expected: filters.DuringStage(filters.AuthZStage),
		},
		{
			name:     "after Fault",
			cfg:      &shared.FilterStageSpec{Stage: shared.FilterStageFault, Predicate: shared.FilterStagePredicateAfter},
			expected: filters.AfterStage(filters.FaultStage),
		},
		{
			name:     "empty predicate defaults to during",
			cfg:      &shared.FilterStageSpec{Stage: shared.FilterStageRateLimit},
			expected: filters.DuringStage(filters.RateLimitStage),
		},
		{
			name:     "unknown stage returns default",
			cfg:      &shared.FilterStageSpec{Stage: "Unknown", Predicate: shared.FilterStagePredicateBefore},
			expected: defaultHeaderMutationFilterStage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertHeaderMutationFilterStage(tt.cfg, defaultHeaderMutationFilterStage)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHttpFiltersHeaderModifiersFilterStage(t *testing.T) {
	t.Run("default stage matches current fixed behavior", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{
			headerMutationInChain: map[string]*header_mutationv3.HeaderMutationPerRoute{
				"test-filter-chain": {},
			},
		}
		fcc := ir.FilterChainCommon{FilterChainName: "test-filter-chain"}

		httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, fcc)

		require.NoError(t, err)
		require.Len(t, httpFilters, 1)
		assert.Equal(t, headerMutationFilterName, httpFilters[0].Filter.GetName())
		assert.Equal(t, filters.DuringStage(filters.RouteStage), httpFilters[0].Stage)
	})

	t.Run("explicit filterStage overrides the default", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{
			headerMutationInChain: map[string]*header_mutationv3.HeaderMutationPerRoute{
				"test-filter-chain": {},
			},
			headerMutationFilterStage: map[string]*shared.FilterStageSpec{
				"test-filter-chain": {Stage: shared.FilterStageAuthN, Predicate: shared.FilterStagePredicateBefore},
			},
		}
		fcc := ir.FilterChainCommon{FilterChainName: "test-filter-chain"}

		httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, fcc)

		require.NoError(t, err)
		require.Len(t, httpFilters, 1)
		assert.Equal(t, headerMutationFilterName, httpFilters[0].Filter.GetName())
		assert.Equal(t, filters.BeforeStage(filters.AuthNStage), httpFilters[0].Stage)
	})

	t.Run("mixed with apiKeyAuth: header mutation set Before AuthN sorts ahead of it", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{
			headerMutationInChain: map[string]*header_mutationv3.HeaderMutationPerRoute{
				"test-filter-chain": {},
			},
			headerMutationFilterStage: map[string]*shared.FilterStageSpec{
				"test-filter-chain": {Stage: shared.FilterStageAuthN, Predicate: shared.FilterStagePredicateBefore},
			},
			apiKeyAuthInChain: map[string]*envoyapikeyauthv3.ApiKeyAuth{
				"test-filter-chain": {},
			},
		}
		fcc := ir.FilterChainCommon{FilterChainName: "test-filter-chain"}

		httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, fcc)
		require.NoError(t, err)

		// HttpFilters appends in source-code order, not chain order - the actual chain
		// order is whatever the caller gets after sorting by Stage, so replicate that
		// sort here the same way the real filter chain is assembled.
		sort.SliceStable(httpFilters, func(i, j int) bool {
			return filters.FilterStageComparison(httpFilters[i].Stage, httpFilters[j].Stage) < 0
		})

		var names []string
		for _, f := range httpFilters {
			names = append(names, f.Filter.GetName())
		}
		headerMutIdx := indexOf(names, headerMutationFilterName)
		apiKeyAuthIdx := indexOf(names, apiKeyAuthFilterNamePrefix)
		require.GreaterOrEqual(t, headerMutIdx, 0)
		require.GreaterOrEqual(t, apiKeyAuthIdx, 0)
		assert.Less(t, headerMutIdx, apiKeyAuthIdx,
			"header mutation set Before AuthN must sort ahead of apiKeyAuth (During AuthN)")
	})

	t.Run("mixed with apiKeyAuth: default stage sorts after it", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{
			headerMutationInChain: map[string]*header_mutationv3.HeaderMutationPerRoute{
				"test-filter-chain": {},
			},
			apiKeyAuthInChain: map[string]*envoyapikeyauthv3.ApiKeyAuth{
				"test-filter-chain": {},
			},
		}
		fcc := ir.FilterChainCommon{FilterChainName: "test-filter-chain"}

		httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, fcc)
		require.NoError(t, err)

		sort.SliceStable(httpFilters, func(i, j int) bool {
			return filters.FilterStageComparison(httpFilters[i].Stage, httpFilters[j].Stage) < 0
		})

		var names []string
		for _, f := range httpFilters {
			names = append(names, f.Filter.GetName())
		}
		headerMutIdx := indexOf(names, headerMutationFilterName)
		apiKeyAuthIdx := indexOf(names, apiKeyAuthFilterNamePrefix)
		require.GreaterOrEqual(t, headerMutIdx, 0)
		require.GreaterOrEqual(t, apiKeyAuthIdx, 0)
		assert.Greater(t, headerMutIdx, apiKeyAuthIdx,
			"header mutation left at the default stage must still sort after apiKeyAuth (During AuthN)")
	})
}

func indexOf(s []string, v string) int {
	for i, e := range s {
		if e == v {
			return i
		}
	}
	return -1
}
