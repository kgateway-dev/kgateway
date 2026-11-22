package trafficpolicy

import (
	"slices"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoymatchingv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/matching/v3"
	csrfv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestCsrfIREquals(t *testing.T) {
	createSimpleCsrf := func(additionalOrigin string) *csrfv3.CsrfPolicy {
		return &csrfv3.CsrfPolicy{
			AdditionalOrigins: []*envoy_type_matcher_v3.StringMatcher{
				{
					MatchPattern: &envoy_type_matcher_v3.StringMatcher_Exact{
						Exact: additionalOrigin,
					},
				},
			},
			FilterEnabled: &envoycorev3.RuntimeFractionalPercent{
				DefaultValue: &typev3.FractionalPercent{
					Numerator:   100,
					Denominator: typev3.FractionalPercent_HUNDRED,
				},
			},
		}
	}
	createCsrfWithShadowEnabled := func(shadowEnabled bool) *csrfv3.CsrfPolicy {
		return &csrfv3.CsrfPolicy{
			ShadowEnabled: &envoycorev3.RuntimeFractionalPercent{
				DefaultValue: &typev3.FractionalPercent{
					Numerator: func() uint32 {
						if shadowEnabled {
							return 100
						}
						return 0
					}(),
					Denominator: typev3.FractionalPercent_HUNDRED,
				},
			},
		}
	}

	tests := []struct {
		name     string
		csrf1    *csrfIR
		csrf2    *csrfIR
		expected bool
	}{
		{
			name:     "both nil are equal",
			csrf1:    nil,
			csrf2:    nil,
			expected: true,
		},
		{
			name:     "nil vs non-nil are not equal",
			csrf1:    nil,
			csrf2:    &csrfIR{policy: createSimpleCsrf("https://example.com")},
			expected: false,
		},
		{
			name:     "non-nil vs nil are not equal",
			csrf1:    &csrfIR{policy: createSimpleCsrf("https://example.com")},
			csrf2:    nil,
			expected: false,
		},
		{
			name:     "same instance is equal",
			csrf1:    &csrfIR{policy: createSimpleCsrf("https://example.com")},
			csrf2:    &csrfIR{policy: createSimpleCsrf("https://example.com")},
			expected: true,
		},
		{
			name:     "different origins are not equal",
			csrf1:    &csrfIR{policy: createSimpleCsrf("https://example.com")},
			csrf2:    &csrfIR{policy: createSimpleCsrf("https://other.com")},
			expected: false,
		},
		{
			name:     "different shadow enabled settings are not equal",
			csrf1:    &csrfIR{policy: createCsrfWithShadowEnabled(true)},
			csrf2:    &csrfIR{policy: createCsrfWithShadowEnabled(false)},
			expected: false,
		},
		{
			name:     "same shadow enabled settings are equal",
			csrf1:    &csrfIR{policy: createCsrfWithShadowEnabled(true)},
			csrf2:    &csrfIR{policy: createCsrfWithShadowEnabled(true)},
			expected: true,
		},
		{
			name:     "nil csrf config fields are equal",
			csrf1:    &csrfIR{policy: nil},
			csrf2:    &csrfIR{policy: nil},
			expected: true,
		},
		{
			name:     "nil vs non-nil csrf config fields are not equal",
			csrf1:    &csrfIR{policy: nil},
			csrf2:    &csrfIR{policy: createSimpleCsrf("https://test.com")},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.csrf1.Equals(tt.csrf2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.csrf2.Equals(tt.csrf1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		csrf := &csrfIR{policy: createSimpleCsrf("https://test.com")}
		assert.True(t, csrf.Equals(csrf), "csrf should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		createSameCsrf := func() *csrfIR {
			return &csrfIR{policy: createCsrfWithShadowEnabled(false)}
		}

		a := createSameCsrf()
		b := createSameCsrf()
		c := createSameCsrf()

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}

func TestCsrfIRValidate(t *testing.T) {
	tests := []struct {
		name    string
		csrf    *csrfIR
		wantErr bool
	}{
		{
			name:    "nil csrf is valid",
			csrf:    nil,
			wantErr: false,
		},
		{
			name:    "csrf with nil config is valid",
			csrf:    &csrfIR{policy: nil},
			wantErr: false,
		},
		{
			name: "valid csrf config passes validation",
			csrf: &csrfIR{
				policy: &csrfv3.CsrfPolicy{
					FilterEnabled: &envoycorev3.RuntimeFractionalPercent{
						DefaultValue: &typev3.FractionalPercent{
							Numerator:   100,
							Denominator: typev3.FractionalPercent_HUNDRED,
						},
					},
					ShadowEnabled: &envoycorev3.RuntimeFractionalPercent{
						DefaultValue: &typev3.FractionalPercent{
							Numerator:   100,
							Denominator: typev3.FractionalPercent_HUNDRED,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty csrf config fails validation",
			csrf: &csrfIR{
				policy: &csrfv3.CsrfPolicy{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.csrf.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// The CSRF filter compares the request's Origin with its Host. A filter that can rewrite request
// headers must not run first, or a policy that merges into the route next to the CSRF policy (e.g.
// from a delegated child route) makes every request look same-origin.
func TestCsrfFilterRunsBeforeFiltersThatRewriteRequestHeaders(t *testing.T) {
	const filterChainName = "test-filter-chain"

	provider := &TrafficPolicyGatewayExtensionIR{
		Name:    "test-extension",
		ExtAuth: &envoy_ext_authz_v3.ExtAuthz{},
		ExtProc: &envoymatchingv3.ExtensionWithMatcher{},
		Jwt:     &envoymatchingv3.ExtensionWithMatcher{},
	}
	providers := func(stage filters.HTTPFilterStage) ProviderNeededMap {
		return ProviderNeededMap{
			Providers: map[string][]Provider{
				filterChainName: {{Name: provider.Name, Extension: provider, FilterStage: stage}},
			},
		}
	}
	plugin := &trafficPolicyPluginGwPass{
		setTransformationInChain: map[string]bool{
			filterChainName: true,
		},
		headerMutationInChain: map[string]*header_mutationv3.HeaderMutationPerRoute{
			filterChainName: {},
		},
		extAuthPerProvider: providers(filters.DuringStage(filters.AuthNStage)),
		extProcPerProvider: providers(defaultExtProcFilterStage),
		jwtPerProvider:     providers(filters.DuringStage(filters.AuthNStage)),
	}
	plugin.handleCsrf(filterChainName, &ir.TypedFilterConfigMap{}, &csrfIR{policy: &csrfv3.CsrfPolicy{}})

	httpFilters, err := plugin.HttpFilters(
		ir.HttpFiltersContext{},
		ir.FilterChainCommon{FilterChainName: filterChainName},
	)
	require.NoError(t, err)

	sortedFilters := filters.StagedHttpFilterList(httpFilters)
	sortedFilters.Sort()
	names := filterNames(sortedFilters)

	csrfIdx := slices.Index(names, csrfExtensionFilterName)
	require.NotEqual(t, -1, csrfIdx, "csrf missing from %v", names)
	for _, name := range []string{
		rustformationFilterNamePrefix,
		headerMutationFilterName,
		extAuthFilterName(provider.Name),
		extProcFilterName(provider.Name),
		jwtFilterName(provider.Name),
	} {
		idx := slices.Index(names, name)
		require.NotEqual(t, -1, idx, "%s missing from %v", name, names)
		assert.Less(t, csrfIdx, idx, "csrf must sort ahead of %s, got %v", name, names)
	}
}
