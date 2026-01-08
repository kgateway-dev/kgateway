package trafficpolicy

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyapikeyauthv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/api_key_auth/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
