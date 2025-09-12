package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backendconfigpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=backendconfigpolicies/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway,gateway.networking.k8s.io/policy=Direct}
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

// BackendConfigPolicySpec defines the desired state of BackendConfigPolicy.
//
// +kubebuilder:validation:AtMostOneOf=http1ProtocolOptions;http2ProtocolOptions
type BackendConfigPolicySpec struct {
	// TargetRefs specifies the target references to attach the policy to.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:rule="self.all(r, (r.group == '' && r.kind == 'Service') || (r.group == 'gateway.kgateway.dev' && r.kind == 'Backend'))",message="TargetRefs must reference either a Kubernetes Service or a Backend API"
	TargetRefs []LocalPolicyTargetReference `json:"targetRefs,omitempty"`

	// TargetSelectors specifies the target selectors to select resources to attach the policy to.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.all(r, (r.group == '' && r.kind == 'Service') || (r.group == 'gateway.kgateway.dev' && r.kind == 'Backend'))",message="TargetSelectors must reference either a Kubernetes Service or a Backend API"
	TargetSelectors []LocalPolicyTargetSelector `json:"targetSelectors,omitempty"`

	// The timeout for new network connections to hosts in the cluster.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
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

	// Http2ProtocolOptions contains the options necessary to configure HTTP/2 backends.
	// Note: Http2ProtocolOptions can only be applied to HTTP/2 backends.
	// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/tls.proto#envoy-v3-api-msg-extensions-transport-sockets-tls-v3-sslconfig) for more details.
	// +optional
	Http2ProtocolOptions *Http2ProtocolOptions `json:"http2ProtocolOptions,omitempty"`

	// TLS contains the options necessary to configure a backend to use TLS origination.
	// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/tls.proto#envoy-v3-api-msg-extensions-transport-sockets-tls-v3-sslconfig) for more details.
	// +optional
	TLS *TLS `json:"tls,omitempty"`

	// LoadBalancer contains the options necessary to configure the load balancer.
	// +optional
	LoadBalancer *LoadBalancer `json:"loadBalancer,omitempty"`

	// HealthCheck contains the options necessary to configure the health check.
	// +optional
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`

	// OutlierDetection contains the options necessary to configure passive health checking.
	// +optional
	OutlierDetection *OutlierDetection `json:"outlierDetection,omitempty"`
}

// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-msg-config-core-v3-http1protocoloptions) for more details.
type Http1ProtocolOptions struct {
	// Enables trailers for HTTP/1. By default the HTTP/1 codec drops proxied trailers.
	// Note: Trailers must also be enabled at the gateway level in order for this option to take effect
	// +optional
	EnableTrailers *bool `json:"enableTrailers,omitempty"`

	// PreserveHttp1HeaderCase determines whether to preserve the case of HTTP1 response headers.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/header_casing
	// +optional
	PreserveHttp1HeaderCase *bool `json:"preserveHttp1HeaderCase,omitempty"`

	// Allows invalid HTTP messaging. When this option is false, then Envoy will terminate
	// HTTP/1.1 connections upon receiving an invalid HTTP message. However,
	// when this option is true, then Envoy will leave the HTTP/1.1 connection
	// open where possible.
	// +optional
	OverrideStreamErrorOnInvalidHttpMessage *bool `json:"overrideStreamErrorOnInvalidHttpMessage,omitempty"`
}

// CommonHttpProtocolOptions are options that are applicable to both HTTP1 and HTTP2 requests.
// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-msg-config-core-v3-httpprotocoloptions) for more details.
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
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	IdleTimeout *metav1.Duration `json:"idleTimeout,omitempty"`

	// Specifies the maximum number of headers that the connection will accept.
	// If not specified, the default of 100 is used. Requests that exceed this limit will receive
	// a 431 response for HTTP/1.x and cause a stream reset for HTTP/2.
	// +optional
	MaxHeadersCount *int `json:"maxHeadersCount,omitempty"`

	// Total duration to keep alive an HTTP request/response stream. If the time limit is reached the stream will be
	// reset independent of any other timeouts. If not specified, this value is not set.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	MaxStreamDuration *metav1.Duration `json:"maxStreamDuration,omitempty"`

	// Maximum requests for a single upstream connection.
	// If set to 0 or unspecified, defaults to unlimited.
	// +optional
	MaxRequestsPerConnection *int `json:"maxRequestsPerConnection,omitempty"`
}
type Http2ProtocolOptions struct {
	// InitialStreamWindowSize is the initial window size for the stream.
	// Valid values range from 65535 (2^16 - 1, HTTP/2 default) to 2147483647 (2^31 - 1, HTTP/2 maximum).
	// Defaults to 268435456 (256 * 1024 * 1024).
	// Values can be specified with units like "64Ki".
	// +optional
	// +kubebuilder:validation:XValidation:message="InitialStreamWindowSize must be between 65535 and 2147483647 bytes (inclusive)",rule="(type(self) == int && int(self) >= 65535 && int(self) <= 2147483647) || (type(self) == string && quantity(self).isGreaterThan(quantity('65534')) && quantity(self).isLessThan(quantity('2147483648')))"
	InitialStreamWindowSize *resource.Quantity `json:"initialStreamWindowSize,omitempty"`

	// InitialConnectionWindowSize is similar to InitialStreamWindowSize, but for the connection level.
	// Same range and default value as InitialStreamWindowSize.
	// Values can be specified with units like "64Ki".
	// +optional
	// +kubebuilder:validation:XValidation:message="InitialConnectionWindowSize must be between 65535 and 2147483647 bytes (inclusive)",rule="(type(self) == int && int(self) >= 65535 && int(self) <= 2147483647) || (type(self) == string && quantity(self).isGreaterThan(quantity('65534')) && quantity(self).isLessThan(quantity('2147483648')))"
	InitialConnectionWindowSize *resource.Quantity `json:"initialConnectionWindowSize,omitempty"`

	// The maximum number of concurrent streams that the connection can have.
	// +optional
	MaxConcurrentStreams *int `json:"maxConcurrentStreams,omitempty"`

	// Allows invalid HTTP messaging and headers. When disabled (default), then
	// the whole HTTP/2 connection is terminated upon receiving invalid HEADERS frame.
	// When enabled, only the offending stream is terminated.
	// +optional
	OverrideStreamErrorOnInvalidHttpMessage *bool `json:"overrideStreamErrorOnInvalidHttpMessage,omitempty"`
}

// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/address.proto#envoy-v3-api-msg-config-core-v3-tcpkeepalive) for more details.
type TCPKeepalive struct {
	// Maximum number of keep-alive probes to send before dropping the connection.
	// +optional
	KeepAliveProbes *int `json:"keepAliveProbes,omitempty"`

	// The number of seconds a connection needs to be idle before keep-alive probes start being sent.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="keepAliveTime must be at least 1 second"
	KeepAliveTime *metav1.Duration `json:"keepAliveTime,omitempty"`

	// The number of seconds between keep-alive probes.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1s')",message="keepAliveInterval must be at least 1 second"
	KeepAliveInterval *metav1.Duration `json:"keepAliveInterval,omitempty"`
}

// +kubebuilder:validation:ExactlyOneOf=secretRef;tlsFiles;insecureSkipVerify;wellKnownCACertificates
type TLS struct {
	// Reference to the TLS secret containing the certificate, key, and optionally the root CA.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// File paths to certificates local to the proxy.
	// +optional
	TLSFiles *TLSFiles `json:"tlsFiles,omitempty"`

	// WellKnownCACertificates specifies whether to use a well-known set of CA
	// certificates for validating the backend's certificate chain. Currently,
	// only the system certificate pool is supported via SDS.
	// +optional
	WellKnownCACertificates *gwv1alpha3.WellKnownCACertificatesType `json:"wellKnownCACertificates,omitempty"`

	// InsecureSkipVerify originates TLS but skips verification of the backend's certificate.
	// WARNING: This is an insecure option that should only be used if the risks are understood.
	// +optional
	InsecureSkipVerify *bool `json:"insecureSkipVerify,omitempty"`

	// The SNI domains that should be considered for TLS connection
	// +optional
	// +kubebuilder:validation:MinLength=1
	Sni *string `json:"sni,omitempty"`

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

	// If the TLS config has the tls cert and key provided, kgateway uses it to perform mTLS by default.
	// Set simpleTLS to true to disable mTLS in favor of server-only TLS (one-way TLS), even if kgateway has the client cert.
	// If unset, defaults to false.
	// +optional
	SimpleTLS *bool `json:"simpleTLS,omitempty"`
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
	// +kubebuilder:validation:MinLength=1
	TLSCertificate *string `json:"tlsCertificate,omitempty"`

	// +optional
	// +kubebuilder:validation:MinLength=1
	TLSKey *string `json:"tlsKey,omitempty"`

	// +optional
	// +kubebuilder:validation:MinLength=1
	RootCA *string `json:"rootCA,omitempty"`
}

// +kubebuilder:validation:ExactlyOneOf=leastRequest;roundRobin;ringHash;maglev;random
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
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
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

	// UseHostnameForHashing specifies whether to use the hostname instead of the resolved IP address for hashing.
	// Defaults to false.
	// +optional
	UseHostnameForHashing *bool `json:"useHostnameForHashing,omitempty"`

	// HashPolicies specifies the hash policies for hashing load balancers (RingHash, Maglev).
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	HashPolicies []*HashPolicy `json:"hashPolicies,omitempty"`
}

type LoadBalancerMaglevConfig struct {
	// UseHostnameForHashing specifies whether to use the hostname instead of the resolved IP address for hashing.
	// Defaults to false.
	// +optional
	UseHostnameForHashing *bool `json:"useHostnameForHashing,omitempty"`

	// HashPolicies specifies the hash policies for hashing load balancers (RingHash, Maglev).
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	HashPolicies []*HashPolicy `json:"hashPolicies,omitempty"`
}

type (
	LoadBalancerRandomConfig struct{}
	SlowStart                struct {
		// Represents the size of slow start window.
		// If set, the newly created host remains in slow start mode starting from its creation time
		// for the duration of slow start window.
		// +optional
		// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
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
		// +kubebuilder:validation:XValidation:rule="(self.matches('^-?(?:[0-9]+(?:\\\\.[0-9]*)?|\\\\.[0-9]+)$') && double(self) > 0.0)",message="Aggression, if specified, must be a string representing a number greater than 0.0"
		Aggression *string `json:"aggression,omitempty"`

		// Minimum weight percentage of an endpoint during slow start.
		// +optional
		MinWeightPercent *uint32 `json:"minWeightPercent,omitempty"`
	}
)

type LocalityType string

const (
	// https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/locality_weight#locality-weighted-load-balancing
	// Locality weighted load balancing enables weighting assignments across different zones and geographical locations by using explicit weights.
	// This field is required to enable locality weighted load balancing.
	LocalityConfigTypeWeightedLb LocalityType = "WeightedLb"
)

// HealthCheck contains the options to configure the health check.
// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/health_check.proto) for more details.
// +optional
// +kubebuilder:validation:XValidation:rule="has(self.http) != has(self.grpc)",message="exactly one of http or grpc must be set"
type HealthCheck struct {
	// Timeout is time to wait for a health check response. If the timeout is reached the
	// health check attempt will be considered a failure.
	// +required
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	Timeout *metav1.Duration `json:"timeout"`

	// Interval is the time between health checks.
	// +required
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	Interval *metav1.Duration `json:"interval"`

	// UnhealthyThreshold is the number of consecutive failed health checks that will be considered
	// unhealthy.
	// Note that for HTTP health checks, if a host responds with a code not in ExpectedStatuses or RetriableStatuses,
	// this threshold is ignored and the host is considered immediately unhealthy.
	// +required
	UnhealthyThreshold *uint32 `json:"unhealthyThreshold"`

	// HealthyThreshold is the number of healthy health checks required before a host is marked
	// healthy. Note that during startup, only a single successful health check is
	// required to mark a host healthy.
	// +required
	HealthyThreshold *uint32 `json:"healthyThreshold"`

	// Http contains the options to configure the HTTP health check.
	// +optional
	Http *HealthCheckHttp `json:"http,omitempty"`

	// Grpc contains the options to configure the gRPC health check.
	// +optional
	Grpc *HealthCheckGrpc `json:"grpc,omitempty"`
}
type HealthCheckHttp struct {
	// Host is the value of the host header in the HTTP health check request. If
	// unset, the name of the cluster this health check is associated
	// with will be used.
	// +optional
	Host *string `json:"host,omitempty"`

	// Path is the HTTP path requested.
	// +required
	Path string `json:"path"`

	// Method is the HTTP method to use.
	// If unset, GET is used.
	// +optional
	// +kubebuilder:validation:Enum=GET;HEAD;POST;PUT;DELETE;OPTIONS;TRACE;PATCH
	Method *string `json:"method,omitempty"`
}

type HealthCheckGrpc struct {
	// ServiceName is the optional name of the service to check.
	// +optional
	ServiceName *string `json:"serviceName,omitempty"`

	// Authority is the authority header used to make the gRPC health check request.
	// If unset, the name of the cluster this health check is associated
	// with will be used.
	// +optional
	Authority *string `json:"authority,omitempty"`
}

// OutlierDetection contains the options to configure passive health checks.
// See [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/outlier#outlier-detection) for more details.
// +optional
type OutlierDetection struct {
	// The number of consecutive server-side error responses (for HTTP traffic,
	// 5xx responses; for TCP traffic, connection failures; etc.) before an
	// ejection occurs. Defaults to 5. If this is zero, consecutive 5xx passive
	// health checks will be disabled. In the future, other types of passive
	// health checking might be added, but none will be enabled by default.
	// +optional
	// +kubebuilder:default=5
	Consecutive5xx *uint32 `json:"consecutive5xx,omitempty"`

	// The time interval between ejection analysis sweeps. This can result in
	// both new ejections as well as hosts being returned to service. Defaults
	// to 10s.
	// +optional
	// +kubebuilder:default="10s"
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	Interval *metav1.Duration `json:"interval,omitempty"`

	// The base time that a host is ejected for. The real time is equal to the
	// base time multiplied by the number of times the host has been ejected.
	// Defaults to 30s.
	// +optional
	// +kubebuilder:default="30s"
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	BaseEjectionTime *metav1.Duration `json:"baseEjectionTime,omitempty"`

	// The maximum % of an upstream cluster that can be ejected due to outlier
	// detection. Defaults to 10%.
	// +optional
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	MaxEjectionPercent *uint32 `json:"maxEjectionPercent,omitempty"`
}

// +kubebuilder:validation:ExactlyOneOf=header;cookie;sourceIP
type HashPolicy struct {
	// Header specifies a header's value as a component of the hash key.
	// +optional
	Header *Header `json:"header,omitempty"`

	// Cookie specifies a given cookie as a component of the hash key.
	// +optional
	Cookie *Cookie `json:"cookie,omitempty"`

	// SourceIP specifies whether to use the request's source IP address as a component of the hash key.
	// +optional
	SourceIP *SourceIP `json:"sourceIP,omitempty"`

	// Terminal, if set, and a hash key is available after evaluating this policy, will cause Envoy to skip the subsequent policies and
	// use the key as it is.
	// This is useful for defining "fallback" policies and limiting the time Envoy spends generating hash keys.
	// +optional
	Terminal *bool `json:"terminal,omitempty"`
}

type Header struct {
	// Name is the name of the header to use as a component of the hash key.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

type Cookie struct {
	// Name of the cookie.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Path is the name of the path for the cookie.
	// +optional
	Path *string `json:"path,omitempty"`

	// TTL specifies the time to live of the cookie.
	// If specified, a cookie with the TTL will be generated if the cookie is not present.
	// If the TTL is present and zero, the generated cookie will be a session cookie.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	TTL *metav1.Duration `json:"ttl,omitempty"`

	// Secure specifies whether the cookie is secure.
	// If true, the cookie will only be sent over HTTPS.
	// +optional
	Secure *bool `json:"secure,omitempty"`

	// HttpOnly specifies whether the cookie is HTTP only, i.e. not accessible to JavaScript.
	// +optional
	HttpOnly *bool `json:"httpOnly,omitempty"`

	// SameSite controls cross-site sending of cookies.
	// Supported values are Strict, Lax, and None.
	// +optional
	// +kubebuilder:validation:Enum=Strict;Lax;None
	SameSite *string `json:"sameSite,omitempty"`
}

type SourceIP struct{}
