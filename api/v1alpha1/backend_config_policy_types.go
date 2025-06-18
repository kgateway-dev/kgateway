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

	// TLS contains the options necessary to configure a backend to use TLS origination.
	// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/tls.proto#envoy-v3-api-msg-extensions-transport-sockets-tls-v3-sslconfig) for more details.
	// +optional
	TLS *TLS `json:"tls,omitempty"`

	// LoadBalancer contains the options necessary to configure the load balancer.
	// +optional
	LoadBalancer *LoadBalancer `json:"loadBalancer,omitempty"`
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

	// Maximum requests for a single upstream connection.
	// If set to 0 or unspecified, defaults to unlimited.
	// +optional
	MaxRequestsPerConnection *int `json:"maxRequestsPerConnection,omitempty"`
}

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

// +kubebuilder:validation:XValidation:rule="has(self.secretRef) != has(self.tlsFiles)",message="Exactly one of secretRef or tlsFiles must be set in TLS"
type TLS struct {
	// Reference to the TLS secret containing the certificate, key, and optionally the root CA.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// File paths to certificates local to the proxy.
	// +optional
	TLSFiles *TLSFiles `json:"tlsFiles,omitempty"`

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
	Parameters *Parameters `json:"parameters,omitempty"`

	// Set Application Level Protocol Negotiation
	// If empty, defaults to ["h2", "http/1.1"].
	// +optional
	AlpnProtocols []string `json:"alpnProtocols,omitempty"`

	// Allow Tls renegotiation, the default value is false.
	// TLS renegotiation is considered insecure and shouldn't be used unless absolutely necessary.
	// +optional
	AllowRenegotiation *bool `json:"allowRenegotiation,omitempty"`

	// If the TLS config has the ca.crt (root CA) provided, kgateway uses it to perform mTLS by default.
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

type Parameters struct {
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

// +kubebuilder:validation:XValidation:rule="has(self.tlsCertificate) || has(self.tlsKey) || has(self.rootCA)",message="At least one of tlsCertificate, tlsKey, or rootCA must be set in TLSFiles"
type TLSFiles struct {
	// +optional
	TLSCertificate string `json:"tlsCertificate,omitempty"`

	// +optional
	TLSKey string `json:"tlsKey,omitempty"`

	// +optional
	RootCA string `json:"rootCA,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="[has(self.leastRequest), has(self.roundRobin), has(self.ringHash), has(self.maglev), has(self.random)].filter(x, x).size() <= 1",message="only one of leastRequest, roundRobin, ringHash, maglev, or random can be set"
type LoadBalancer struct {
	// HealthyPanicThreshold configures envoy's panic threshold percentage between 0-100. Once the number of non-healthy hosts
	// reaches this percentage, envoy disregards health information.
	// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/panic_threshold.html).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	HealthyPanicThreshold *uint32 `json:"healthyPanicThreshold,omitempty"`

	// This allows batch updates of endpoints health/weight/metadata that happen during a time window.
	// this help lower cpu usage when endpoint change rate is high. defaults to 1 second.
	// Set to 0 to disable and have changes applied immediately.
	// +optional
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('0s')",message="updateMergeWindow must be a valid duration string"
	UpdateMergeWindow *metav1.Duration `json:"updateMergeWindow,omitempty"`

	// LeastRequest configures the least request load balancer type.
	// +optional
	LeastRequest *LoadBalancerLeastRequestConfig `json:"leastRequest,omitempty"`

	// RoundRobin configures the round robin load balancer type.
	// +optional
	RoundRobin *LoadBalancerRoundRobinConfig `json:"roundRobin,omitempty"`

	// RingHash configures the ring hash load balancer type.
	// +optional
	RingHash *LoadBalancerRingHashConfig `json:"ringHash,omitempty"`

	// Maglev configures the maglev load balancer type.
	// +optional
	Maglev *LoadBalancerMaglevConfig `json:"maglev,omitempty"`

	// Random configures the random load balancer type.
	// +optional
	Random *LoadBalancerRandomConfig `json:"random,omitempty"`

	// LocalityType specifies the locality config type to use.
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/load_balancing_policies/common/v3/common.proto#envoy-v3-api-msg-extensions-load-balancing-policies-common-v3-localitylbconfig
	// +optional
	// +kubebuilder:validation:Enum=WeightedLb
	LocalityType *LocalityType `json:"localityType,omitempty"`

	// UseHostnameForHashing specifies whether to use the hostname instead of the resolved IP address for hashing.
	// Defaults to false.
	// +optional
	// +default=false
	UseHostnameForHashing bool `json:"useHostnameForHashing,omitempty"`

	// If set to true, the load balancer will drain connections when the host set changes.
	//
	// Ring Hash or Maglev can be used to ensure that clients with the same key
	// are routed to the same upstream host.
	// Distruptions can cause new connections with the same key as existing connections
	// to be routed to different hosts.
	// Enabling this feature will cause the load balancer to drain existing connections
	// when the host set changes, ensuring that new connections with the same key are
	// consistently routed to the same host.
	// Connections are not immediately closed, but are allowed to drain
	// before being closed.
	// +optional
	CloseConnectionsOnHostSetChange *bool `json:"closeConnectionsOnHostSetChange,omitempty"`
}

// LoadBalancerLeastRequestConfig configures the least request load balancer type.
type LoadBalancerLeastRequestConfig struct {
	// How many choices to take into account.
	// Defaults to 2.
	// +optional
	// +default=2
	ChoiceCount uint32 `json:"choiceCount,omitempty"`

	// SlowStart configures the slow start configuration for the load balancer.
	// +optional
	SlowStart *SlowStart `json:"slowStart,omitempty"`
}

// LoadBalancerRoundRobinConfig configures the round robin load balancer type.
type LoadBalancerRoundRobinConfig struct {
	// SlowStart configures the slow start configuration for the load balancer.
	// +optional
	SlowStart *SlowStart `json:"slowStart,omitempty"`
}

// LoadBalancerRingHashConfig configures the ring hash load balancer type.
type LoadBalancerRingHashConfig struct {
	// MinimumRingSize is the minimum size of the ring.
	// +optional
	MinimumRingSize *uint64 `json:"minimumRingSize,omitempty"`

	// MaximumRingSize is the maximum size of the ring.
	// +optional
	MaximumRingSize *uint64 `json:"maximumRingSize,omitempty"`
}

type LoadBalancerMaglevConfig struct{}
type LoadBalancerRandomConfig struct{}

type SlowStart struct {
	// Represents the size of slow start window.
	// If set, the newly created host remains in slow start mode starting from its creation time
	// for the duration of slow start window.
	// +optional
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('0s')",message="window must be a valid duration string"
	Window *metav1.Duration `json:"window,omitempty"`

	// This parameter controls the speed of traffic increase over the slow start window. Defaults to 1.0,
	// so that endpoint would get linearly increasing amount of traffic.
	// When increasing the value for this parameter, the speed of traffic ramp-up increases non-linearly.
	// The value of aggression parameter should be greater than 0.0.
	// By tuning the parameter, is possible to achieve polynomial or exponential shape of ramp-up curve.
	//
	// During slow start window, effective weight of an endpoint would be scaled with time factor and aggression:
	// `new_weight = weight * max(min_weight_percent, time_factor ^ (1 / aggression))`,
	// where `time_factor=(time_since_start_seconds / slow_start_time_seconds)`.
	//
	// As time progresses, more and more traffic would be sent to endpoint, which is in slow start window.
	// Once host exits slow start, time_factor and aggression no longer affect its weight.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == \"\" || (self.matches('^-?(?:[0-9]+(?:\\\\.[0-9]*)?|\\\\.[0-9]+)$') && double(self) > 0.0)",message="Aggression, if specified, must be a string representing a number greater than 0.0"
	Aggression string `json:"aggression,omitempty"`

	// Minimum weight percentage of an endpoint during slow start.
	// +optional
	MinWeightPercent *uint32 `json:"minWeightPercent,omitempty"`
}

type LocalityType string

const (
	// https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/locality_weight#locality-weighted-load-balancing
	// Locality weighted load balancing enables weighting assignments across different zones and geographical locations by using explicit weights.
	// This field is required to enable locality weighted load balancing.
	LocalityConfigTypeWeightedLb LocalityType = "WeightedLb"
)
