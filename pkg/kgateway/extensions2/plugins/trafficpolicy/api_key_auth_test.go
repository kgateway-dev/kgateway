package trafficpolicy

import (
	"errors"
	"fmt"
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyapikeyauthv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/api_key_auth/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestAPIKeyAuthIREquals(t *testing.T) {
	// Helper to create simple API key auth configurations for testing
	createAPIKeyAuth := func(headerName string, hideCredentials bool) *envoyapikeyauthv3.ApiKeyAuthPerRoute {
		return &envoyapikeyauthv3.ApiKeyAuthPerRoute{
			Credentials: []*envoyapikeyauthv3.Credential{
				{
					Key:    "test-key",
					Client: "test-client",
				},
			},
			KeySources: []*envoyapikeyauthv3.KeySource{
				{
					Header: headerName,
				},
			},
			Forwarding: &envoyapikeyauthv3.Forwarding{
				Header:          "x-client-id",
				HideCredentials: hideCredentials,
			},
		}
	}

	tests := []struct {
		name        string
		apiKeyAuth1 *apiKeyAuthIR
		apiKeyAuth2 *apiKeyAuthIR
		expected    bool
	}{
		{
			name:        "both nil are equal",
			apiKeyAuth1: nil,
			apiKeyAuth2: nil,
			expected:    true,
		},
		{
			name:        "nil vs non-nil are not equal",
			apiKeyAuth1: nil,
			apiKeyAuth2: &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)},
			expected:    false,
		},
		{
			name:        "non-nil vs nil are not equal",
			apiKeyAuth1: &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)},
			apiKeyAuth2: nil,
			expected:    false,
		},
		{
			name:        "both policy nil are equal",
			apiKeyAuth1: &apiKeyAuthIR{config: nil},
			apiKeyAuth2: &apiKeyAuthIR{config: nil},
			expected:    true,
		},
		{
			name:        "same configuration is equal",
			apiKeyAuth1: &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)},
			apiKeyAuth2: &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)},
			expected:    true,
		},
		{
			name:        "different header names are not equal",
			apiKeyAuth1: &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)},
			apiKeyAuth2: &apiKeyAuthIR{config: createAPIKeyAuth("x-api-key", false)},
			expected:    false,
		},
		{
			name:        "different hide credentials settings are not equal",
			apiKeyAuth1: &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)},
			apiKeyAuth2: &apiKeyAuthIR{config: createAPIKeyAuth("api-key", true)},
			expected:    false,
		},
		{
			name: "different credentials are not equal",
			apiKeyAuth1: &apiKeyAuthIR{
				config: &envoyapikeyauthv3.ApiKeyAuthPerRoute{
					Credentials: []*envoyapikeyauthv3.Credential{
						{Key: "key1", Client: "client1"},
					},
				},
			},
			apiKeyAuth2: &apiKeyAuthIR{
				config: &envoyapikeyauthv3.ApiKeyAuthPerRoute{
					Credentials: []*envoyapikeyauthv3.Credential{
						{Key: "key2", Client: "client2"},
					},
				},
			},
			expected: false,
		},
		{
			name: "same credentials are equal",
			apiKeyAuth1: &apiKeyAuthIR{
				config: &envoyapikeyauthv3.ApiKeyAuthPerRoute{
					Credentials: []*envoyapikeyauthv3.Credential{
						{Key: "key1", Client: "client1"},
					},
				},
			},
			apiKeyAuth2: &apiKeyAuthIR{
				config: &envoyapikeyauthv3.ApiKeyAuthPerRoute{
					Credentials: []*envoyapikeyauthv3.Credential{
						{Key: "key1", Client: "client1"},
					},
				},
			},
			expected: true,
		},
		{
			name:        "both disabled are equal",
			apiKeyAuth1: &apiKeyAuthIR{disable: true},
			apiKeyAuth2: &apiKeyAuthIR{disable: true},
			expected:    true,
		},
		{
			name:        "disabled vs enabled are not equal",
			apiKeyAuth1: &apiKeyAuthIR{disable: true},
			apiKeyAuth2: &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)},
			expected:    false,
		},
		{
			name:        "enabled vs disabled are not equal",
			apiKeyAuth1: &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)},
			apiKeyAuth2: &apiKeyAuthIR{disable: true},
			expected:    false,
		},
		{
			name:        "disabled with config vs disabled without config are not equal",
			apiKeyAuth1: &apiKeyAuthIR{disable: true, config: createAPIKeyAuth("api-key", false)},
			apiKeyAuth2: &apiKeyAuthIR{disable: true},
			expected:    false,
		},
		{
			name:        "disabled with same config are equal",
			apiKeyAuth1: &apiKeyAuthIR{disable: true, config: createAPIKeyAuth("api-key", false)},
			apiKeyAuth2: &apiKeyAuthIR{disable: true, config: createAPIKeyAuth("api-key", false)},
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.apiKeyAuth1.Equals(tt.apiKeyAuth2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.apiKeyAuth2.Equals(tt.apiKeyAuth1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		apiKeyAuth := &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)}
		assert.True(t, apiKeyAuth.Equals(apiKeyAuth), "apiKeyAuth should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		a := &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)}
		b := &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)}
		c := &apiKeyAuthIR{config: createAPIKeyAuth("api-key", false)}

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}

func TestAPIKeyAuthIRValidate(t *testing.T) {
	tests := []struct {
		name       string
		apiKeyAuth *apiKeyAuthIR
		wantErr    bool
	}{
		{
			name:       "nil IR validates successfully",
			apiKeyAuth: nil,
			wantErr:    false,
		},
		{
			name:       "nil policy validates successfully",
			apiKeyAuth: &apiKeyAuthIR{config: nil},
			wantErr:    false,
		},
		{
			name: "valid policy validates successfully",
			apiKeyAuth: &apiKeyAuthIR{
				config: &envoyapikeyauthv3.ApiKeyAuthPerRoute{
					Credentials: []*envoyapikeyauthv3.Credential{
						{
							Key:    "test-key",
							Client: "test-client",
						},
					},
					KeySources: []*envoyapikeyauthv3.KeySource{
						{
							Header: "api-key",
						},
					},
					Forwarding: &envoyapikeyauthv3.Forwarding{
						Header:          "x-client-id",
						HideCredentials: false,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "policy with empty credentials validates successfully",
			apiKeyAuth: &apiKeyAuthIR{
				config: &envoyapikeyauthv3.ApiKeyAuthPerRoute{
					Credentials: []*envoyapikeyauthv3.Credential{},
					KeySources: []*envoyapikeyauthv3.KeySource{
						{
							Header: "api-key",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "policy with no client ID header forwarding validates successfully",
			apiKeyAuth: &apiKeyAuthIR{
				config: &envoyapikeyauthv3.ApiKeyAuthPerRoute{
					Credentials: []*envoyapikeyauthv3.Credential{
						{
							Key:    "test-key",
							Client: "test-client",
						},
					},
					KeySources: []*envoyapikeyauthv3.KeySource{
						{
							Header: "api-key",
						},
					},
					Forwarding: &envoyapikeyauthv3.Forwarding{
						HideCredentials: false,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.apiKeyAuth.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandleAPIKeyAuth(t *testing.T) {
	tests := []struct {
		name           string
		apiKeyAuthIr   *apiKeyAuthIR
		expectChain    bool
		expectRoute    bool
		expectDisabled bool
	}{
		{
			name:           "nil IR does nothing",
			apiKeyAuthIr:   nil,
			expectChain:    false,
			expectRoute:    false,
			expectDisabled: false,
		},
		{
			name:           "nil policy does nothing",
			apiKeyAuthIr:   &apiKeyAuthIR{config: nil},
			expectChain:    false,
			expectRoute:    false,
			expectDisabled: false,
		},
		{
			name: "valid policy adds to chain and route",
			apiKeyAuthIr: &apiKeyAuthIR{
				config: &envoyapikeyauthv3.ApiKeyAuthPerRoute{
					Credentials: []*envoyapikeyauthv3.Credential{
						{
							Key:    "test-key",
							Client: "test-client",
						},
					},
					KeySources: []*envoyapikeyauthv3.KeySource{
						{
							Header: "api-key",
						},
					},
					Forwarding: &envoyapikeyauthv3.Forwarding{
						Header:          "x-client-id",
						HideCredentials: false,
					},
				},
			},
			expectChain:    true,
			expectRoute:    true,
			expectDisabled: false,
		},
		{
			name: "disabled policy sets disabled route config",
			apiKeyAuthIr: &apiKeyAuthIR{
				disable: true,
			},
			expectChain:    false,
			expectRoute:    true,
			expectDisabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &trafficPolicyPluginGwPass{
				apiKeyAuthInChain: make(map[string]*envoyapikeyauthv3.ApiKeyAuth),
			}
			fcn := "test-filter-chain"
			typedFilterConfig := &ir.TypedFilterConfigMap{}

			plugin.handleAPIKeyAuth(fcn, typedFilterConfig, tt.apiKeyAuthIr)

			if tt.expectChain {
				assert.NotNil(t, plugin.apiKeyAuthInChain[fcn], "should add to chain")
			} else {
				assert.Nil(t, plugin.apiKeyAuthInChain[fcn], "should not add to chain")
			}

			if tt.expectRoute {
				config := typedFilterConfig.GetTypedConfig(apiKeyAuthFilterNamePrefix)
				assert.NotNil(t, config, "should add per-route config")
				if config != nil {
					if tt.expectDisabled {
						// When disabled, we get a FilterConfig with Disabled=true
						filterConfig, ok := config.(*envoyroutev3.FilterConfig)
						require.True(t, ok, "config should be FilterConfig when disabled")
						assert.True(t, filterConfig.Disabled, "filter should be disabled")
					} else {
						perRouteConfig, ok := config.(*envoyapikeyauthv3.ApiKeyAuthPerRoute)
						require.True(t, ok, "config should be ApiKeyAuthPerRoute")
						assert.NotNil(t, perRouteConfig.Credentials)
						assert.NotNil(t, perRouteConfig.KeySources)
					}
				}
			} else {
				config := typedFilterConfig.GetTypedConfig(apiKeyAuthFilterNamePrefix)
				assert.Nil(t, config, "should not add per-route config")
			}
		})
	}
}

func TestHttpFiltersAPIKeyAuth(t *testing.T) {
	t.Run("adds api key auth filter and auth-enabled filter to chain", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{
			enableAuthMetadata: true,
			apiKeyAuthInChain: map[string]*envoyapikeyauthv3.ApiKeyAuth{
				"test-filter-chain": {},
			},
		}
		fcc := ir.FilterChainCommon{FilterChainName: "test-filter-chain"}

		httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, fcc)

		require.NoError(t, err)
		require.NotNil(t, httpFilters)
		// api key auth filter followed by auth-enabled metadata filter
		assert.Equal(t, 2, len(httpFilters))
		assert.Equal(t, apiKeyAuthFilterNamePrefix, httpFilters[0].Filter.GetName())
		assert.Equal(t, filters.DuringStage(filters.AuthNStage), httpFilters[0].Stage)
		assert.Equal(t, APIKeyAuthEnabledFilterName, httpFilters[1].Filter.GetName())
		assert.Equal(t, filters.AfterStage(filters.AuthNStage), httpFilters[1].Stage)
	})
}

func TestAPIKeyAuthPolicyPlugin(t *testing.T) {
	t.Run("applies api key auth configuration to route", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{enableAuthMetadata: true}
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				apiKeyAuth: &apiKeyAuthIR{
					config: &envoyapikeyauthv3.ApiKeyAuthPerRoute{
						Credentials: []*envoyapikeyauthv3.Credential{
							{Key: "test-key", Client: "test-client"},
						},
					},
				},
			},
		}
		pCtx := &ir.RouteContext{
			Policy: policy,
		}
		outputRoute := &envoyroutev3.Route{}

		// Execute
		err := plugin.ApplyForRoute(pCtx, outputRoute)

		// Verify
		require.NoError(t, err)
		require.NotNil(t, pCtx.TypedFilterConfig)
		apiKeyAuthConfig, ok := pCtx.TypedFilterConfig[apiKeyAuthFilterNamePrefix]
		assert.True(t, ok)
		assert.NotNil(t, apiKeyAuthConfig)
		assert.NotEmpty(t, pCtx.TypedFilterConfig[APIKeyAuthEnabledFilterName])
		assert.Contains(t, fmt.Sprintf("%s", pCtx.TypedFilterConfig[APIKeyAuthEnabledFilterName]),
			`\"key\":\"auth_succeeded\",\"value\":{\"stringValue\":\"true\"}}`, "api_key_auth_enabled must set dynamic metadata")
	})

	t.Run("handles disabled api key auth configuration", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{enableAuthMetadata: true}
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				apiKeyAuth: &apiKeyAuthIR{disable: true},
			},
		}
		pCtx := &ir.RouteContext{
			Policy: policy,
		}
		outputRoute := &envoyroutev3.Route{}

		// Execute
		err := plugin.ApplyForRoute(pCtx, outputRoute)

		// Verify
		require.NoError(t, err)
		assert.NotNil(t, pCtx.TypedFilterConfig, pCtx)
		assert.NotEmpty(t, pCtx.TypedFilterConfig[apiKeyAuthFilterNamePrefix])
		assert.NotEmpty(t, pCtx.TypedFilterConfig[APIKeyAuthEnabledFilterName])
		assert.NotContains(t, fmt.Sprintf("%s", pCtx.TypedFilterConfig[APIKeyAuthEnabledFilterName]), AuthSucceededMetadataKey, "api_key_auth_enabled must not set dynamic metadata if the policy is disabled at the route level")
	})
}

func TestDedupeAPIKeyCredentials(t *testing.T) {
	const policyRef = "app/apikey"
	// A value distinctive enough that finding it anywhere in an error message is proof of a leak.
	const apiKeyValue = "k-le4k-canary-1111" //nolint:gosec // G101: a test fixture, not a credential

	cred := func(client, key string) *envoyapikeyauthv3.Credential {
		return &envoyapikeyauthv3.Credential{Client: client, Key: key}
	}

	tests := []struct {
		name     string
		parsed   []parsedAPIKey
		expected []*envoyapikeyauthv3.Credential
		// errContains are substrings every returned error message together must cover.
		errContains []string
		errCount    int
	}{
		{
			name: "distinct credentials are all emitted, ordered by client",
			parsed: []parsedAPIKey{
				{client: "client2", key: "key-2", secret: "app/keys"},
				{client: "client1", key: "key-1", secret: "app/keys"},
			},
			expected: []*envoyapikeyauthv3.Credential{
				cred("client1", "key-1"),
				cred("client2", "key-2"),
			},
		},
		{
			name: "one client may hold several distinct keys",
			parsed: []parsedAPIKey{
				{client: "client1", key: "key-b", secret: "app/keys"},
				{client: "client1", key: "key-a", secret: "app/keys"},
			},
			expected: []*envoyapikeyauthv3.Credential{
				cred("client1", "key-a"),
				cred("client1", "key-b"),
			},
		},
		{
			name: "the same credential in two secrets is collapsed without an error",
			parsed: []parsedAPIKey{
				{client: "client1", key: "key-1", secret: "gateway-system/keys"},
				{client: "client1", key: "key-1", secret: "app/keys"},
				{client: "client2", key: "key-2", secret: "app/keys"},
			},
			expected: []*envoyapikeyauthv3.Credential{
				cred("client1", "key-1"),
				cred("client2", "key-2"),
			},
		},
		{
			name: "two clients sharing a key value is reported",
			parsed: []parsedAPIKey{
				{client: "client2", key: apiKeyValue, secret: "app/keys-b"},
				{client: "client1", key: apiKeyValue, secret: "app/keys-a"},
			},
			// Only the first of the conflicting pair survives, so the emitted list still
			// satisfies Envoy's uniqueness rule even though the caller discards it.
			expected: []*envoyapikeyauthv3.Credential{
				cred("client1", apiKeyValue),
			},
			errCount:    1,
			errContains: []string{`client "client1" in secret app/keys-a`, `client "client2" in secret app/keys-b`},
		},
		{
			name: "every conflicting client is reported",
			parsed: []parsedAPIKey{
				{client: "client1", key: apiKeyValue, secret: "app/keys-a"},
				{client: "client2", key: apiKeyValue, secret: "app/keys-b"},
				{client: "client3", key: apiKeyValue, secret: "app/keys-c"},
			},
			expected: []*envoyapikeyauthv3.Credential{
				cred("client1", apiKeyValue),
			},
			errCount:    2,
			errContains: []string{`"client2"`, `"client3"`},
		},
		{
			name:     "no credentials",
			parsed:   nil,
			expected: []*envoyapikeyauthv3.Credential{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentials, errs := dedupeAPIKeyCredentials(tt.parsed, policyRef)

			require.Len(t, errs, tt.errCount)
			joined := errors.Join(errs...)
			for _, want := range tt.errContains {
				require.ErrorContains(t, joined, want)
			}
			if tt.errCount > 0 {
				// The message is copied verbatim into policy status, which is far more
				// readable and more widely replicated than a log line.
				assert.NotContains(t, joined.Error(), apiKeyValue, "error message must not contain the api key value")
			}

			require.Len(t, credentials, len(tt.expected))
			for i, want := range tt.expected {
				assert.True(t, proto.Equal(want, credentials[i]), "credential %d: want %v, got %v", i, want, credentials[i])
			}
		})
	}
}

// TestDedupeAPIKeyCredentialsIsOrderIndependent guards the KRT contract: Secret.Data is a map
// and the secret fetch is unordered, so the same inputs reaching the loop in a different order
// must still produce byte-identical config. Otherwise apiKeyAuthIR.Equals reports a change on
// every recompute and the gateway is pushed config it already has.
func TestDedupeAPIKeyCredentialsIsOrderIndependent(t *testing.T) {
	base := []parsedAPIKey{
		{client: "client1", key: "key-1", secret: "app/keys-a"},
		{client: "client2", key: "key-2", secret: "app/keys-b"},
		{client: "client1", key: "key-1", secret: "gateway-system/keys-a"},
		{client: "client3", key: "key-3", secret: "app/keys-c"},
	}

	want, errs := dedupeAPIKeyCredentials(base, "app/apikey")
	require.Empty(t, errs)
	wantCfg := &envoyapikeyauthv3.ApiKeyAuthPerRoute{Credentials: want}

	// Every rotation of the input is a plausible map iteration order.
	for i := range base {
		rotated := append(append([]parsedAPIKey{}, base[i:]...), base[:i]...)
		got, errs := dedupeAPIKeyCredentials(rotated, "app/apikey")
		require.Empty(t, errs)
		gotCfg := &envoyapikeyauthv3.ApiKeyAuthPerRoute{Credentials: got}
		assert.True(t, proto.Equal(wantCfg, gotCfg), "rotation by %d produced a different config: %v", i, got)
	}
}
