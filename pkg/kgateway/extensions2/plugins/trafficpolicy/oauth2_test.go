package trafficpolicy

import (
	"fmt"
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyoauth2v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/oauth2/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestRedirectPath(t *testing.T) {
	tests := []struct {
		uri     string
		want    string
		wantErr string
	}{
		{
			uri:  defaultRedictURI,
			want: "/oauth2/redirect",
		},
		{
			uri:  "https://foo.com/bar/baz",
			want: "/bar/baz",
		},
		{
			uri:     "foo.com/bar/baz",
			want:    "",
			wantErr: "missing scheme",
		},
		{
			uri:     "https://foo.com/",
			want:    "",
			wantErr: "missing path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			a := assert.New(t)
			path, err := parseRedirectPath(tt.uri)
			a.Equal(tt.want, path)
			if tt.wantErr != "" {
				a.ErrorContains(err, tt.wantErr)
			} else {
				a.NoError(err)
			}
		})
	}
}

func TestHttpFiltersOAuth2(t *testing.T) {
	t.Run("adds oauth2 filter and auth-enabled filter to chain", func(t *testing.T) {
		plugin := &trafficPolicyPluginGwPass{
			enableAuthMetadata: true,
			oauth2PerProvider: ProviderNeededMap{
				Providers: map[string][]Provider{
					"test-filter-chain": {
						{
							Name: "test-oauth2",
							Extension: &TrafficPolicyGatewayExtensionIR{
								Name: "test-oauth2",
								OAuth2: &oauthPerProviderConfig{
									cfg: &envoyoauth2v3.OAuth2{},
								},
							},
						},
					},
				},
			},
		}
		fcc := ir.FilterChainCommon{FilterChainName: "test-filter-chain"}

		httpFilters, err := plugin.HttpFilters(ir.HttpFiltersContext{}, fcc)

		require.NoError(t, err)
		require.NotNil(t, httpFilters)
		// auth-enabled metadata filter followed by oauth2 filter
		assert.Equal(t, 2, len(httpFilters))
		assert.Equal(t, OauthEnabledFilterName, httpFilters[0].Filter.GetName())
		assert.Equal(t, filters.AfterStage(filters.AuthNStage), httpFilters[0].Stage)
		assert.Equal(t, oauthFilterName("test-oauth2"), httpFilters[1].Filter.GetName())
		assert.Equal(t, filters.BeforeStage(filters.AuthNStage), httpFilters[1].Stage)
	})
}

func TestOAuth2PolicyPlugin(t *testing.T) {
	t.Run("applies oauth2 configuration to route", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{enableAuthMetadata: true}
		policy := &TrafficPolicy{
			spec: trafficPolicySpecIr{
				oauth2: &oauthIR{
					oauthPerProviderConfig: &oauthPerProviderConfig{
						cfg: &envoyoauth2v3.OAuth2{},
					},
					source: &TrafficPolicyGatewayExtensionIR{
						Name: "test-oauth2",
						OAuth2: &oauthPerProviderConfig{
							cfg: &envoyoauth2v3.OAuth2{},
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
		oauthConfig, ok := pCtx.TypedFilterConfig[oauthFilterName("test-oauth2")]
		assert.True(t, ok)
		assert.NotNil(t, oauthConfig)
		assert.NotEmpty(t, pCtx.TypedFilterConfig[OauthEnabledFilterName])
		assert.Contains(t, fmt.Sprintf("%s", pCtx.TypedFilterConfig[OauthEnabledFilterName]),
			`\"key\":\"auth_succeeded\",\"value\":{\"stringValue\":\"true\"}}`, "oauth2_enabled must set dynamic metadata")
	})
}
