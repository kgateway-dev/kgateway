package trafficpolicy

import (
	"fmt"
	"slices"
	"strings"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyjwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	envoyoauth2v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/oauth2/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	kgwv1a1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	pluginsdkutils "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/utils"
)

const (
	OauthAccessTokenCookiePrefix  = "AccessToken"
	OauthIdTokenCookiePrefix      = "IdToken"
	OauthRefreshTokenCookiePrefix = "RefreshToken"
	OauthHMACCookiePrefix         = "OauthHMAC"
	OauthExpiresCookiePrefix      = "OauthExpires"
	OauthNonceCookiePrefix        = "OauthNonce"
	OauthCodeVerifierCookiePrefix = "OauthCodeVerifier"

	defaultRedictURI            = "%REQ(x-forwarded-proto)%://%REQ(:authority)%/oauth2/redirect"
	clientSecretKey             = "client-secret"
	defaultTokenEndpointTimeout = 15 * time.Second

	oauthJWTRequirementName     = "oauth2"
	oauthJWTAccessTokenProvider = "oauth2/accessToken" //nolint:gosec // G101: This is only an identifier within the Envoy filter, not a credential
	oauthJWTIDTokenProvider     = "oauth2/idToken"     //nolint:gosec // G101: This is only an identifier within the Envoy filter, not a credential
)

type oauthIR struct {
	*oauthPerProviderConfig
	source *TrafficPolicyGatewayExtensionIR
}

type oauthPerProviderConfig struct {
	cfg     *envoyoauth2v3.OAuth2
	jwtCfg  *envoyjwtauthnv3.JwtAuthentication
	secrets []*envoytlsv3.Secret
}

func (a *oauthPerProviderConfig) Equals(b *oauthPerProviderConfig) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return proto.Equal(a.cfg, b.cfg) && proto.Equal(a.jwtCfg, b.jwtCfg) && slices.EqualFunc(a.secrets, b.secrets, func(x, y *envoytlsv3.Secret) bool {
		return proto.Equal(x, y)
	})
}

var _ PolicySubIR = (*oauthIR)(nil)

func (a *oauthIR) Equals(other PolicySubIR) bool {
	b, ok := other.(*oauthIR)
	if !ok {
		return false
	}
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	return a.oauthPerProviderConfig.Equals(b.oauthPerProviderConfig) && a.source.Equals(*b.source)
}

func (o *oauthIR) Validate() error {
	if o == nil {
		return nil
	}
	if err := o.cfg.ValidateAll(); err != nil {
		return err
	}
	for _, secret := range o.secrets {
		if err := secret.ValidateAll(); err != nil {
			return err
		}
	}
	return nil
}

func constructOAuth2(
	krtctx krt.HandlerContext,
	in *kgwv1a1.TrafficPolicy,
	fetchGatewayExtension FetchGatewayExtensionFunc,
	out *trafficPolicySpecIr,
) error {
	spec := in.Spec.OAuth2
	if spec == nil {
		return nil
	}

	provider, err := fetchGatewayExtension(krtctx, spec.ExtensionRef, in.GetNamespace())
	if err != nil {
		return fmt.Errorf("extauth: %w", err)
	}
	if provider.OAuth2 == nil {
		return pluginutils.ErrInvalidExtensionType(kgwv1a1.GatewayExtensionTypeOAuth2)
	}
	out.oauth2 = &oauthIR{
		oauthPerProviderConfig: provider.OAuth2,
		source:                 provider,
	}

	return nil
}

func buildOAuth2ProviderConfig(
	krtctx krt.HandlerContext,
	ext *ir.GatewayExtension,
	backends *krtcollections.BackendIndex,
	secrets *krtcollections.SecretIndex,
	discoverer *oidcProviderConfigDiscoverer,
) (*oauthPerProviderConfig, error) {
	in := ext.OAuth2

	tokenEndpoint := ptr.Deref(in.TokenEndpoint, "").String()
	authorizationEndpoint := ptr.Deref(in.AuthorizationEndpoint, "").String()
	endSessionEndpoint := ptr.Deref(in.EndSessionEndpoint, "").String()
	var jwksURI string
	if in.JWT != nil {
		jwksURI = ptr.Deref(in.JWT.JWKSURI, "").String()
	}

	if in.IssuerURI != nil {
		// only discover config if we need to, i.e., when either tokenEndpoint, authorizationEndpoint, or endSessionEndpoint is not provided
		if in.TokenEndpoint == nil || in.AuthorizationEndpoint == nil || in.EndSessionEndpoint == nil || (in.JWT != nil && in.JWT.JWKSURI == nil) {
			openidCfg, err := discoverer.get(*in.IssuerURI)
			if err != nil {
				return nil, err
			}
			if tokenEndpoint == "" {
				tokenEndpoint = openidCfg.TokenEndpoint
			}
			if authorizationEndpoint == "" {
				authorizationEndpoint = openidCfg.AuthorizationEndpoint
			}
			if endSessionEndpoint == "" {
				endSessionEndpoint = ptr.Deref(openidCfg.EndSessionEndpoint, "")
			}
			if jwksURI == "" {
				jwksURI = openidCfg.JWKSURI
			}
		}
	}

	if tokenEndpoint == "" {
		return nil, fmt.Errorf("oauth2 token endpoint not specified or not found in issuer well-known configuration")
	}
	if authorizationEndpoint == "" {
		return nil, fmt.Errorf("oauth2 authorization endpoint not specified or not found in issuer well-known configuration")
	}

	backend, err := resolveBackend(krtctx, backends, false, ext.ObjectSource, in.BackendRef.BackendObjectReference)
	if err != nil || backend == nil {
		return nil, fmt.Errorf("error resolving oauth2 backend %v: %w", in.BackendRef.BackendObjectReference, err)
	}

	// Fetch the client credentials
	credSecret, err := secrets.GetSecret(krtctx,
		krtcollections.From{GroupKind: wellknown.GatewayExtensionGVK.GroupKind(), Namespace: ext.Namespace},
		gwv1.SecretObjectReference{
			Name: gwv1.ObjectName(in.Credentials.ClientSecretRef.Name), Namespace: new(gwv1.Namespace(ext.Namespace)),
		},
	)
	if err != nil {
		return nil, err
	}
	clientSecretData, ok := credSecret.Data[clientSecretKey]
	if !ok || len(clientSecretData) == 0 {
		return nil, fmt.Errorf("%s not found or empty in secret %s referenced by GatewayExtension %s",
			clientSecretKey, credSecret.ResourceName(), ext.ResourceName())
	}
	// TODO(shashank): customize cookie names for collisions
	hmacSecret, err := secrets.GetSecretWithoutRefGrant(krtctx, wellknown.OAuth2HMACSecret.Name, wellknown.OAuth2HMACSecret.Namespace)
	if err != nil {
		return nil, err
	}
	hmacSecretData, ok := hmacSecret.Data[wellknown.OAuth2HMACSecretKey]
	if !ok || len(hmacSecretData) == 0 {
		return nil, fmt.Errorf("%s not found or empty in secret %s for OAuth2",
			wellknown.OAuth2HMACSecretKey, hmacSecret.ResourceName())
	}

	redirectURI := ptr.Deref(in.RedirectURI, defaultRedictURI)
	redirectPath, err := parseRedirectPath(redirectURI)
	if err != nil {
		return nil, fmt.Errorf("invalid redirectURI: %w", err)
	}

	cookieSuffix := getCookieSuffix(ext.ObjectSource)
	cookieNames := &envoyoauth2v3.OAuth2Credentials_CookieNames{
		BearerToken:  OauthAccessTokenCookiePrefix + "-" + cookieSuffix,
		IdToken:      OauthIdTokenCookiePrefix + "-" + cookieSuffix,
		OauthHmac:    OauthHMACCookiePrefix + "-" + cookieSuffix,
		OauthExpires: OauthExpiresCookiePrefix + "-" + cookieSuffix,
		RefreshToken: OauthRefreshTokenCookiePrefix + "-" + cookieSuffix,
		OauthNonce:   OauthNonceCookiePrefix + "-" + cookieSuffix,
		CodeVerifier: OauthCodeVerifierCookiePrefix + "-" + cookieSuffix,
	}
	if in.Cookies != nil && in.Cookies.Names != nil {
		if in.Cookies.Names.AccessToken != nil {
			cookieNames.BearerToken = *in.Cookies.Names.AccessToken
		}
		if in.Cookies.Names.IDToken != nil {
			cookieNames.IdToken = *in.Cookies.Names.IDToken
		}
	}

	forwardBearerToken := ptr.Deref(in.ForwardAccessToken, false)
	cfg := &envoyoauth2v3.OAuth2{
		Config: &envoyoauth2v3.OAuth2Config{
			TokenEndpoint: &envoycorev3.HttpUri{
				Uri: tokenEndpoint,
				HttpUpstreamType: &envoycorev3.HttpUri_Cluster{
					Cluster: backend.ClusterName(),
				},
				Timeout: durationpb.New(defaultTokenEndpointTimeout),
			},
			AuthorizationEndpoint: authorizationEndpoint,
			ForwardBearerToken:    forwardBearerToken,
			// Preserve the Authorization header by default unless it is being used to explicitly forward the access(bearer) token.
			// This is useful when the client explicitly sets the Authorization header, e.g., for basic auth
			PreserveAuthorizationHeader: !forwardBearerToken,
			AuthScopes:                  in.Scopes,
			Credentials: &envoyoauth2v3.OAuth2Credentials{
				ClientId: in.Credentials.ClientID,
				TokenSecret: &envoytlsv3.SdsSecretConfig{
					Name:      oauthClientSecretName(in.Credentials.ClientSecretRef.Name, ext.Namespace),
					SdsConfig: adsConfigSource(),
				},
				TokenFormation: &envoyoauth2v3.OAuth2Credentials_HmacSecret{
					HmacSecret: &envoytlsv3.SdsSecretConfig{
						Name:      oauthHMACSecretName(hmacSecret.Name, hmacSecret.Namespace),
						SdsConfig: adsConfigSource(),
					},
				},
				CookieNames: cookieNames,
			},
			RedirectUri: redirectURI,
			RedirectPathMatcher: &envoymatcherv3.PathMatcher{
				Rule: &envoymatcherv3.PathMatcher_Path{
					Path: &envoymatcherv3.StringMatcher{
						MatchPattern: &envoymatcherv3.StringMatcher_Exact{
							Exact: redirectPath,
						},
					},
				},
			},
			SignoutPath: &envoymatcherv3.PathMatcher{
				Rule: &envoymatcherv3.PathMatcher_Path{
					Path: &envoymatcherv3.StringMatcher{
						MatchPattern: &envoymatcherv3.StringMatcher_Exact{
							Exact: in.LogoutPath,
						},
					},
				},
			},
			EndSessionEndpoint: endSessionEndpoint,
		},
	}

	if in.Cookies != nil && in.Cookies.Domain != nil {
		cfg.Config.Credentials.CookieDomain = *in.Cookies.Domain
	}

	if in.Cookies != nil && in.Cookies.SameSite != nil {
		config := &envoyoauth2v3.CookieConfig{}
		switch *in.Cookies.SameSite {
		case kgwv1a1.OAuth2CookieSameSiteLax:
			config.SameSite = envoyoauth2v3.CookieConfig_LAX
		case kgwv1a1.OAuth2CookieSameSiteStrict:
			config.SameSite = envoyoauth2v3.CookieConfig_STRICT
		case kgwv1a1.OAuth2CookieSameSiteNone:
			config.SameSite = envoyoauth2v3.CookieConfig_NONE
		}

		cfg.Config.CookieConfigs = &envoyoauth2v3.CookieConfigs{
			BearerTokenCookieConfig:  config,
			IdTokenCookieConfig:      config,
			OauthHmacCookieConfig:    config,
			OauthExpiresCookieConfig: config,
			RefreshTokenCookieConfig: config,
			OauthNonceCookieConfig:   config,
			CodeVerifierCookieConfig: config,
		}
	}

	if in.Cookies != nil {
		cfg.Config.DisableAccessTokenSetCookie = ptr.Deref(in.Cookies.DisableAccessTokenSetCookie, false)
		cfg.Config.DisableIdTokenSetCookie = ptr.Deref(in.Cookies.DisableIDTokenSetCookie, false)
		cfg.Config.DisableRefreshTokenSetCookie = ptr.Deref(in.Cookies.DisableRefreshTokenSetCookie, false)
	}

	if in.DenyRedirect != nil {
		matcher, err := pluginsdkutils.ToEnvoyHeaderMatchers(in.DenyRedirect.Headers)
		if err != nil {
			return nil, fmt.Errorf("invalid deny redirect matcher: %w", err)
		}
		cfg.Config.DenyRedirectMatcher = matcher
	}

	jwtCfg, err := buildOAuth2JWTConfig(ext, jwksURI, cookieNames, backend)
	if err != nil {
		return nil, err
	}

	return &oauthPerProviderConfig{
		cfg:    cfg,
		jwtCfg: jwtCfg,
		secrets: []*envoytlsv3.Secret{
			{
				Name: oauthClientSecretName(in.Credentials.ClientSecretRef.Name, ext.Namespace),
				Type: &envoytlsv3.Secret_GenericSecret{
					GenericSecret: &envoytlsv3.GenericSecret{
						Secret: &envoycorev3.DataSource{
							Specifier: &envoycorev3.DataSource_InlineBytes{
								InlineBytes: clientSecretData,
							},
						},
					},
				},
			},
			{
				Name: oauthHMACSecretName(hmacSecret.Name, hmacSecret.Namespace),
				Type: &envoytlsv3.Secret_GenericSecret{
					GenericSecret: &envoytlsv3.GenericSecret{
						Secret: &envoycorev3.DataSource{
							Specifier: &envoycorev3.DataSource_InlineBytes{
								InlineBytes: hmacSecretData,
							},
						},
					},
				},
			},
		},
	}, nil
}

func buildOAuth2JWTConfig(
	ext *ir.GatewayExtension,
	jwksURI string,
	cookieNames *envoyoauth2v3.OAuth2Credentials_CookieNames,
	backend *ir.BackendObjectIR,
) (*envoyjwtauthnv3.JwtAuthentication, error) {
	jwt := ext.OAuth2.JWT
	if jwt == nil {
		return nil, nil
	}

	if jwt.AccessToken == nil && jwt.IDToken == nil {
		return nil, nil
	}

	if jwksURI == "" {
		return nil, fmt.Errorf("jwksURI not specified or not found in issuer well-known configuration. Required since JWT parsing is configured on GatewayExtension %s", ext.NamespacedName())
	}

	providers := map[string]*envoyjwtauthnv3.JwtProvider{}
	var requirements []*envoyjwtauthnv3.JwtRequirement

	jwksSource := &envoyjwtauthnv3.JwtProvider_RemoteJwks{
		RemoteJwks: &envoyjwtauthnv3.RemoteJwks{
			HttpUri: &envoycorev3.HttpUri{
				Timeout: &durationpb.Duration{Seconds: remoteJWKSTimeoutSecs},
				Uri:     jwksURI,
				HttpUpstreamType: &envoycorev3.HttpUri_Cluster{
					Cluster: backend.ClusterName(),
				},
			},
		},
	}
	if jwt.AccessToken != nil {
		claimsToHeaders := translateJWTClaimsToHeaders(jwt.AccessToken.ClaimsToHeaders)
		providers[oauthJWTAccessTokenProvider] = &envoyjwtauthnv3.JwtProvider{
			FromCookies:         []string{cookieNames.BearerToken},
			PayloadInMetadata:   "accessToken",
			JwksSourceSpecifier: jwksSource,
			Audiences:           jwt.AccessToken.Audiences,
			ClaimToHeaders:      claimsToHeaders,
			ClearRouteCache:     len(claimsToHeaders) > 0,
		}
		requirements = append(requirements, &envoyjwtauthnv3.JwtRequirement{
			RequiresType: &envoyjwtauthnv3.JwtRequirement_ProviderName{
				ProviderName: oauthJWTAccessTokenProvider,
			},
		})
	}

	if jwt.IDToken != nil {
		claimsToHeaders := translateJWTClaimsToHeaders(jwt.IDToken.ClaimsToHeaders)
		providers[oauthJWTIDTokenProvider] = &envoyjwtauthnv3.JwtProvider{
			FromCookies:         []string{cookieNames.IdToken},
			PayloadInMetadata:   "idToken",
			JwksSourceSpecifier: jwksSource,
			Audiences:           jwt.IDToken.Audiences,
			ClaimToHeaders:      claimsToHeaders,
			ClearRouteCache:     len(claimsToHeaders) > 0,
		}
		requirements = append(requirements, &envoyjwtauthnv3.JwtRequirement{
			RequiresType: &envoyjwtauthnv3.JwtRequirement_ProviderName{
				ProviderName: oauthJWTIDTokenProvider,
			},
		})
	}

	var combinedRequirement *envoyjwtauthnv3.JwtRequirement
	switch {
	// should never happen due to earlier guard clauses, but helps catch issues faster if the behavior ever changes
	case len(requirements) == 0:
		return nil, fmt.Errorf("did not create any JWT requirements for GatewayExtension %s, but JWT parsing was configured", ext.NamespacedName())
	// requires_all needs at least two requirements, so only use that if we have more than one
	case len(requirements) == 1:
		combinedRequirement = requirements[0]
	default:
		combinedRequirement = &envoyjwtauthnv3.JwtRequirement{
			RequiresType: &envoyjwtauthnv3.JwtRequirement_RequiresAll{
				RequiresAll: &envoyjwtauthnv3.JwtRequirementAndList{Requirements: requirements},
			},
		}
	}

	return &envoyjwtauthnv3.JwtAuthentication{
		Providers: providers,
		RequirementMap: map[string]*envoyjwtauthnv3.JwtRequirement{
			oauthJWTRequirementName: combinedRequirement,
		},
	}, nil
}

func oauthClientSecretName(name, namespace string) string {
	return fmt.Sprintf("oauth2/client_secret/%s/%s", namespace, name)
}

func oauthHMACSecretName(name, namespace string) string {
	return fmt.Sprintf("oauth2/hmac_secret/%s/%s", namespace, name)
}

func parseRedirectPath(redirectURI string) (string, error) {
	_, hostAndPath, found := strings.Cut(redirectURI, "://")
	if !found {
		return "", fmt.Errorf("invalid redirect URI: %s; missing scheme", redirectURI)
	}
	_, path, found := strings.Cut(hostAndPath, "/")
	if !found || path == "" {
		return "", fmt.Errorf("invalid redirect URI: %s; missing path", redirectURI)
	}
	return "/" + path, nil
}

func oauthFilterName(provider string) string {
	return "envoy.filters.http.oauth2/" + provider
}

func oauthJWTFilterName(provider string) string {
	return "envoy.filters.http.oauth2/jwt/" + provider
}

func adsConfigSource() *envoycorev3.ConfigSource {
	return &envoycorev3.ConfigSource{
		ResourceApiVersion: envoycorev3.ApiVersion_V3,
		ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{
			Ads: &envoycorev3.AggregatedConfigSource{},
		},
	}
}

func (p *trafficPolicyPluginGwPass) handleOauth2(filterChain string, perFilterConfig *ir.TypedFilterConfigMap, in *oauthIR) {
	if in == nil {
		return
	}

	// TODO: add disable capability when needed
	p.oauth2PerProvider.Add(filterChain, in.source.Name, in.source)
	perFilterConfig.AddTypedConfig(oauthFilterName(in.source.Name), EnableFilterPerRoute())
	if in.jwtCfg != nil {
		perFilterConfig.AddTypedConfig(oauthJWTFilterName(in.source.Name), &envoyjwtauthnv3.PerRouteConfig{
			RequirementSpecifier: &envoyjwtauthnv3.PerRouteConfig_RequirementName{
				RequirementName: oauthJWTRequirementName,
			},
		})
	}
	for _, secret := range in.secrets {
		p.secrets[secret.Name] = secret
	}
}

// getCookieSuffix generates a unique suffix for cookie names based on the given object
func getCookieSuffix(src ir.ObjectSource) string {
	hash := utils.HashString(src.NamespacedName().String())
	return fmt.Sprintf("%x", hash)
}
