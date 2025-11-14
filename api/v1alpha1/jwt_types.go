package v1alpha1

import corev1 "k8s.io/api/core/v1"

// JWTValidation defines the providers used to configure JWT validation
type JWTValidation struct {
	// ExtensionRef references a GatewayExtension that provides the jwt providers
	// +required
	ExtensionRef *NamespacedObjectReference `json:"extensionRef"`

	// TODO: add support for ValidationMode here (REQUIRE_VALID,ALLOW_MISSING,ALLOW_MISSING_OR_FAILED)

	// TODO(npolshak): Add option to disable all jwt filters.
}

// JWTProvider configures the JWT Provider
// If multiple providers are specified for a given JWT policy, the providers will be `OR`-ed together and will allow validation to any of the providers.
type JWTProvider struct {
	// Issuer of the JWT. the 'iss' claim of the JWT must match this.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	Issuer string `json:"issuer"`

	// Audiences is the list of audiences to be used for the JWT provider.
	// If specified an incoming JWT must have an 'aud' claim, and it must be in this list.
	// If not specified, the audiences will not be checked in the token.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +optional
	Audiences []string `json:"audiences,omitempty"`

	// TokenSource configures where to find the JWT of the current provider.
	// +optional
	TokenSource *JWTTokenSource `json:"tokenSource,omitempty"`

	// ClaimsToHeaders is the list of claims to headers to be used for the JWT provider.
	// Optionally set the claims from the JWT payload that you want to extract and add as headers
	// to the request before the request is forwarded to the upstream destination.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +optional
	ClaimsToHeaders []JWTClaimToHeader `json:"claimsToHeaders,omitempty"`

	// JWKS is the source for the JSON Web Keys to be used to validate the JWT.
	JWKS JWKS `json:"jwks"`

	// KeepToken configures if the token is forwarded upstream.
	// If Remove, the header containing the token will be removed.
	// If Forward, the header containing the token will be forwarded upstream.
	// +kubebuilder:validation:Enum=Forward;Remove
	// +kubebuilder:default=Remove
	// +optional
	KeepToken *KeepToken `json:"keepToken,omitempty"`
}

// KeepToken configures if the token is forwarded upstream.
type KeepToken string

const (
	TokenForward KeepToken = "Forward"
	TokenRemove  KeepToken = "Remove"
)

// HeaderSource configures how to retrieve a JWT from a header
type HeaderSource struct {
	// Header is the name of the header. for example, "Authorization"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	Header *string `json:"header,omitempty"`
	// Prefix before the token. for example, "Bearer "
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	Prefix *string `json:"prefix,omitempty"`
}

// JWTTokenSource configures the source for the JWTToken
type JWTTokenSource struct {
	// HeaderSource configures retrieving token from the headers
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +optional
	HeaderSource []HeaderSource `json:"headers,omitempty"`
	// QueryParams configures retrieving token from these query params
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +optional
	QueryParams []string `json:"queryParams,omitempty"`
}

// JWTClaimToHeader allows copying verified claims to headers sent upstream
type JWTClaimToHeader struct {
	// Name is the JWT claim name, for example, "sub".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	Name string `json:"name"`

	// Header is the header the claim will be copied to, for example, "x-sub".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	Header string `json:"header"`
}

// JWKS (JSON Web Key Set) configures the source for the JWKS
type JWKS struct {
	// LocalJWKS configures getting the public keys to validate the JWT from a Kubernetes configmap,
	// or inline (raw string) JWKS.
	// +optional
	LocalJWKS *LocalJWKS `json:"local,omitempty"`

	// TODO: Add support for remote JWKS
}

// LocalJWKS configures getting the public keys to validate the JWT from a Kubernetes ConfigMap,
// or inline (raw string) JWKS.
// +kubebuilder:validation:ExactlyOneOf=inline;configMapRef
type LocalJWKS struct {
	// Inline is the JWKS as the raw, inline JWKS string
	// This can be an individual key, a key set or a pem block public key
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	Inline *string `json:"inline,omitempty"`

	// ConfigMapRef configures storing the JWK in a Kubernetes ConfigMap in the same namespace as the JWTValidationPolicy.
	// +optional
	ConfigMapRef *corev1.LocalObjectReference `json:"configMapRef,omitempty"`
}
