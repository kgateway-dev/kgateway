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

// OAuth2Policy specifies the OAuth2 policy to apply to requests.
type OAuth2Policy struct {
	// ExtensionRef specifies the GatewayExtension that should be used for OAuth2.
	// +required
	ExtensionRef shared.NamespacedObjectReference `json:"extensionRef"`
}
