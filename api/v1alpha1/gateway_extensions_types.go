package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=gatewayextensions,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=gatewayextensions/status,verbs=get;update;patch

// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=".spec.type",description="Which extension type?"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp",description="The age of the gatewayextension."

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
type GatewayExtension struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayExtensionSpec   `json:"spec,omitempty"`
	Status GatewayExtensionStatus `json:"status,omitempty"`
}

// GatewayExtensionType indicates the type of the GatewayExtension.
type GatewayExtensionType string

const (
	// GatewayExtensionTypeExtAuth is the type for Extauth extensions.
	GatewayExtensionTypeExtAuth GatewayExtensionType = "ExtAuth"
	// GatewayExtensionTypeExtProc is the type for ExtProc extensions.
	GatewayExtensionTypeExtProc GatewayExtensionType = "ExtProc"
	// GatewayExtensionTypeRateLimit is the type for RateLimit extensions.
	GatewayExtensionTypeRateLimit GatewayExtensionType = "RateLimit"
)

// ExtAuthProvider defines the configuration for an ExtAuth provider.
type ExtAuthProvider struct {
	// GrpcService is the GRPC service that will handle the authentication.
	// +required
	GrpcService *ExtGrpcService `json:"grpcService"`

	// FailOpen determines if requests are allowed when the ext auth service is unavailable.
	// Defaults to false, meaning requests will be denied if the ext auth service is unavailable.
	// +optional
	// +kubebuilder:default=false
	FailOpen bool `json:"failOpen"`

	// ClearRouteCache determines if the route cache should be cleared to allow the
	// external authorization service to correctly affect routing decisions.
	// +optional
	// +kubebuilder:default=false
	ClearRouteCache bool `json:"clearRouteCache"`

	// WithRequestBody allows the request body to be buffered and sent to the authorization service.
	// Warning: buffering has implications for streaming and therefore performance.
	// +optional
	WithRequestBody *BufferSettings `json:"withRequestBody,omitempty"`

	// StatusOnError sets the HTTP status response code that is returned to the client when the
	// authorization server returns an error or cannot be reached. Must be in the range of 100-511 inclusive.
	// The default matches the deny response code of 403 Forbidden.
	// +optional
	// +kubebuilder:default=403
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=511
	StatusOnError uint32 `json:"statusOnError"`

	// StatPrefix is an optional prefix to include when emitting stats from the extauth filter,
	// enabling different instances of the filter to have unique stats.
	// +optional
	// +kubebuilder:validation:MinLength=1
	StatPrefix *string `json:"statPrefix,omitempty"`
}

// ExtProcProvider defines the configuration for an ExtProc provider.
type ExtProcProvider struct {
	// GrpcService is the GRPC service that will handle the processing.
	// +required
	GrpcService *ExtGrpcService `json:"grpcService"`

	// FailOpen determines if requests are allowed when the ext proc service is unavailable.
	// Defaults to true, meaning requests are allowed upstream even if the ext proc service is unavailable.
	// +optional
	// +kubebuilder:default=true
	FailOpen bool `json:"failOpen"`
}

// ExtGrpcService defines the GRPC service that will handle the processing.
type ExtGrpcService struct {
	// BackendRef references the backend GRPC service.
	// +required
	BackendRef *gwv1.BackendRef `json:"backendRef"`

	// Authority is the authority header to use for the GRPC service.
	// +optional
	Authority *string `json:"authority,omitempty"`

	// RequestTimeout is the timeout for the gRPC request. This is the timeout for a specific request.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid timeout value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1ms')",message="timeout must be at least 1ms."
	RequestTimeout *metav1.Duration `json:"requestTimeout,omitempty"`
}

// RateLimitProvider defines the configuration for a RateLimit service provider.
type RateLimitProvider struct {
	// GrpcService is the GRPC service that will handle the rate limiting.
	// +required
	GrpcService *ExtGrpcService `json:"grpcService"`

	// Domain identifies a rate limiting configuration for the rate limit service.
	// All rate limit requests must specify a domain, which enables the configuration
	// to be per application without fear of overlap (e.g., "api", "web", "admin").
	// +required
	Domain string `json:"domain"`

	// FailOpen determines if requests are limited when the rate limit service is unavailable.
	// Defaults to true, meaning requests are allowed upstream and not limited if the rate limit service is unavailable.
	// +optional
	// +kubebuilder:default=true
	FailOpen bool `json:"failOpen"`

	// Timeout provides an optional timeout value for requests to the rate limit service.
	// For rate limiting, prefer using this timeout rather than setting the generic `timeout` on the `GrpcService`.
	// See [envoy issue](https://github.com/envoyproxy/envoy/issues/20070) for more info.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:default="100ms"
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// XRateLimitHeaders configures the standard version to use for X-RateLimit headers emitted.
	// See [envoy docs](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ratelimit/v3/rate_limit.proto#envoy-v3-api-field-extensions-filters-http-ratelimit-v3-ratelimit-enable-x-ratelimit-headers) for more info.
	// Disabled by default.
	// +kubebuilder:validation:Enum=Off;DraftVersion03
	// +kubebuilder:default="Off"
	XRateLimitHeaders XRLHeadersStandard `json:"xRateLimitHeaders,omitempty"`
}

// XRLHeadersStandard controls how XRateLimit headers will emitted.
type XRLHeadersStandard string

const (
	// XRLHeaderOff disables emitting of XRateLimit headers.
	XRLHeaderOff XRLHeadersStandard = "Off"
	// XRLHeaderDraftV03 outputs headers as described in [draft RFC version 03](https://tools.ietf.org/id/draft-polli-ratelimit-headers-03.html).
	XRLHeaderDraftV03 XRLHeadersStandard = "DraftVersion03"
)

// GatewayExtensionSpec defines the desired state of GatewayExtension.
// +kubebuilder:validation:XValidation:message="ExtAuth must be set when type is ExtAuth",rule="self.type != 'ExtAuth' || has(self.extAuth)"
// +kubebuilder:validation:XValidation:message="ExtProc must be set when type is ExtProc",rule="self.type != 'ExtProc' || has(self.extProc)"
// +kubebuilder:validation:XValidation:message="RateLimit must be set when type is RateLimit",rule="self.type != 'RateLimit' || has(self.rateLimit)"
// +kubebuilder:validation:XValidation:message="ExtAuth must not be set when type is not ExtAuth",rule="self.type == 'ExtAuth' || !has(self.extAuth)"
// +kubebuilder:validation:XValidation:message="ExtProc must not be set when type is not ExtProc",rule="self.type == 'ExtProc' || !has(self.extProc)"
// +kubebuilder:validation:XValidation:message="RateLimit must not be set when type is not RateLimit",rule="self.type == 'RateLimit' || !has(self.rateLimit)"
type GatewayExtensionSpec struct {
	// Type indicates the type of the GatewayExtension to be used.
	// +unionDiscriminator
	// +kubebuilder:validation:Enum=ExtAuth;ExtProc;RateLimit
	// +required
	Type GatewayExtensionType `json:"type"`

	// ExtAuth configuration for ExtAuth extension type.
	// +optional
	// +unionMember:type=ExtAuth
	ExtAuth *ExtAuthProvider `json:"extAuth,omitempty"`

	// ExtProc configuration for ExtProc extension type.
	// +optional
	// +unionMember:type=ExtProc
	ExtProc *ExtProcProvider `json:"extProc,omitempty"`

	// RateLimit configuration for RateLimit extension type.
	// +optional
	// +unionMember:type=RateLimit
	RateLimit *RateLimitProvider `json:"rateLimit,omitempty"`
}

// GatewayExtensionStatus defines the observed state of GatewayExtension.
type GatewayExtensionStatus struct {
	// Conditions is the list of conditions for the GatewayExtension.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type GatewayExtensionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayExtension `json:"items"`
}
