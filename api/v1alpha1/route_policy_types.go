package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=routepolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=routepolicies/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="gateway.networking.k8s.io/policy=Direct"
type RoutePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoutePolicySpec `json:"spec,omitempty"`
	Status PolicyStatus    `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type RoutePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RoutePolicy `json:"items"`
}

type RoutePolicySpec struct {

	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	TargetRefs []LocalPolicyTargetReference `json:"targetRefs,omitempty"`

	AI *AIRoutePolicy `json:"ai,omitempty"`

	Transformation TransformationPolicy `json:"transformation,omitempty"`

	ExtAuth *ExtAuthRoutePolicy `json:"extAuth,omitempty"`
}

// TransformationPolicy config is used to modify envoy behavior at a route level.
// These modifications can be performed on the request and response paths.
type TransformationPolicy struct {
	// +optional
	Request *Transform `json:"request,omitempty"`
	// +optional
	Response *Transform `json:"response,omitempty"`
}

// Transform defines the operations to be performed by the transformation.
// These operations may include changing the actual request/response but may also cause side effects.
// Side effects may include setting info that can be used in future steps (e.g. dynamic metadata) and can cause envoy to buffer.
type Transform struct {
	// Set is a list of headers and the value they should be set to.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	Set []HeaderTransformation `json:"set,omitempty"`

	// Add is a list of headers to add to the request and what that value should be set to.
	// If there is already a header with these values then append the value as an extra entry.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	Add []HeaderTransformation `json:"add,omitempty"`

	// Remove is a list of header names to remove from the request/response.
	// +optional
	// +listType=set
	// +kubebuilder:validation:MaxItems=16
	Remove []string `json:"remove,omitempty"`

	// Body controls both how to parse the body and if needed how to set.
	// +optional
	//
	// If empty, body will not be buffered.
	Body *BodyTransformation `json:"body,omitempty"`
}

type InjaTemplate string

// EnvoyHeaderName is the name of a header or pseudo header
// Based on gateway api v1.Headername but allows a singular : at the start
//
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=256
// +kubebuilder:validation:Pattern=`^:?[A-Za-z0-9!#$%&'*+\-.^_\x60|~]+$`
// +k8s:deepcopy-gen=false
type HeaderName string
type HeaderTransformation struct {
	// Name is the name of the header to interact with.
	// +required
	Name HeaderName `json:"name,omitempty"`
	// Value is the template to apply to generate the output value for the header.
	Value InjaTemplate `json:"value,omitempty"`
}

// BodyparseBehavior defines how the body should be parsed
// If set to json and the body is not json then the filter will not perform the transformation.
// +kubebuilder:validation:Enum=AsString;AsJson
type BodyParseBehavior string

const (
	BodyParseBehaviorAsString BodyParseBehavior = "AsString"
	BodyParseBehaviorAsJSON   BodyParseBehavior = "AsJson"
)

// BodyTransformation controls how the body should be parsed and transformed.
type BodyTransformation struct {
	// ParseAs defines what auto formatting should be applied to the body.
	// This can make interacting with keys within a json body much easier if AsJson is selected.
	// +kubebuilder:default=AsString
	ParseAs BodyParseBehavior `json:"parseAs"`
	// Value is the template to apply to generate the output value for the body.
	Value *InjaTemplate `json:"value,omitempty"`
}

// ExtAuthRoutePolicy configures external authentication for a route.
// This policy will determine the ext auth server to use and how to  talk to it.
// Note that most of these fields are passed along as is to Envoy.
// For more details on particular fields please see the Envoy ExtAuth documentation.
// https://raw.githubusercontent.com/envoyproxy/envoy/f910f4abea24904aff04ec33a00147184ea7cffa/api/envoy/extensions/filters/http/ext_authz/v3/ext_authz.proto
// +kubebuilder:validation:XValidation:message="only one of 'providerRef' or 'disabled' may be set",rule="(has(self.providerRef) && !self.disabled) || (!has(self.providerRef) && self.disabled)"
type ExtAuthRoutePolicy struct {
	// ProviderRef references the ExternalProvider that should be used for authentication.
	// +optional
	ProviderRef *gwv1.LocalObjectReference `json:"providerRef,omitempty"`

	// Disabled will disable the ext auth filter for this route.
	// +optional
	// +kubebuilder:default=false
	Disabled bool `json:"disabled,omitempty"`

	// FailureModeAllow determines the behavior on authorization service errors.
	// When true, requests will be allowed even if the authorization service fails or returns HTTP 5xx errors.
	// +optional
	// +kubebuilder:default=false
	FailureModeAllow bool `json:"failureModeAllow,omitempty"`

	// WithRequestBody allows the request body to be buffered and sent to the authorization service.
	// Warning buffering has implications for streaming and therefore performance.
	// +optional
	WithRequestBody *BufferSettings `json:"withRequestBody,omitempty"`

	// ClearRouteCache allows the authorization service to affect routing decisions.
	// +kubebuilder:default=false
	ClearRouteCache bool `json:"clearRouteCache,omitempty"`

	// MetadataContextNamespaces specifies metadata namespaces to pass to the authorization service.
	// Default to allowing jwt info if processing for jwt is configured.
	// +optional
	// +listType=set
	// +kubebuilder:default=["jwt"]
	MetadataContextNamespaces []string `json:"metadataContextNamespaces,omitempty"`

	// IncludePeerCertificate determines if the client's X.509 certificate should be sent to the authorization service.
	// When true, the certificate will be included if available.
	// +optional
	// +kubebuilder:default=false
	IncludePeerCertificate bool `json:"includePeerCertificate,omitempty"`

	// IncludeTLSSession determines if TLS session details should be sent to the authorization service.
	// When true, the SNI name from TLSClientHello will be included if available.
	// +optional
	// +kubebuilder:default=false
	IncludeTLSSession bool `json:"includeTLSSession,omitempty"`
}

// BufferSettings configures how the request body should be buffered.
type BufferSettings struct {
	// MaxRequestBytes sets the maximum size of a message body to buffer.
	// Requests exceeding this size will receive HTTP 413 and not be sent to the authorization service.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	MaxRequestBytes uint32 `json:"maxRequestBytes"`

	// AllowPartialMessage determines if partial messages should be allowed.
	// When true, requests will be sent to the authorization service even if they exceed maxRequestBytes.
	// +optional
	// +kubebuilder:default=false
	AllowPartialMessage bool `json:"allowPartialMessage,omitempty"`

	// PackAsBytes determines if the body should be sent as raw bytes.
	// When true, the body is sent as raw bytes in the raw_body field.
	// When false, the body is sent as UTF-8 string in the body field.
	// +optional
	// +kubebuilder:default=false
	PackAsBytes bool `json:"packAsBytes,omitempty"`
}
