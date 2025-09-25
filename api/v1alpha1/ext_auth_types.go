package v1alpha1

// ExtAuthPolicy configures external authentication/authorization for a route.
// This policy will determine the ext auth server to use and how to talk to it.
// Note that most of these fields are passed along as is to Envoy.
// For more details on particular fields please see the Envoy ExtAuth documentation.
// https://raw.githubusercontent.com/envoyproxy/envoy/f910f4abea24904aff04ec33a00147184ea7cffa/api/envoy/extensions/filters/http/ext_authz/v3/ext_authz.proto
//
// +kubebuilder:validation:ExactlyOneOf=extensionRef;disable
type ExtAuthPolicy struct {
	// ExtensionRef references the GatewayExtension that should be used for auth.
	// +optional
	ExtensionRef *NamespacedObjectReference `json:"extensionRef,omitempty"`

	// WithRequestBody allows the request body to be buffered and sent to the auth service.
	// Warning buffering has implications for streaming and therefore performance.
	// +optional
	WithRequestBody *ExtAuthBufferSettings `json:"withRequestBody,omitempty"`

	// Additional context for the auth service.
	// +optional
	ContextExtensions map[string]string `json:"contextExtensions,omitempty"`

	// Disable all external auth filters.
	// Can be used to disable external auth policies applied at a higher level in the config hierarchy.
	// +optional
	Disable *PolicyDisable `json:"disable,omitempty"`
}

// ExtAuthBufferSettings configures how the request body should be buffered.
type ExtAuthBufferSettings struct {
	// MaxRequestBytes sets the maximum size of a message body to buffer.
	// Requests exceeding this size will receive HTTP 413 and not be sent to the auth service.
	// +required
	// +kubebuilder:validation:Minimum=1
	MaxRequestBytes int32 `json:"maxRequestBytes"`

	// AllowPartialMessage determines if partial messages should be allowed.
	// When true, requests will be sent to the auth service even if they exceed maxRequestBytes.
	// The default behavior is false.
	// +optional
	// +kubebuilder:default=false
	AllowPartialMessage bool `json:"allowPartialMessage,omitempty"`

	// PackAsBytes determines if the body should be sent as raw bytes.
	// When true, the body is sent as raw bytes in the raw_body field.
	// When false, the body is sent as UTF-8 string in the body field.
	// The default behavior is false.
	// +optional
	// +kubebuilder:default=false
	PackAsBytes bool `json:"packAsBytes,omitempty"`
}

// ExtAuthProvider defines the configuration for an ExtAuth provider.
type ExtAuthProvider struct {
	// GrpcService is the GRPC service that will handle the auth.
	// +required
	GrpcService *ExtGrpcService `json:"grpcService"`

	// FailOpen determines if requests are allowed when the ext auth service is unavailable.
	// Defaults to false, meaning requests will be denied if the ext auth service is unavailable.
	// +optional
	// +kubebuilder:default=false
	FailOpen bool `json:"failOpen,omitempty"`

	// ClearRouteCache determines if the route cache should be cleared to allow the
	// external authentication service to correctly affect routing decisions.
	// +optional
	// +kubebuilder:default=false
	ClearRouteCache bool `json:"clearRouteCache,omitempty"`

	// WithRequestBody allows the request body to be buffered and sent to the auth service.
	// Warning: buffering has implications for streaming and therefore performance.
	// +optional
	WithRequestBody *ExtAuthBufferSettings `json:"withRequestBody,omitempty"`

	// StatusOnError sets the HTTP status response code that is returned to the client when the
	// auth server returns an error or cannot be reached. Must be in the range of 100-511 inclusive.
	// The default matches the deny response code of 403 Forbidden.
	// +optional
	// +kubebuilder:default=403
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=511
	StatusOnError int32 `json:"statusOnError,omitempty"`

	// StatPrefix is an optional prefix to include when emitting stats from the extauthz filter,
	// enabling different instances of the filter to have unique stats.
	// +optional
	// +kubebuilder:validation:MinLength=1
	StatPrefix *string `json:"statPrefix,omitempty"`
}
