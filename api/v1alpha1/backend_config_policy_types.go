package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backendconfigpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backendconfigpolicies/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
type BackendConfigPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BackendConfigPolicySpec `json:"spec,omitempty"`
	Status            gwv1alpha2.PolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type BackendConfigPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackendConfigPolicy `json:"items"`
}

type BackendConfigPolicySpec struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	TargetRefs []LocalPolicyTargetReference `json:"targetRefs,omitempty"`

	// TargetSelectors specifies the target selectors to select resources to attach the policy to.
	// +optional
	TargetSelectors []LocalPolicyTargetSelector `json:"targetSelectors,omitempty"`

	// The timeout for new network connections to hosts in the cluster.
	// +optional
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('0s')",message="connectTimeout must be a valid duration string"
	ConnectTimeout *metav1.Duration `json:"connectTimeout,omitempty"`

	// Soft limit on size of the cluster's connections read and write buffers.
	// If unspecified, an implementation defined default is applied (1MiB).
	// +optional
	PerConnectionBufferLimitBytes *int `json:"perConnectionBufferLimitBytes,omitempty"`

	// Configure OS-level TCP keepalive checks.
	// +optional
	TCPKeepalive *TCPKeepalive `json:"tcpKeepalive,omitempty"`

	// Additional options when handling HTTP requests upstream, applicable to
	// both HTTP1 and HTTP2 requests.
	// +optional
	CommonHttpProtocolOptions *CommonHttpProtocolOptions `json:"commonHttpProtocolOptions,omitempty"`

	// Additional options when handling HTTP1 requests upstream.
	// +optional
	Http1ProtocolOptions *Http1ProtocolOptions `json:"http1ProtocolOptions,omitempty"`
}

// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-msg-config-core-v3-http1protocoloptions) for more details.
type Http1ProtocolOptions struct {
	// Enables trailers for HTTP/1. By default the HTTP/1 codec drops proxied trailers.
	// Note: Trailers must also be enabled at the gateway level in order for this option to take effect
	// +optional
	EnableTrailers *bool `json:"enableTrailers,omitempty"`

	// The format of the header key.
	// +optional
	// +kubebuilder:validation:Enum=ProperCaseHeaderKeyFormat;PreserveCaseHeaderKeyFormat
	HeaderFormat *HeaderFormat `json:"headerFormat,omitempty"`

	// Allows invalid HTTP messaging. When this option is false, then Envoy will terminate
	// HTTP/1.1 connections upon receiving an invalid HTTP message. However,
	// when this option is true, then Envoy will leave the HTTP/1.1 connection
	// open where possible.
	// +optional
	OverrideStreamErrorOnInvalidHttpMessage *bool `json:"overrideStreamErrorOnInvalidHttpMessage,omitempty"`
}

const (
	ProperCaseHeaderKeyFormat   HeaderFormat = "ProperCaseHeaderKeyFormat"
	PreserveCaseHeaderKeyFormat HeaderFormat = "PreserveCaseHeaderKeyFormat"
)

type HeaderFormat string

// CommonHttpProtocolOptions are options that are applicable to both HTTP1 and HTTP2 requests.
// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-msg-config-core-v3-httpprotocoloptions) for more details.
// +kubebuilder:validation:XValidation:message="idleTimeout must be a valid duration string (e.g. \"1s\", \"500ms\")",rule="(!has(self.idleTimeout) || (has(self.idleTimeout) && self.idleTimeout.matches('^([0-9]{1,5}(h|m|s|ms)){1,4}$')))"
// +kubebuilder:validation:XValidation:message="maxStreamDuration must be a valid duration string (e.g. \"1s\", \"500ms\")",rule="(!has(self.maxStreamDuration) || (has(self.maxStreamDuration) && self.maxStreamDuration.matches('^([0-9]{1,5}(h|m|s|ms)){1,4}$')))"
type CommonHttpProtocolOptions struct {
	// The idle timeout for connections. The idle timeout is defined as the
	// period in which there are no active requests. When the
	// idle timeout is reached the connection will be closed. If the connection is an HTTP/2
	// downstream connection a drain sequence will occur prior to closing the connection.
	// Note that request based timeouts mean that HTTP/2 PINGs will not keep the connection alive.
	// If not specified, this defaults to 1 hour. To disable idle timeouts explicitly set this to 0.
	//	Disabling this timeout has a highly likelihood of yielding connection leaks due to lost TCP
	//	FIN packets, etc.
	// +optional
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('0s')",message="idleTimeout must be a valid duration string"
	IdleTimeout *metav1.Duration `json:"idleTimeout,omitempty"`

	// Specifies the maximum number of headers that the connection will accept.
	// If not specified, the default of 100 is used. Requests that exceed this limit will receive
	// a 431 response for HTTP/1.x and cause a stream reset for HTTP/2.
	// +optional
	MaxHeadersCount *int `json:"maxHeadersCount,omitempty"`

	// Total duration to keep alive an HTTP request/response stream. If the time limit is reached the stream will be
	// reset independent of any other timeouts. If not specified, this value is not set.
	// +optional
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('0s')",message="maxStreamDuration must be a valid duration string"
	MaxStreamDuration *metav1.Duration `json:"maxStreamDuration,omitempty"`

	// Action to take when a client request with a header name containing underscore characters is received.
	// If this setting is not specified, the value defaults to ALLOW.
	// Note: upstream responses are not affected by this setting.
	// +optional
	HeadersWithUnderscoresAction *HeadersWithUnderscoresAction `json:"headersWithUnderscoresAction,omitempty"`

	// Maximum requests for a single upstream connection.
	// If set to 0 or unspecified, defaults to unlimited.
	// +optional
	MaxRequestsPerConnection *int `json:"maxRequestsPerConnection,omitempty"`
}

// +kubebuilder:validation:Enum=Allow;RejectRequest;DropHeader
type HeadersWithUnderscoresAction string

const (
	// Allow headers with underscores. This is the default behavior.
	HeadersWithUnderscoresActionAllow HeadersWithUnderscoresAction = "Allow"
	// Reject client request. HTTP/1 requests are rejected with the 400 status. HTTP/2 requests
	// end with the stream reset. The "httpN.requests_rejected_with_underscores_in_headers" counter
	// is incremented for each rejected request.
	HeadersWithUnderscoresActionRejectRequest HeadersWithUnderscoresAction = "RejectRequest"
	// Drop the header with name containing underscores. The header is dropped before the filter chain is
	// invoked and as such filters will not see dropped headers. The
	// "httpN.dropped_headers_with_underscores" is incremented for each dropped header.
	HeadersWithUnderscoresActionDropHeader HeadersWithUnderscoresAction = "DropHeader"
)

// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/address.proto#envoy-v3-api-msg-config-core-v3-tcpkeepalive) for more details.
type TCPKeepalive struct {
	// Maximum number of keep-alive probes to send before dropping the connection.
	// +optional
	KeepAliveProbes *int `json:"keepAliveProbes,omitempty"`

	// The number of seconds a connection needs to be idle before keep-alive probes start being sent.
	// +optional
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('0s')",message="keepAliveTime must be a valid duration string"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="keepAliveTime must be at least 1 second"
	KeepAliveTime *metav1.Duration `json:"keepAliveTime,omitempty"`

	// The number of seconds between keep-alive probes.
	// +optional
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('0s')",message="keepAliveInterval must be a valid duration string"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="keepAliveInterval must be at least 1 second"
	KeepAliveInterval *metav1.Duration `json:"keepAliveInterval,omitempty"`
}
