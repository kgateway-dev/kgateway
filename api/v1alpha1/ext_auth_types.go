package v1alpha1

// ExtAuthPolicy configures external authentication for a route.
// This policy will determine the ext auth server to use and how to  talk to it.
// Note that most of these fields are passed along as is to Envoy.
// For more details on particular fields please see the Envoy ExtAuth documentation.
// https://raw.githubusercontent.com/envoyproxy/envoy/f910f4abea24904aff04ec33a00147184ea7cffa/api/envoy/extensions/filters/http/ext_authz/v3/ext_authz.proto
//
// +kubebuilder:validation:ExactlyOneOf=extensionRef;disable
type ExtAuthPolicy struct {
	// ExtensionRef references the GatewayExtension that should be used for authentication.
	// +optional
	ExtensionRef *NamespacedObjectReference `json:"extensionRef,omitempty"`

	// WithRequestBody allows the request body to be buffered and sent to the authorization service.
	// Warning buffering has implications for streaming and therefore performance.
	// +optional
	WithRequestBody *ExtAuthBufferSettings `json:"withRequestBody,omitempty"`

	// Additional context for the authorization service.
	// +optional
	ContextExtensions map[string]string `json:"contextExtensions,omitempty"`

	// Disable all external authorization filters.
	// Can be used to disable external authorization policies applied at a higher level in the config hierarchy.
	// +optional
	Disable *PolicyDisable `json:"disable,omitempty"`
}

// ExtAuthBufferSettings configures how the request body should be buffered.
type ExtAuthBufferSettings struct {
	// MaxRequestBytes sets the maximum size of a message body to buffer.
	// Requests exceeding this size will receive HTTP 413 and not be sent to the authorization service.
	// +required
	// +kubebuilder:validation:Minimum=1
	MaxRequestBytes uint32 `json:"maxRequestBytes"`

	// AllowPartialMessage determines if partial messages should be allowed.
	// When true, requests will be sent to the authorization service even if they exceed maxRequestBytes.
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
