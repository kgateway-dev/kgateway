package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
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

	// SSLConfig contains the options necessary to configure a backend to use TLS origination.
	// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/tls.proto#envoy-v3-api-msg-extensions-transport-sockets-tls-v3-sslconfig) for more details.
	// +optional
	SSLConfig *SSLConfig `json:"sslConfig,omitempty"`
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

// +kubebuilder:validation:XValidation:rule="has(self.secretRef) != has(self.sslFiles)",message="Exactly one of secretRef or sslFiles must be set in SSLConfig"
type SSLConfig struct {
	// Reference to the TLS secret containing the certificate, key, and optionally the root CA.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// File paths to certificates local to the proxy.
	// +optional
	SSLFiles *SSLFiles `json:"sslFiles,omitempty"`

	// The SNI domains that should be considered for TLS connection
	// +optional
	Sni string `json:"sni,omitempty"`

	// Verify that the Subject Alternative Name in the peer certificate is one of the specified values.
	// note that a root_ca must be provided if this option is used.
	// +optional
	VerifySubjectAltName []string `json:"verifySubjectAltName,omitempty"`

	// General TLS parameters. See the [envoy docs](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/common.proto#extensions-transport-sockets-tls-v3-tlsparameters)
	// for more information on the meaning of these values.
	// +optional
	SSLParameters *SSLParameters `json:"sslParameters,omitempty"`

	// Set Application Level Protocol Negotiation
	// If empty, defaults to ["h2", "http/1.1"].
	// +optional
	AlpnProtocols []string `json:"alpnProtocols,omitempty"`

	// Allow Tls renegotiation, the default value is false.
	// TLS renegotiation is considered insecure and shouldn't be used unless absolutely necessary.
	// +optional
	AllowRenegotiation *bool `json:"allowRenegotiation,omitempty"`

	// If the SSL config has the ca.crt (root CA) provided, kgateway uses it to perform mTLS by default.
	// Set oneWayTls to true to disable mTLS in favor of server-only TLS (one-way TLS), even if kgateway has the root CA.
	// If unset, defaults to false.
	// +optional
	OneWayTLS *bool `json:"oneWayTLS,omitempty"`
}

// TLSVersion defines the TLS version.
// +kubebuilder:validation:Enum=AUTO;"1.0";"1.1";"1.2";"1.3"
type TLSVersion string

const (
	TLSVersionAUTO TLSVersion = "AUTO"
	TLSVersion1_0  TLSVersion = "1.0"
	TLSVersion1_1  TLSVersion = "1.1"
	TLSVersion1_2  TLSVersion = "1.2"
	TLSVersion1_3  TLSVersion = "1.3"
)

type SSLParameters struct {
	// Minimum TLS version.
	// +optional
	TLSMinVersion *TLSVersion `json:"tlsMinVersion,omitempty"`

	// Maximum TLS version.
	// +optional
	TLSMaxVersion *TLSVersion `json:"tlsMaxVersion,omitempty"`

	// +optional
	CipherSuites []string `json:"cipherSuites,omitempty"`

	// +optional
	EcdhCurves []string `json:"ecdhCurves,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="has(self.tlsCertificate) || has(self.tlsKey) || has(self.rootCA)",message="At least one of tlsCertificate, tlsKey, or rootCA must be set in SSLFiles"
type SSLFiles struct {
	// +optional
	TLSCertificate string `json:"tlsCertificate,omitempty"`

	// +optional
	TLSKey string `json:"tlsKey,omitempty"`

	// +optional
	RootCA string `json:"rootCA,omitempty"`
}
