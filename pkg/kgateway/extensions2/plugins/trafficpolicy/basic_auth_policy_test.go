package trafficpolicy

import (
	"fmt"
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_basic_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/basic_auth/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestValidateAndFilterSHAUsers(t *testing.T) {
	tests := []struct {
		name            string
		htpasswdData    string
		expectedValid   []string
		expectedInvalid []string
	}{
		{
			name:            "empty htpasswd data",
			htpasswdData:    "",
			expectedValid:   []string{},
			expectedInvalid: []string{},
		},
		{
			name:            "single valid SHA user",
			htpasswdData:    "user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
			expectedValid:   []string{"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs="},
			expectedInvalid: []string{},
		},
		{
			name: "multiple valid SHA users",
			htpasswdData: `user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=
user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=
user3:{SHA}d95o2uzYI7q7tY7bHI4U1xBug7s=`,
			expectedValid: []string{
				"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
				"user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=",
				"user3:{SHA}d95o2uzYI7q7tY7bHI4U1xBug7s=",
			},
			expectedInvalid: []string{},
		},
		{
			name:            "MD5 hash is filtered out",
			htpasswdData:    "alice:$apr1$3zSE0Abt$IuETi4l5yO87MuOrbSE4V.",
			expectedValid:   []string{},
			expectedInvalid: []string{"alice"},
		},
		{
			name:            "bcrypt hash is filtered out",
			htpasswdData:    "bob:$2y$05$r3J4d3VepzFkedkd/q1vI.pBYIpSqjfN0qOARV3ScUHysatnS0cL2",
			expectedValid:   []string{},
			expectedInvalid: []string{"bob"},
		},
		{
			name: "mixed valid and invalid users",
			htpasswdData: `user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=
alice:$apr1$3zSE0Abt$IuETi4l5yO87MuOrbSE4V.
user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=
bob:$2y$05$r3J4d3VepzFkedkd/q1vI.pBYIpSqjfN0qOARV3ScUHysatnS0cL2
user3:{SHA}d95o2uzYI7q7tY7bHI4U1xBug7s=`,
			expectedValid: []string{
				"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
				"user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=",
				"user3:{SHA}d95o2uzYI7q7tY7bHI4U1xBug7s=",
			},
			expectedInvalid: []string{"alice", "bob"},
		},
		{
			name: "empty lines and comments are skipped",
			htpasswdData: `# Comment line
user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=

# Another comment
user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=

`,
			expectedValid: []string{
				"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
				"user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=",
			},
			expectedInvalid: []string{},
		},
		{
			name: "whitespace is trimmed",
			htpasswdData: `  user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=
	user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=	`,
			expectedValid: []string{
				"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
				"user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=",
			},
			expectedInvalid: []string{},
		},
		{
			name:            "malformed entry without colon",
			htpasswdData:    "invalidentry",
			expectedValid:   []string{},
			expectedInvalid: []string{"invalidentry"},
		},
		{
			name: "malformed entries mixed with valid ones",
			htpasswdData: `user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=
malformedentry
user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=`,
			expectedValid: []string{
				"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
				"user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=",
			},
			expectedInvalid: []string{"malformedentry"},
		},
		{
			name:            "username with special characters",
			htpasswdData:    "user@example.com:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
			expectedValid:   []string{"user@example.com:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs="},
			expectedInvalid: []string{},
		},
		{
			name:            "password with colon in hash (multiple colons in line)",
			htpasswdData:    "user:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=:extra",
			expectedValid:   []string{},
			expectedInvalid: []string{"user"},
		},
		{
			name:            "crypt hash is filtered out",
			htpasswdData:    "user:rl5FQ9fW/7E6A",
			expectedValid:   []string{},
			expectedInvalid: []string{"user"},
		},
		{
			name: "all users invalid - only MD5 and bcrypt",
			htpasswdData: `alice:$apr1$3zSE0Abt$IuETi4l5yO87MuOrbSE4V.
bob:$2y$05$r3J4d3VepzFkedkd/q1vI.pBYIpSqjfN0qOARV3ScUHysatnS0cL2`,
			expectedValid:   []string{},
			expectedInvalid: []string{"alice", "bob"},
		},
		{
			name: "duplicate valid users",
			htpasswdData: `user:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=
user:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=`,
			expectedValid:   []string{"user:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs="},
			expectedInvalid: []string{"user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validUsers, invalidUsernames := validateAndFilterSHAUsers(tt.htpasswdData)
			assert.Equal(t, tt.expectedValid, validUsers, "valid users mismatch")
			assert.Equal(t, tt.expectedInvalid, invalidUsernames, "invalid usernames mismatch")
		})
	}
}

func TestHttpFiltersBasicAuth(t *testing.T) {
	t.Run("adds basic auth filter and auth-enabled filter to chain", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{
			enableAuthMetadata: true,
			basicAuthInChain: map[string]*envoy_basic_auth_v3.BasicAuth{
				"test-filter-chain": {},
			},
		}
		fcc := ir.FilterChainCommon{FilterChainName: "test-filter-chain"}

		httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, fcc)

		require.NoError(t, err)
		require.NotNil(t, httpFilters)
		// basic auth filter followed by auth-enabled metadata filter
		assert.Equal(t, 2, len(httpFilters))
		assert.Equal(t, basicAuthFilterName, httpFilters[0].Filter.GetName())
		assert.Equal(t, filters.DuringStage(filters.AuthNStage), httpFilters[0].Stage)
		assert.Equal(t, BasicAuthEnabledFilterName, httpFilters[1].Filter.GetName())
		assert.Equal(t, filters.AfterStage(filters.AuthNStage), httpFilters[1].Stage)
	})
}

func TestBasicAuthPolicyPlugin(t *testing.T) {
	t.Run("applies basic auth configuration to route", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{enableAuthMetadata: true}
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				basicAuth: &basicAuthIR{
					policy: &envoy_basic_auth_v3.BasicAuthPerRoute{},
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
		basicAuthConfig, ok := pCtx.TypedFilterConfig[basicAuthFilterName]
		assert.True(t, ok)
		assert.NotNil(t, basicAuthConfig)
		assert.NotEmpty(t, pCtx.TypedFilterConfig[BasicAuthEnabledFilterName])
		assert.Contains(t, fmt.Sprintf("%s", pCtx.TypedFilterConfig[BasicAuthEnabledFilterName]),
			`\"key\":\"auth_succeeded\",\"value\":{\"stringValue\":\"true\"}}`, "basic_auth_enabled must set dynamic metadata")
	})

	t.Run("handles disabled basic auth configuration", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{enableAuthMetadata: true}
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				basicAuth: &basicAuthIR{disable: true},
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
		assert.NotEmpty(t, pCtx.TypedFilterConfig[basicAuthFilterName])
		assert.NotEmpty(t, pCtx.TypedFilterConfig[BasicAuthEnabledFilterName])
		assert.NotContains(t, fmt.Sprintf("%s", pCtx.TypedFilterConfig[BasicAuthEnabledFilterName]), AuthSucceededMetadataKey, "basic_auth_enabled must not set dynamic metadata if the policy is disabled at the route level")
	})
}
