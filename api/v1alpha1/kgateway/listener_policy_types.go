package kgateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=listenerpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=listenerpolicies/status,verbs=get;update;patch

// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Accepted')].status",description="Listener policy acceptance status"
// +kubebuilder:printcolumn:name="Attached",type=string,JSONPath=".status.ancestors[*].conditions[?(@.type=='Attached')].status",description="Listener policy attachment status"

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="gateway.networking.k8s.io/policy=Direct"
// ListenerPolicy is used for configuring Envoy listener-level settings that apply to all protocol types (HTTP, HTTPS, TCP, TLS).
// These policies can only target `Gateway` objects.
type ListenerPolicy struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +required
	Spec ListenerPolicySpec `json:"spec"`
	// +optional
	Status gwv1.PolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ListenerPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ListenerPolicy `json:"items"`
}

// ListenerPolicySpec defines the desired state of a listener policy.
type ListenerPolicySpec struct {
	// TargetRefs specifies the target resources by reference to attach the policy to.
	// Only supports `Gateway` resources
	// +optional
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:rule="self.all(r, r.kind == 'Gateway' && (!has(r.group) || r.group == 'gateway.networking.k8s.io'))",message="targetRefs may only reference Gateway resource"
	TargetRefs []shared.LocalPolicyTargetReference `json:"targetRefs,omitempty"`

	// TargetSelectors specifies the target selectors to select `Gateway` resources to attach the policy to.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.all(r, r.kind == 'Gateway' && (!has(r.group) || r.group == 'gateway.networking.k8s.io'))",message="targetSelectors may only reference Gateway resource"
	TargetSelectors []shared.LocalPolicyTargetSelector `json:"targetSelectors,omitempty"`

	// Default specifies default listener configuration for all Listeners, unless a per-port
	// configuration is defined.
	// +optional
	Default *ListenerConfig `json:"default,omitempty"`

	// Per port configuration allows overriding the listener config per port. Once set, this
	// configuration completely replaces the default configuration for all listeners handling traffic
	// that match this port. Unspecified fields in per-port configuration will not inherit values from default.
	//
	// +optional
	// +listType=map
	// +listMapKey=port
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:validation:XValidation:message="Port for listener configuration must be unique within the Gateway",rule="self.all(t1, self.exists_one(t2, t1.port == t2.port))"
	PerPort []ListenerPortConfig `json:"perPort,omitempty"`
}

type ListenerPortConfig struct {
	// The Port indicates the Port Number to which the Listener configuration will be
	// applied. This configuration will be applied to all Listeners handling
	// traffic that match this port.
	//
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Listener stores the configuration that will be applied to all Listeners handling
	// matching the given port.
	//
	// +required
	Listener ListenerConfig `json:"listener"`
}

type ListenerConfig struct {

	// ProxyProtocol configures the PROXY protocol listener filter.
	// When set, Envoy will expect connections to include the PROXY protocol header.
	// This is commonly used when kgateway is behind a load balancer that preserves client IP information.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/listener/proxy_protocol/v3/proxy_protocol.proto
	// +optional
	ProxyProtocol *ProxyProtocolConfig `json:"proxyProtocol,omitempty"`

	// PerConnectionBufferLimitBytes sets the per-connection buffer limit for all listeners on the gateway.
	// This controls the maximum size of read and write buffers for new connections.
	// When using Envoy as an edge proxy, configuring the listener buffer limit is important to guard against
	// potential attacks or misconfigured downstreams that could hog the proxy's resources.
	// If unspecified, an implementation-defined default is applied (1MiB).
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/listener/v3/listener.proto#envoy-v3-api-field-config-listener-v3-listener-per-connection-buffer-limit-bytes
	// +optional
	// +kubebuilder:validation:Minimum=0
	PerConnectionBufferLimitBytes *int32 `json:"perConnectionBufferLimitBytes,omitempty"`

	// HTTPListenerPolicy is intended to be used for configuring the Envoy `HttpConnectionManager` and any other config or policy
	// that should map 1-to-1 with a given HTTP listener, such as the Envoy health check HTTP filter.
	// +optional
	HTTPSettings *HTTPSettings `json:"httpSettings,omitempty"`
}

// ProxyProtocolConfig configures the PROXY protocol listener filter.
// The presence of this configuration enables PROXY protocol support.
type ProxyProtocolConfig struct {
}
type HTTPSettings struct {
	// AccessLoggingConfig contains various settings for Envoy's access logging service.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto
	// +kubebuilder:validation:MaxItems=16
	// +optional
	AccessLog []AccessLog `json:"accessLog,omitempty"`

	// Tracing contains various settings for Envoy's OpenTelemetry tracer.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/trace/v3/opentelemetry.proto.html
	// +optional
	Tracing *Tracing `json:"tracing,omitempty"`

	// UpgradeConfig contains configuration for HTTP upgrades like WebSocket.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/v1.34.1/intro/arch_overview/http/upgrades.html
	// +optional
	UpgradeConfig *UpgradeConfig `json:"upgradeConfig,omitempty"`

	// UseRemoteAddress determines whether to use the remote address for the original client.
	// Note: If this field is omitted, it will fallback to the default value of 'true', which we set for all Envoy HCMs.
	// Thus, setting this explicitly to true is unnecessary (but will not cause any harm).
	// When true, Envoy will use the remote address of the connection as the client address.
	// When false, Envoy will use the X-Forwarded-For header to determine the client address.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-use-remote-address
	// +optional
	UseRemoteAddress *bool `json:"useRemoteAddress,omitempty"`

	// PreserveExternalRequestId determines whether the connection manager will keep the x-request-id header if passed for
	// a request that is edge (Edge request is the request from external clients to front Envoy) and not reset it, which is the current Envoy behaviour. This defaults to false.
	// See here for more information https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-preserve-external-request-id
	// +optional
	PreserveExternalRequestId *bool `json:"preserveExternalRequestId,omitempty"`

	// GenerateRequestId:  Whether the connection manager will generate the x-request-id header if it does not exist.
	// This defaults to true. Generating a random UUID4 is expensive so in high throughput scenarios where this feature is not desired it can be disabled.
	// See here for more information https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-generate-request-id
	// +optional
	GenerateRequestId *bool `json:"generateRequestId,omitempty"`

	// XffNumTrustedHops is the number of additional ingress proxy hops from the right side of the X-Forwarded-For HTTP header to trust when determining the origin client's IP address.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-xff-num-trusted-hops
	// +kubebuilder:validation:Minimum=0
	// +optional
	XffNumTrustedHops *int32 `json:"xffNumTrustedHops,omitempty"`

	// ServerHeaderTransformation determines how the server header is transformed.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-server-header-transformation
	// +kubebuilder:validation:Enum=Overwrite;AppendIfAbsent;PassThrough
	// +optional
	ServerHeaderTransformation *ServerHeaderTransformation `json:"serverHeaderTransformation,omitempty"`

	// StreamIdleTimeout is the idle timeout for HTTP streams.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-stream-idle-timeout
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	StreamIdleTimeout *metav1.Duration `json:"streamIdleTimeout,omitempty"`

	// IdleTimeout is the idle timeout for connnections.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-v3-api-msg-config-core-v3-httpprotocoloptions
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	IdleTimeout *metav1.Duration `json:"idleTimeout,omitempty"`

	// HealthCheck configures [Envoy health checks](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/health_check/v3/health_check.proto)
	// +optional
	HealthCheck *EnvoyHealthCheck `json:"healthCheck,omitempty"`

	// PreserveHttp1HeaderCase determines whether to preserve the case of HTTP1 request headers.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/header_casing
	// +optional
	PreserveHttp1HeaderCase *bool `json:"preserveHttp1HeaderCase,omitempty"`

	// AcceptHTTP10 determines whether to accept incoming HTTP/1.0 and HTTP 0.9 requests.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#config-core-v3-http1protocoloptions
	// +optional
	AcceptHttp10 *bool `json:"acceptHttp10,omitempty"`

	// DefaultHostForHttp10 specifies a default host for HTTP/1.0 requests. This is highly suggested if acceptHttp10 is true and a no-op if acceptHttp10 is false.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#config-core-v3-http1protocoloptions
	// +optional
	// +kubebuilder:validation:MinLength=1
	DefaultHostForHttp10 *string `json:"defaultHostForHttp10,omitempty"`

	// EarlyRequestHeaderModifier defines header modifications to be applied early in the request processing,
	// before route selection.
	// For example, if you use ExternalAuthz to add a header, you may want to remove it here, to make
	// sure it did not come from the client.
	// +optional
	EarlyRequestHeaderModifier *gwv1.HTTPHeaderFilter `json:"earlyRequestHeaderModifier,omitempty"`

	// MaxRequestHeadersKb sets the maximum size of request headers that Envoy will accept.
	// If unset, the Envoy default is 60 KiB.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-max-request-headers-kb
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=8192
	MaxRequestHeadersKb *int32 `json:"maxRequestHeadersKb,omitempty"`

	// UuidRequestIdConfig configures the behavior of the UUID request ID extension.
	// This extension sets the x-request-id header to a UUID value.
	// +optional
	UuidRequestIdConfig *UuidRequestIdConfig `json:"uuidRequestIdConfig,omitempty"`
}

// AccessLog represents the top-level access log configuration.
type AccessLog struct {
	// Output access logs to local file
	// +optional
	FileSink *FileSink `json:"fileSink,omitempty"`

	// Send access logs to gRPC service
	// +optional
	GrpcService *AccessLogGrpcService `json:"grpcService,omitempty"`

	// Send access logs to an OTel collector
	// +optional
	OpenTelemetry *OpenTelemetryAccessLogService `json:"openTelemetry,omitempty"`

	// Filter access logs configuration
	// +optional
	Filter *AccessLogFilter `json:"filter,omitempty"`
}

// FileSink represents the file sink configuration for access logs.
// +kubebuilder:validation:ExactlyOneOf=stringFormat;jsonFormat
type FileSink struct {
	// the file path to which the file access logging service will sink
	// +required
	Path string `json:"path"`
	// the format string by which envoy will format the log lines
	// https://www.envoyproxy.io/docs/envoy/v1.33.0/configuration/observability/access_log/usage#format-strings
	// +optional
	StringFormat *string `json:"stringFormat,omitempty"`
	// the format object by which to envoy will emit the logs in a structured way.
	// https://www.envoyproxy.io/docs/envoy/v1.33.0/configuration/observability/access_log/usage#format-dictionaries
	// +optional
	JsonFormat *runtime.RawExtension `json:"jsonFormat,omitempty"`
}

// AccessLogGrpcService represents the gRPC service configuration for access logs.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/access_loggers/grpc/v3/als.proto#envoy-v3-api-msg-extensions-access-loggers-grpc-v3-httpgrpcaccesslogconfig
type AccessLogGrpcService struct {
	CommonAccessLogGrpcService `json:",inline"`

	// Additional request headers to log in the access log
	// +optional
	AdditionalRequestHeadersToLog []string `json:"additionalRequestHeadersToLog,omitempty"`

	// Additional response headers to log in the access log
	// +optional
	AdditionalResponseHeadersToLog []string `json:"additionalResponseHeadersToLog,omitempty"`

	// Additional response trailers to log in the access log
	// +optional
	AdditionalResponseTrailersToLog []string `json:"additionalResponseTrailersToLog,omitempty"`
}

// Common configuration for gRPC access logs.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/access_loggers/grpc/v3/als.proto#envoy-v3-api-msg-extensions-access-loggers-grpc-v3-commongrpcaccesslogconfig
type CommonAccessLogGrpcService struct {
	CommonGrpcService `json:",inline"`

	// name of log stream
	// +required
	LogName string `json:"logName"`
}

// Common gRPC service configuration created by setting `envoy_grpc“ as the gRPC client
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/grpc_service.proto#envoy-v3-api-msg-config-core-v3-grpcservice
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/grpc_service.proto#envoy-v3-api-msg-config-core-v3-grpcservice-envoygrpc
type CommonGrpcService struct {
	// The backend gRPC service. Can be any type of supported backend (Kubernetes Service, kgateway Backend, etc..)
	// +required
	BackendRef gwv1.BackendRef `json:"backendRef"`

	// The :authority header in the grpc request. If this field is not set, the authority header value will be cluster_name.
	// Note that this authority does not override the SNI. The SNI is provided by the transport socket of the cluster.
	// +optional
	Authority *string `json:"authority,omitempty"`

	// Maximum gRPC message size that is allowed to be received. If a message over this limit is received, the gRPC stream is terminated with the RESOURCE_EXHAUSTED error.
	// Defaults to 0, which means unlimited.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxReceiveMessageLength *int32 `json:"maxReceiveMessageLength,omitempty"`

	// This provides gRPC client level control over envoy generated headers. If false, the header will be sent but it can be overridden by per stream option. If true, the header will be removed and can not be overridden by per stream option. Default to false.
	// +optional
	SkipEnvoyHeaders *bool `json:"skipEnvoyHeaders,omitempty"`

	// The timeout for the gRPC request. This is the timeout for a specific request
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Additional metadata to include in streams initiated to the GrpcService.
	// This can be used for scenarios in which additional ad hoc authorization headers (e.g. x-foo-bar: baz-key) are to be injected
	// +optional
	InitialMetadata []HeaderValue `json:"initialMetadata,omitempty"`

	// Indicates the retry policy for re-establishing the gRPC stream.
	// If max interval is not provided, it will be set to ten times the provided base interval
	// +optional
	RetryPolicy *RetryPolicy `json:"retryPolicy,omitempty"`
}

// Header name/value pair.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/base.proto#envoy-v3-api-msg-config-core-v3-headervalue
type HeaderValue struct {
	// Header name.
	// +required
	Key string `json:"key"`

	// Header value.
	// +optional
	Value *string `json:"value,omitempty"`
}

// Specifies the retry policy of remote data source when fetching fails.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/base.proto#envoy-v3-api-msg-config-core-v3-retrypolicy
type RetryPolicy struct {
	// Specifies parameters that control retry backoff strategy.
	// the default base interval is 1000 milliseconds and the default maximum interval is 10 times the base interval.
	// +optional
	RetryBackOff *BackoffStrategy `json:"retryBackOff,omitempty"`

	// Specifies the allowed number of retries. Defaults to 1.
	// +optional
	// +kubebuilder:validation:Minimum=1
	NumRetries *int32 `json:"numRetries,omitempty"`
}

// Configuration defining a jittered exponential back off strategy.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/backoff.proto#envoy-v3-api-msg-config-core-v3-backoffstrategy
type BackoffStrategy struct {
	// The base interval to be used for the next back off computation. It should be greater than zero and less than or equal to max_interval.
	// +required
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	BaseInterval metav1.Duration `json:"baseInterval"`

	// Specifies the maximum interval between retries. This parameter is optional, but must be greater than or equal to the base_interval if set. The default is 10 times the base_interval.
	// +optional
	// +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
	MaxInterval *metav1.Duration `json:"maxInterval,omitempty"`
}

// OpenTelemetryAccessLogService represents the OTel configuration for access logs.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/access_loggers/open_telemetry/v3/logs_service.proto
type OpenTelemetryAccessLogService struct {
	// Send access logs to gRPC service
	// +required
	GrpcService CommonAccessLogGrpcService `json:"grpcService"`

	// OpenTelemetry LogResource fields, following Envoy access logging formatting.
	// +optional
	Body *string `json:"body,omitempty"`

	// If specified, Envoy will not generate built-in resource labels like log_name, zone_name, cluster_name, node_name.
	// +optional
	DisableBuiltinLabels *bool `json:"disableBuiltinLabels,omitempty"`

	// Additional attributes that describe the specific event occurrence.
	// +optional
	Attributes *KeyAnyValueList `json:"attributes,omitempty"`

	// Additional resource attributes that describe the resource.
	// If the `service.name` resource attribute is not specified, it adds it with the default value
	// of the envoy cluster name, ie: `<gateway-name>.<gateway-namespace>`
	// +optional
	ResourceAttributes *KeyAnyValueList `json:"resourceAttributes,omitempty"`
}

// A list of key-value pair that is used to store Span attributes, Link attributes, etc.
type KeyAnyValueList struct {
	// A collection of key/value pairs of key-value pairs.
	// +kubebuilder:validation:items:Type=object
	// +optional
	Values []KeyAnyValue `json:"values,omitempty"`
}

// KeyValue is a key-value pair that is used to store Span attributes, Link attributes, etc.
type KeyAnyValue struct {
	// Attribute keys must be unique
	// +required
	Key string `json:"key"`
	// Value may contain a primitive value such as a string or integer or it may contain an arbitrary nested object containing arrays, key-value lists and primitives.
	// +required
	Value AnyValue `json:"value"`
}

// AnyValue is used to represent any type of attribute value. AnyValue may contain a primitive value such as a string or integer or it may contain an arbitrary nested object containing arrays, key-value lists and primitives.
// This is limited to string and nested values as OTel only supports them
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type AnyValue struct {
	// +optional
	StringValue *string `json:"stringValue,omitempty"`
	// TODO: Add support for ArrayValue && KvListValue
	// +kubebuilder:validation:items:Type=object
	// +kubebuilder:validation:items:XPreserveUnknownFields
	// +optional
	ArrayValue []AnyValue `json:"arrayValue,omitempty"`
	// +kubebuilder:validation:Type=object
	// +kubebuilder:validation:XPreserveUnknownFields
	// +optional
	KvListValue *KeyAnyValueList `json:"kvListValue,omitempty"`
}

// AccessLogFilter represents the top-level filter structure.
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-accesslogfilter
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type AccessLogFilter struct {
	*FilterType `json:",inline"` // embedded to allow for validation
	// Performs a logical "and" operation on the result of each individual filter.
	// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-andfilter
	// +kubebuilder:validation:MinItems=2
	// +optional
	AndFilter []FilterType `json:"andFilter,omitempty"`
	// Performs a logical "or" operation on the result of each individual filter.
	// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-orfilter
	// +kubebuilder:validation:MinItems=2
	// +optional
	OrFilter []FilterType `json:"orFilter,omitempty"`
}

// FilterType represents the type of filter to apply (only one of these should be set).
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#envoy-v3-api-msg-config-accesslog-v3-accesslogfilter
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type FilterType struct {
	// +optional
	StatusCodeFilter *StatusCodeFilter `json:"statusCodeFilter,omitempty"`
	// +optional
	DurationFilter *DurationFilter `json:"durationFilter,omitempty"`
	// Filters for requests that are not health check requests.
	// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-nothealthcheckfilter
	// +optional
	NotHealthCheckFilter *bool `json:"notHealthCheckFilter,omitempty"`
	// Filters for requests that are traceable.
	// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-traceablefilter
	// +optional
	TraceableFilter *bool `json:"traceableFilter,omitempty"`
	// +optional
	HeaderFilter *HeaderFilter `json:"headerFilter,omitempty"`
	// +optional
	ResponseFlagFilter *ResponseFlagFilter `json:"responseFlagFilter,omitempty"`
	// +optional
	GrpcStatusFilter *GrpcStatusFilter `json:"grpcStatusFilter,omitempty"`
	// +optional
	CELFilter *CELFilter `json:"celFilter,omitempty"`
}

// ComparisonFilter represents a filter based on a comparison.
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-comparisonfilter
type ComparisonFilter struct {
	// +required
	Op Op `json:"op"`

	// Value to compare against.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	// +required
	Value int32 `json:"value"`
}

// Op represents comparison operators.
// +kubebuilder:validation:Enum=EQ;GE;LE
type Op string

const (
	EQ Op = "EQ" // Equal
	GE Op = "GE" // Greater or equal
	LE Op = "LE" // Less or equal
)

// StatusCodeFilter filters based on HTTP status code.
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#envoy-v3-api-msg-config-accesslog-v3-statuscodefilter
type StatusCodeFilter ComparisonFilter

// DurationFilter filters based on request duration.
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-durationfilter
type DurationFilter ComparisonFilter

// DenominatorType defines the fraction percentages support several fixed denominator values.
// +kubebuilder:validation:enum=HUNDRED,TEN_THOUSAND,MILLION
type DenominatorType string

const (
	// 100.
	//
	// **Example**: 1/100 = 1%.
	HUNDRED DenominatorType = "HUNDRED"
	// 10,000.
	//
	// **Example**: 1/10000 = 0.01%.
	TEN_THOUSAND DenominatorType = "TEN_THOUSAND"
	// 1,000,000.
	//
	// **Example**: 1/1000000 = 0.0001%.
	MILLION DenominatorType = "MILLION"
)

// HeaderFilter filters requests based on headers.
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-headerfilter
type HeaderFilter struct {
	// +required
	Header gwv1.HTTPHeaderMatch `json:"header"`
}

// ResponseFlagFilter filters based on response flags.
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-responseflagfilter
type ResponseFlagFilter struct {
	// +kubebuilder:validation:MinItems=1
	// +required
	Flags []string `json:"flags"`
}

// CELFilter filters requests based on Common Expression Language (CEL).
type CELFilter struct {
	// The CEL expressions to evaluate. AccessLogs are only emitted when the CEL expressions evaluates to true.
	// see: https://www.envoyproxy.io/docs/envoy/v1.33.0/xds/type/v3/cel.proto.html#common-expression-language-cel-proto
	// +required
	Match string `json:"match"`
}

// GrpcStatusFilter filters gRPC requests based on their response status.
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#enum-config-accesslog-v3-grpcstatusfilter-status
type GrpcStatusFilter struct {
	// +kubebuilder:validation:MinItems=1
	// +optional
	Statuses []GrpcStatus `json:"statuses,omitempty"`
	// +optional
	Exclude *bool `json:"exclude,omitempty"`
}

// Tracing represents the top-level Envoy's tracer.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-tracing
type Tracing struct {
	// Provider defines the upstream to which envoy sends traces
	// +required
	Provider TracingProvider `json:"provider"`

	// Target percentage of requests managed by this HTTP connection manager that will be force traced if the x-client-trace-id header is set. Defaults to 100%
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	ClientSampling *int32 `json:"clientSampling,omitempty"`

	// Target percentage of requests managed by this HTTP connection manager that will be randomly selected for trace generation, if not requested by the client or not forced. Defaults to 100%
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	RandomSampling *int32 `json:"randomSampling,omitempty"`

	// Target percentage of requests managed by this HTTP connection manager that will be traced after all other sampling checks have been applied (client-directed, force tracing, random sampling). Defaults to 100%
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	OverallSampling *int32 `json:"overallSampling,omitempty"`

	// Whether to annotate spans with additional data. If true, spans will include logs for stream events. Defaults to false
	// +optional
	Verbose *bool `json:"verbose,omitempty"`

	// Maximum length of the request path to extract and include in the HttpUrl tag. Used to truncate lengthy request paths to meet the needs of a tracing backend. Default: 256
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxPathTagLength *int32 `json:"maxPathTagLength,omitempty"`

	// A list of attributes with a unique name to create attributes for the active span.
	// +optional
	Attributes []CustomAttribute `json:"attributes,omitempty"`

	// Create separate tracing span for each upstream request if true. Defaults to false
	// Link to envoy docs for more info
	// +optional
	SpawnUpstreamSpan *bool `json:"spawnUpstreamSpan,omitempty"`
}

// Describes attributes for the active span.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/tracing/v3/custom_tag.proto#envoy-v3-api-msg-type-tracing-v3-customtag
// +kubebuilder:validation:MaxProperties=2
// +kubebuilder:validation:MinProperties=1
type CustomAttribute struct {
	// The name of the attribute
	// +required
	Name string `json:"name"`

	// A literal attribute value.
	// +optional
	Literal *CustomAttributeLiteral `json:"literal,omitempty"`

	// An environment attribute value.
	// +optional
	Environment *CustomAttributeEnvironment `json:"environment,omitempty"`

	// A request header attribute value.
	// +optional
	RequestHeader *CustomAttributeHeader `json:"requestHeader,omitempty"`

	// An attribute to obtain the value from the metadata.
	// +optional
	Metadata *CustomAttributeMetadata `json:"metadata,omitempty"`
}

// Literal type attribute with a static value.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/tracing/v3/custom_tag.proto#type-tracing-v3-customtag-literal
type CustomAttributeLiteral struct {
	// Static literal value to populate the attribute value.
	// +required
	Value string `json:"value"`
}

// Environment type attribute with environment name and default value.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/tracing/v3/custom_tag.proto#type-tracing-v3-customtag-environment
type CustomAttributeEnvironment struct {
	// Environment variable name to obtain the value to populate the attribute value.
	// +required
	Name string `json:"name"`

	// When the environment variable is not found, the attribute value will be populated with this default value if specified,
	// otherwise no attribute will be populated.
	// +optional
	DefaultValue *string `json:"defaultValue,omitempty"`
}

// Header type attribute with header name and default value.
// https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/tracing/v3/custom_tag.proto#type-tracing-v3-customtag-header
type CustomAttributeHeader struct {
	// Header name to obtain the value to populate the attribute value.
	// +required
	Name string `json:"name"`

	// When the header does not exist, the attribute value will be populated with this default value if specified,
	// otherwise no attribute will be populated.
	// +optional
	DefaultValue *string `json:"defaultValue,omitempty"`
}

// Metadata type attribute using MetadataKey to retrieve the protobuf value from Metadata, and populate the attribute value with the canonical JSON representation of it.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/tracing/v3/custom_tag.proto#type-tracing-v3-customtag-metadata
type CustomAttributeMetadata struct {
	// Specify what kind of metadata to obtain attribute value from
	// +required
	Kind MetadataKind `json:"kind"`

	// Metadata key to define the path to retrieve the attribute value.
	// +required
	MetadataKey MetadataKey `json:"metadataKey"`

	// When no valid metadata is found, the attribute value would be populated with this default value if specified, otherwise no attribute would be populated.
	// +optional
	DefaultValue *string `json:"defaultValue,omitempty"`
}

// Describes different types of metadata sources.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/metadata/v3/metadata.proto#envoy-v3-api-msg-type-metadata-v3-metadatakind-request
// +kubebuilder:validation:Enum=Request;Route;Cluster;Host
type MetadataKind string

const (
	// Request kind of metadata.
	MetadataKindRequest MetadataKind = "Request"
	// Route kind of metadata.
	MetadataKindRoute MetadataKind = "Route"
	// Cluster kind of metadata.
	MetadataKindCluster MetadataKind = "Cluster"
	// Host kind of metadata.
	MetadataKindHost MetadataKind = "Host"
)

// MetadataKey provides a way to retrieve values from Metadata using a key and a path.
type MetadataKey struct {
	// The key name of the Metadata from which to retrieve the Struct
	// +required
	Key string `json:"key"`

	// The path used to retrieve a specific Value from the Struct. This can be either a prefix or a full path,
	// depending on the use case
	// +required
	Path []MetadataPathSegment `json:"path"`
}

// Specifies a segment in a path for retrieving values from Metadata.
type MetadataPathSegment struct {
	// The key used to retrieve the value in the struct
	// +required
	Key string `json:"key"`
}

// TracingProvider defines the list of providers for tracing
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type TracingProvider struct {
	// Tracing contains various settings for Envoy's OTel tracer.
	// +optional
	OpenTelemetry *OpenTelemetryTracingConfig `json:"openTelemetry,omitempty"`
}

// OpenTelemetryTracingConfig represents the top-level Envoy's OpenTelemetry tracer.
// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/trace/v3/opentelemetry.proto.html
type OpenTelemetryTracingConfig struct {
	// Send traces to the gRPC service
	// +required
	GrpcService CommonGrpcService `json:"grpcService"`

	// The name for the service. This will be populated in the ResourceSpan Resource attributes
	// Defaults to the envoy cluster name. Ie: `<gateway-name>.<gateway-namespace>`
	// +optional
	ServiceName *string `json:"serviceName,omitempty"`

	// An ordered list of resource detectors. Currently supported values are `EnvironmentResourceDetector`
	// +optional
	ResourceDetectors []ResourceDetector `json:"resourceDetectors,omitempty"`

	// Specifies the sampler to be used by the OpenTelemetry tracer. This field can be left empty. In this case, the default Envoy sampling decision is used.
	// Currently supported values are `AlwaysOn`
	// +optional
	Sampler *Sampler `json:"sampler,omitempty"`
}

// ResourceDetector defines the list of supported ResourceDetectors
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type ResourceDetector struct {
	// +optional
	EnvironmentResourceDetector *EnvironmentResourceDetectorConfig `json:"environmentResourceDetector,omitempty"`
}

// EnvironmentResourceDetectorConfig specified the EnvironmentResourceDetector
type EnvironmentResourceDetectorConfig struct{}

// Sampler defines the list of supported Samplers
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type Sampler struct {
	// +optional
	AlwaysOn *AlwaysOnConfig `json:"alwaysOnConfig,omitempty"`
}

// AlwaysOnConfig specified the AlwaysOn samplerc
type AlwaysOnConfig struct{}

// GrpcStatus represents possible gRPC statuses.
// +kubebuilder:validation:Enum=OK;CANCELED;UNKNOWN;INVALID_ARGUMENT;DEADLINE_EXCEEDED;NOT_FOUND;ALREADY_EXISTS;PERMISSION_DENIED;RESOURCE_EXHAUSTED;FAILED_PRECONDITION;ABORTED;OUT_OF_RANGE;UNIMPLEMENTED;INTERNAL;UNAVAILABLE;DATA_LOSS;UNAUTHENTICATED
type GrpcStatus string

const (
	OK                  GrpcStatus = "OK"
	CANCELED            GrpcStatus = "CANCELED"
	UNKNOWN             GrpcStatus = "UNKNOWN"
	INVALID_ARGUMENT    GrpcStatus = "INVALID_ARGUMENT"
	DEADLINE_EXCEEDED   GrpcStatus = "DEADLINE_EXCEEDED"
	NOT_FOUND           GrpcStatus = "NOT_FOUND"
	ALREADY_EXISTS      GrpcStatus = "ALREADY_EXISTS"
	PERMISSION_DENIED   GrpcStatus = "PERMISSION_DENIED"
	RESOURCE_EXHAUSTED  GrpcStatus = "RESOURCE_EXHAUSTED"
	FAILED_PRECONDITION GrpcStatus = "FAILED_PRECONDITION"
	ABORTED             GrpcStatus = "ABORTED"
	OUT_OF_RANGE        GrpcStatus = "OUT_OF_RANGE"
	UNIMPLEMENTED       GrpcStatus = "UNIMPLEMENTED"
	INTERNAL            GrpcStatus = "INTERNAL"
	UNAVAILABLE         GrpcStatus = "UNAVAILABLE"
	DATA_LOSS           GrpcStatus = "DATA_LOSS"
	UNAUTHENTICATED     GrpcStatus = "UNAUTHENTICATED"
)

// UpgradeConfig represents configuration for HTTP upgrades.
type UpgradeConfig struct {
	// List of upgrade types to enable (e.g. "websocket", "CONNECT", etc.)
	// +kubebuilder:validation:MinItems=1
	// +optional
	EnabledUpgrades []string `json:"enabledUpgrades,omitempty"`
}

// ServerHeaderTransformation determines how the server header is transformed.
type ServerHeaderTransformation string

const (
	// OverwriteServerHeaderTransformation overwrites the server header.
	OverwriteServerHeaderTransformation ServerHeaderTransformation = "Overwrite"
	// AppendIfAbsentServerHeaderTransformation appends to the server header if it's not present.
	AppendIfAbsentServerHeaderTransformation ServerHeaderTransformation = "AppendIfAbsent"
	// PassThroughServerHeaderTransformation passes through the server header unchanged.
	PassThroughServerHeaderTransformation ServerHeaderTransformation = "PassThrough"
)

// EnvoyHealthCheck represents configuration for Envoy's health check filter.
// The filter will be configured in No pass through mode, and will only match requests with the specified path.
type EnvoyHealthCheck struct {
	// Path defines the exact path that will be matched for health check requests.
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern="^/[-a-zA-Z0-9@:%.+~#?&/=_]+$"
	// +required
	Path string `json:"path"`
}

// UuidRequestIdConfig configures the UUID request ID extension.
// Based on: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/request_id/uuid/v3/uuid.proto
type UuidRequestIdConfig struct {
	// PackTraceReason determines if the trace sampling decision is embedded into the UUID.
	// Defaults to true. Set to false to prevent Envoy from mutating the Request ID,
	// which is useful when preserving exact UUIDs from external systems.
	// +optional
	PackTraceReason *bool `json:"packTraceReason,omitempty"`

	// UseRequestIDForTraceSampling determines if the Request ID is used to calculate the
	// trace sampling decision. Defaults to true. This ensures consistent sampling decisions
	// for a given Request ID across the mesh.
	// +optional
	UseRequestIDForTraceSampling *bool `json:"useRequestIdForTraceSampling,omitempty"`
}
