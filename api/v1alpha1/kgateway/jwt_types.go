package kgateway

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

// JWTAuth defines the providers used to configure JWT authentication
// +kubebuilder:validation:ExactlyOneOf=extensionRef;disable
type JWTAuth struct {
	// ExtensionRef references a GatewayExtension that provides the jwt providers
	// +optional
	ExtensionRef *shared.NamespacedObjectReference `json:"extensionRef,omitempty"`

	// Disable all JWT filters.
	// Can be used to disable JWT policies applied at a higher level in the config hierarchy.
	// +optional
	Disable *shared.PolicyDisable `json:"disable,omitempty"`
}

// JWTProvider configures the JWT Provider
// If multiple providers are specified for a given JWT policy, the providers will be `OR`-ed together and will allow validation to any of the providers.
type JWTProvider struct {
	// Issuer of the JWT. the 'iss' claim of the JWT must match this.
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	Issuer string `json:"issuer"`

	// Audiences is the list of audiences to be used for the JWT provider.
	// If specified an incoming JWT must have an 'aud' claim, and it must be in this list.
	// If not specified, the audiences will not be checked in the token.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	// +optional
	Audiences []string `json:"audiences,omitempty"`

	// TokenSource configures where to find the JWT of the current provider.
	// +optional
	TokenSource *JWTTokenSource `json:"tokenSource,omitempty"`

	// ClaimsToHeaders is the list of claims to headers to be used for the JWT provider.
	// Optionally set the claims from the JWT payload that you want to extract and add as headers
	// to the request before the request is forwarded to the upstream destination.
	// Note: if ClaimsToHeaders is set, the Envoy route cache will be cleared.
	// This allows the JWT filter to correctly affect routing decisions.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	// +optional
	ClaimsToHeaders []JWTClaimToHeader `json:"claimsToHeaders,omitempty"`

	// JWKS is the source for the JSON Web Keys to be used to validate the JWT.
	// +required
	JWKS JWKS `json:"jwks"`

	// ForwardToken configures if the JWT token is forwarded to the upstream backend.
	// If true, the header containing the token will be forwarded upstream.
	// If false or not set, the header containing the token will be removed.
	// +optional
	ForwardToken *bool `json:"forwardToken,omitempty"`
}

// HeaderSource configures how to retrieve a JWT from a header
type HeaderSource struct {
	// Header is the name of the header. for example, "Authorization"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +required
	Header string `json:"header"`
	// Prefix before the token. for example, "Bearer "
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +optional
	Prefix *string `json:"prefix,omitempty"`
}

// JWTTokenSource configures the source for the JWTToken
// Exactly one of HeaderSource or QueryParameter must be specified.
// +kubebuilder:validation:ExactlyOneOf=header;queryParameter
type JWTTokenSource struct {
	// HeaderSource configures retrieving token from a header
	// +optional
	HeaderSource *HeaderSource `json:"header,omitempty"`
	// QueryParameter configures retrieving token from the query parameter
	// +optional
	QueryParameter *string `json:"queryParameter,omitempty"`
}

// JWTClaimToHeader allows copying verified claims to headers sent upstream
type JWTClaimToHeader struct {
	// Name is the JWT claim name, for example, "sub".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +required
	Name string `json:"name"`

	// Header is the header the claim will be copied to, for example, "x-sub".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +required
	Header string `json:"header"`
}

// JWKS (JSON Web Key Set) configures the source for the JWKS
// Exactly one of LocalJWKS or RemoteJWKS must be specified.
// +kubebuilder:validation:ExactlyOneOf=local;remote
type JWKS struct {
	// LocalJWKS configures getting the public keys to validate the JWT from a Kubernetes configmap,
	// or inline (raw string) JWKS.
	// +optional
	LocalJWKS *LocalJWKS `json:"local,omitempty"`

	// RemoteJWKS configures getting the public keys to validate the JWT from a remote JWKS server.
	// +optional
	RemoteJWKS *RemoteJWKS `json:"remote,omitempty"`
}

// LocalJWKS configures getting the public keys to validate the JWT from a Kubernetes ConfigMap,
// or inline (raw string) JWKS.
// +kubebuilder:validation:ExactlyOneOf=inline;configMapRef
type LocalJWKS struct {
	// Inline is the JWKS as the raw, inline JWKS string
	// This can be an individual key, a key set or a pem block public key
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=16384
	// +optional
	Inline *string `json:"inline,omitempty"`

	// ConfigMapRef configures storing the JWK in a Kubernetes ConfigMap in the same namespace as the GatewayExtension.
	// The ConfigMap must have a data key named 'jwks' that contains the JWKS.
	// +optional
	ConfigMapRef *corev1.LocalObjectReference `json:"configMapRef,omitempty"`
}

type RemoteJWKS struct {
	// URL is the URL of the remote JWKS server, it must be a full FQDN with protocol, host and path.
	// For example, https://example.com/keys
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +required
	URL string `json:"url"`

	// BackendRef is reference to the backend of the JWKS server.
	// +required
	BackendRef gwv1.BackendObjectReference `json:"backendRef"`

	// Duration after which the cached JWKS expires.
	// If unspecified, the default cache duration is 5 minutes.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1ms')",message="cacheDuration must be at least 1ms."
	CacheDuration *metav1.Duration `json:"cacheDuration,omitempty"`

	// AsyncFetch configures fetching the JWKS asynchronously and caching it on a timer,
	// instead of fetching it on demand during request handling.
	// +optional
	AsyncFetch *JWKSAsyncFetch `json:"asyncFetch,omitempty"`

	// RetryPolicy configures how the JWKS fetch is retried (with exponential backoff)
	// when the remote JWKS server is unavailable.
	// +optional
	RetryPolicy *JWKSRetryPolicy `json:"retryPolicy,omitempty"`
}

// JWKSAsyncFetch configures asynchronous fetching of the remote JWKS.
type JWKSAsyncFetch struct {
	// FastListener controls when the listener is considered ready relative to the
	// initial JWKS fetch.
	// If false or unset, the listener waits for the first JWKS fetch to complete before
	// it starts serving traffic, so requests are never validated against an empty key set.
	// If true, the listener starts immediately and the first fetch happens in the background.
	// +optional
	FastListener *bool `json:"fastListener,omitempty"`

	// FailedRefetchDuration is how long to wait before retrying the fetch after a failure.
	// If unspecified, Envoy default of 1 second is used.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1ms')",message="failedRefetchDuration must be at least 1ms."
	FailedRefetchDuration *metav1.Duration `json:"failedRefetchDuration,omitempty"`
}

// JWKSRetryPolicy configures retries with an exponential backoff for fetching
// the remote JWKS when the server is unavailable.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/base.proto#envoy-v3-api-msg-config-core-v3-retrypolicy
type JWKSRetryPolicy struct {
	// NumRetries is the allowed number of retries when fetching the JWKS fails.
	// Defaults to 1 if unset.
	// +optional
	// +kubebuilder:validation:Minimum=1
	NumRetries *int32 `json:"numRetries,omitempty"`

	// BackOff configures the exponential backoff strategy between retries.
	// If unset, the default base interval is 1000ms and the default maximum interval is
	// 10 times the base interval.
	// +optional
	BackOff *JWKSRetryBackOff `json:"backOff,omitempty"`
}

// JWKSRetryBackOff configures an exponential backoff strategy.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/backoff.proto#envoy-v3-api-msg-config-core-v3-backoffstrategy
// +kubebuilder:validation:XValidation:rule="!has(self.maxInterval) || duration(self.maxInterval) >= duration(self.baseInterval)",message="maxInterval must be greater than or equal to baseInterval"
type JWKSRetryBackOff struct {
	// BaseInterval is the base interval for the exponential backoff computation.
	// It must be greater than zero and less than or equal to MaxInterval.
	// +required
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1ms')",message="baseInterval must be at least 1ms."
	BaseInterval metav1.Duration `json:"baseInterval"`

	// MaxInterval is the maximum interval between retries. If set, it must be greater than
	// or equal to BaseInterval. Defaults to 10 times the BaseInterval.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1ms')",message="maxInterval must be at least 1ms."
	MaxInterval *metav1.Duration `json:"maxInterval,omitempty"`
}
