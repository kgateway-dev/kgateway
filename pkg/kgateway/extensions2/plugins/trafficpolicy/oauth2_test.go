package trafficpolicy

import (
	"fmt"
	"testing"
	"time"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyoauth2v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/oauth2/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kgwv1a1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestBuildOAuth2JWTConfig(t *testing.T) {
	makeBackend := func(name string) *ir.BackendObjectIR {
		b := ir.NewBackendObjectIR(ir.ObjectSource{
			Namespace: "default",
			Name:      name,
			Kind:      "Backend",
		}, 443, "", "")
		return &b
	}
	cookieNames := &envoyoauth2v3.OAuth2Credentials_CookieNames{
		BearerToken: "AccessToken-abc",
		IdToken:     "IdToken-abc",
	}

	t.Run("uses provided jwksBackend cluster for RemoteJwks", func(t *testing.T) {
		jwksBackend := makeBackend("jwks-backend")
		ext := &ir.GatewayExtension{
			OAuth2: &kgwv1a1.OAuth2Provider{
				JWT: &kgwv1a1.OAuth2JWTConfig{
					AccessToken: &kgwv1a1.OAuth2JWTProcessingConfig{},
				},
			},
		}

		jwtCfg, err := buildOAuth2JWTConfig(ext, "https://example.com/jwks.json", cookieNames, jwksBackend)

		require.NoError(t, err)
		require.NotNil(t, jwtCfg)
		provider := jwtCfg.Providers[oauthJWTAccessTokenProvider]
		require.NotNil(t, provider)
		assert.Equal(t, jwksBackend.ClusterName(), provider.GetRemoteJwks().GetHttpUri().GetCluster())
	})

	t.Run("jwksBackend cluster is independent of primary backend", func(t *testing.T) {
		primaryBackend := makeBackend("primary-backend")
		jwksBackend := makeBackend("jwks-backend")
		require.NotEqual(t, primaryBackend.ClusterName(), jwksBackend.ClusterName())

		ext := &ir.GatewayExtension{
			OAuth2: &kgwv1a1.OAuth2Provider{
				JWT: &kgwv1a1.OAuth2JWTConfig{
					AccessToken: &kgwv1a1.OAuth2JWTProcessingConfig{},
				},
			},
		}

		jwtCfg, err := buildOAuth2JWTConfig(ext, "https://example.com/jwks.json", cookieNames, jwksBackend)

		require.NoError(t, err)
		provider := jwtCfg.Providers[oauthJWTAccessTokenProvider]
		cluster := provider.GetRemoteJwks().GetHttpUri().GetCluster()
		assert.Equal(t, jwksBackend.ClusterName(), cluster)
		assert.NotEqual(t, primaryBackend.ClusterName(), cluster)
	})

	t.Run("leaves remote JWKS fetch options at their defaults when unset", func(t *testing.T) {
		ext := &ir.GatewayExtension{
			OAuth2: &kgwv1a1.OAuth2Provider{
				JWT: &kgwv1a1.OAuth2JWTConfig{
					AccessToken: &kgwv1a1.OAuth2JWTProcessingConfig{},
				},
			},
		}

		jwtCfg, err := buildOAuth2JWTConfig(ext, "https://example.com/jwks.json", cookieNames, makeBackend("jwks-backend"))

		require.NoError(t, err)
		remote := jwtCfg.Providers[oauthJWTAccessTokenProvider].GetRemoteJwks()
		require.NotNil(t, remote)
		assert.Equal(t, 5*time.Second, remote.GetHttpUri().GetTimeout().AsDuration(), "fetch timeout should default to 5s")
		assert.Nil(t, remote.GetCacheDuration(), "cache duration should be left to Envoy's default")
		assert.Nil(t, remote.GetAsyncFetch(), "async fetch should be off by default")
		assert.Nil(t, remote.GetRetryPolicy(), "retry policy should be off by default")
	})

	t.Run("applies remote JWKS fetch options to access and ID token providers", func(t *testing.T) {
		ext := &ir.GatewayExtension{
			OAuth2: &kgwv1a1.OAuth2Provider{
				JWT: &kgwv1a1.OAuth2JWTConfig{
					CacheDuration: &metav1.Duration{Duration: 30 * time.Minute},
					Timeout:       &metav1.Duration{Duration: 10 * time.Second},
					AsyncFetch: &kgwv1a1.JWKSAsyncFetch{
						FastListener:          new(true),
						FailedRefetchDuration: &metav1.Duration{Duration: 5 * time.Second},
					},
					RetryPolicy: &kgwv1a1.JWKSRetryPolicy{
						NumRetries: new(int32(3)),
						BackOff: &kgwv1a1.JWKSRetryBackOff{
							BaseInterval: metav1.Duration{Duration: 500 * time.Millisecond},
							MaxInterval:  &metav1.Duration{Duration: 5 * time.Second},
						},
					},
					AccessToken: &kgwv1a1.OAuth2JWTProcessingConfig{},
					IDToken:     &kgwv1a1.OAuth2JWTProcessingConfig{},
				},
			},
		}

		jwtCfg, err := buildOAuth2JWTConfig(ext, "https://example.com/jwks.json", cookieNames, makeBackend("jwks-backend"))

		require.NoError(t, err)
		for _, name := range []string{oauthJWTAccessTokenProvider, oauthJWTIDTokenProvider} {
			remote := jwtCfg.Providers[name].GetRemoteJwks()
			require.NotNil(t, remote, "provider %s should use remote JWKS", name)
			assert.Equal(t, 10*time.Second, remote.GetHttpUri().GetTimeout().AsDuration(), name)
			assert.Equal(t, 30*time.Minute, remote.GetCacheDuration().AsDuration(), name)
			assert.True(t, remote.GetAsyncFetch().GetFastListener(), name)
			assert.Equal(t, 5*time.Second, remote.GetAsyncFetch().GetFailedRefetchDuration().AsDuration(), name)
			assert.Equal(t, uint32(3), remote.GetRetryPolicy().GetNumRetries().GetValue(), name)
			assert.Equal(t, 500*time.Millisecond, remote.GetRetryPolicy().GetRetryBackOff().GetBaseInterval().AsDuration(), name)
			assert.Equal(t, 5*time.Second, remote.GetRetryPolicy().GetRetryBackOff().GetMaxInterval().AsDuration(), name)
		}
	})
}

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
