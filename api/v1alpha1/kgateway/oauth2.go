package kgateway

import (
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

// HttpsUri specifies an HTTPS URI
// +kubebuilder:validation:Pattern=`^https://([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(:[0-9]{1,5})?(/[a-zA-Z0-9\-._~!$&'()*+,;=:@%]*)*/?(\?[a-zA-Z0-9\-._~!$&'()*+,;=:@%/?]*)?$`
type HttpsUri string

func (h HttpsUri) String() string {
	return string(h)
}

// OAuth2CookieSameSite specifies the SameSite attribute for OAuth2 cookies
// +kubebuilder:validation:Enum=Lax;Strict;None
type OAuth2CookieSameSite string

const (
	// OAuth2CookieSameSiteLax specifies the Lax SameSite attribute for OAuth2 cookies
	OAuth2CookieSameSiteLax OAuth2CookieSameSite = "Lax"

	// OAuth2CookieSameSiteStrict specifies the Strict SameSite attribute for OAuth2 cookies
	OAuth2CookieSameSiteStrict OAuth2CookieSameSite = "Strict"

	// OAuth2CookieSameSiteNone specifies the None SameSite attribute for OAuth2 cookies
	OAuth2CookieSameSiteNone OAuth2CookieSameSite = "None"
)

// OAuth2Provider specifies the configuration for OAuth2 extension provider.
//
// +kubebuilder:validation:XValidation:message="Either issuerURI, or both authorizationEndpoint and tokenEndpoint must be specified",rule="has(self.issuerURI) || (has(self.authorizationEndpoint) && has(self.tokenEndpoint))"
type OAuth2Provider struct {
	// BackendRef specifies the Backend to use for the OAuth2 provider.
	// +required
	BackendRef gwv1.BackendRef `json:"backendRef"`

	// AuthorizationEndpoint specifies the endpoint to redirect to for authorization in response to unauthorized requests.
	// Refer to https://datatracker.ietf.org/doc/html/rfc6749#section-3.1 for more details.
	// +optional
	AuthorizationEndpoint *HttpsUri `json:"authorizationEndpoint,omitempty"`

	// TokenEndpoint specifies the endpoint on the authorization server to retrieve the access token from.
	// Refer to https://datatracker.ietf.org/doc/html/rfc6749#section-3.2 for more details.
	// +optional
	TokenEndpoint *HttpsUri `json:"tokenEndpoint,omitempty"`

	// RedirectURI specifies the URL passed to the authorization endpoint.
	// Defaults to <request-scheme>://<host>/oauth2/redirect, where the URL scheme and host are derived from the original request.
	// Refer to https://datatracker.ietf.org/doc/html/rfc6749#section-3.1.2 for more details.
	// +optional
	RedirectURI *string `json:"redirectURI,omitempty"`

	// LogoutPath specifies the path to log out a user, clearing their credential cookies.
	// Defaults to /logout.
	// +optional
	//
	// +kubebuilder:default="/logout"
	// +kubebuilder:validation:MinLength=1
	LogoutPath string `json:"logoutPath,omitempty"`

	// ForwardAccessToken specifies whether to forward the access token to the backend service.
	// If set to true, the token is forwarded over a cookie named BearerToken and is also set in the Authorization header.
	// Defaults to false.
	// +optional
	ForwardAccessToken *bool `json:"forwardAccessToken,omitempty"`

	// List of OAuth scopes to be claimed in the authentication request.
	// Defaults to "user" scope if not specified.
	// When using OpenID, the "openid" scope must be included.
	// Refer to https://datatracker.ietf.org/doc/html/rfc6749#section-3.3 for more details.
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// Credentials specifies the Oauth2 client credentials to use for authentication.
	// +required
	Credentials OAuth2Credentials `json:"credentials"`

	// IssuerURI specifies the OpenID provider's issuer URL to discover the OpenID provider's configuration.
	// The Issuer must be a URI RFC 3986 [RFC3986] with a scheme component that must be https, a host component,
	// and optionally, port and path components and no query or fragment components.
	// It discovers the authorizationEndpoint, tokenEndpoint, and endSessionEndpoint if specified in the discovery response.
	// Refer to https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderConfig for more details.
	// Note that the OpenID provider configuration is cached and only refreshed periodically when the GatewayExtension object
	// is reprocessed.
	// +optional
	//
	// +kubebuilder:validation:Pattern=`^https://([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(:[0-9]{1,5})?(/[a-zA-Z0-9\-._~!$&'()*+,;=:@%]*)*/?$`
	IssuerURI *string `json:"issuerURI,omitempty"`

	// EndSessionEndpoint specifies the URL that redirects a user's browser to in order to initiate a single logout
	// across all applications and the OpenID provider. Users are directed to this endpoint when they access the logout path.
	// This should only be set when the OpenID provider supports RP-Initiated Logout and "openid" is included in the list of scopes.
	// Refer to https://openid.net/specs/openid-connect-rpinitiated-1_0.html#RPLogout for more details.
	// +optional
	EndSessionEndpoint *HttpsUri `json:"endSessionEndpoint,omitempty"`

	// Cookies specifies the configuration for the OAuth2 cookies.
	// +optional
	Cookies *OAuth2CookieConfig `json:"cookies,omitempty"`

	// DenyRedirectMatcher specifies the matcher to match requests that should be denied redirects to the authorization endpoint.
	// Matching requests will receive a 401 Unauthorized response instead of being redirected.
	// This is useful for AJAX requests where redirects should be avoided.
	// +optional
	DenyRedirect *OAuth2DenyRedirectMatcher `json:"denyRedirect,omitempty"`
}

type OAuth2CookieConfig struct {
	// CookieDomain specifies the domain to set on the access and ID token cookies.
	// If set, the cookies will be set for the specified domain and all its subdomains. This is useful when requests
	// to subdomains are not required to be re-authenticated after the user has logged into the parent domain.
	// If not set, the cookies will default to the host of the request, not including the subdomains.
	// +optional
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9]))*$`
	Domain *string `json:"domain,omitempty"`

	// Names specifies the names of the cookies used to store the tokens.
	// If not set, the default names will be used.
	// +optional
	Names *OAuth2CookieNames `json:"names,omitempty"`

	// SameSite specifies the SameSite attribute for the OAuth2 cookies.
	// If not set, the default is Lax.
	// +optional
	SameSite *OAuth2CookieSameSite `json:"sameSite,omitempty"`

	// DisableAccessTokenSetCookie specifies whether to disable setting the access token cookie.
	// This can be used when the access token is too large to fit in a cookie. When true, the
	// set-cookie response header for the access token will be omitted.
	// +optional
	DisableAccessTokenSetCookie *bool `json:"disableAccessTokenSetCookie,omitempty"`

	// DisableIDTokenSetCookie specifies whether to disable setting the ID token cookie.
	// This can be used when the ID token is too large to fit in a cookie. When true, the
	// set-cookie response header for the ID token will be omitted.
	// +optional
	DisableIDTokenSetCookie *bool `json:"disableIDTokenSetCookie,omitempty"`

	// DisableRefreshTokenSetCookie specifies whether to disable setting the refresh token cookie.
	// This can be used when the refresh token is too large to fit in a cookie. When true, the
	// set-cookie response header for the refresh token will be omitted.
	// +optional
	DisableRefreshTokenSetCookie *bool `json:"disableRefreshTokenSetCookie,omitempty"`
}

// OAuth2Credentials specifies the Oauth2 client credentials.
type OAuth2Credentials struct {
	// ClientID specifies the client ID issued to the client during the registration process.
	// Refer to https://datatracker.ietf.org/doc/html/rfc6749#section-2.3.1 for more details.
	// +required
	//
	// +kubebuilder:validation:MinLength=1
	ClientID string `json:"clientID"`

	// ClientSecretRef specifies a Secret that contains the client secret stored in the key 'client-secret'
	// to use in the authentication request to obtain the access token.
	// Refer to https://datatracker.ietf.org/doc/html/rfc6749#section-2.3.1 for more details.
	// +required
	ClientSecretRef corev1.LocalObjectReference `json:"clientSecretRef"`
}

// OAuth2CookieNames specifies the names of the cookies used to store the tokens.
type OAuth2CookieNames struct {
	// AccessToken specifies the name of the cookie used to store the access token.
	// +optional
	//
	// +kubebuilder:validation:MinLength=1
	AccessToken *string `json:"accessToken,omitempty"`

	// IDToken specifies the name of the cookie used to store the ID token.
	// +optional
	//
	// +kubebuilder:validation:MinLength=1
	IDToken *string `json:"idToken,omitempty"`
}

// OAuth2DenyRedirectMatcher specifies the matcher to match requests that should be denied redirects to the authorization endpoint.
type OAuth2DenyRedirectMatcher struct {
	// Headers specifies the list of HTTP headers to match on requests that should be denied redirects.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Headers []gwv1.HTTPHeaderMatch `json:"headers,omitempty"`
}

// OAuth2Policy specifies the OAuth2 policy to apply to requests.
type OAuth2Policy struct {
	// ExtensionRef specifies the GatewayExtension that should be used for OAuth2.
	// +required
	ExtensionRef shared.NamespacedObjectReference `json:"extensionRef"`
}
