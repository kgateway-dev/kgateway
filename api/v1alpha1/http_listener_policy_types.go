package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=httplistenerpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=httplistenerpolicies/status,verbs=get;update;patch

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="gateway.networking.k8s.io/policy=Direct"
// HTTPListenerPolicy is intended to be used for configuring the Envoy `HttpConnectionManager` and any other config or policy
// that should map 1-to-1 with a given HTTP listener, such as the Envoy health check HTTP filter.
// Currently these policies can only be applied per `Gateway` but support for `Listener` attachment may be added in the future.
// See https://github.com/kgateway-dev/kgateway/issues/11786 for more details.
type HTTPListenerPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec HTTPListenerPolicySpec `json:"spec,omitempty"`

	Status gwv1alpha2.PolicyStatus `json:"status,omitempty"`
	// TODO: embed this into a typed Status field when
	// https://github.com/kubernetes/kubernetes/issues/131533 is resolved
}

// +kubebuilder:object:root=true
type HTTPListenerPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HTTPListenerPolicy `json:"items"`
}

// HTTPListenerPolicySpec defines the desired state of a HTTP listener policy.
type HTTPListenerPolicySpec struct {
	// TargetRefs specifies the target resources by reference to attach the policy to.
	// +optional
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:rule="self.all(r, r.kind == 'Gateway' && (!has(r.group) || r.group == 'gateway.networking.k8s.io'))",message="targetRefs may only reference Gateway resources"
	TargetRefs []LocalPolicyTargetReference `json:"targetRefs,omitempty"`

	// TargetSelectors specifies the target selectors to select resources to attach the policy to.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.all(r, r.kind == 'Gateway' && (!has(r.group) || r.group == 'gateway.networking.k8s.io'))",message="targetSelectors may only reference Gateway resources"
	TargetSelectors []LocalPolicyTargetSelector `json:"targetSelectors,omitempty"`

	// AccessLoggingConfig contains various settings for Envoy's access logging service.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto
	// +kubebuilder:validation:Items={type=object}
	//
	// +kubebuilder:validation:MaxItems=16
	AccessLog []AccessLog `json:"accessLog,omitempty"`

	// Tracing contains various settings for Envoy's OpenTelemetry tracer.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/trace/v3/opentelemetry.proto.html
	// +optional
	Tracing *Tracing `json:"tracing,omitempty"`

	// UpgradeConfig contains configuration for HTTP upgrades like WebSocket.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/v1.34.1/intro/arch_overview/http/upgrades.html
	UpgradeConfig *UpgradeConfig `json:"upgradeConfig,omitempty"`

	// UseRemoteAddress determines whether to use the remote address for the original client.
	// Note: If this field is omitted, it will fallback to the default value of 'true', which we set for all Envoy HCMs.
	// Thus, setting this explicitly to true is unnecessary (but will not cause any harm).
	// When true, Envoy will use the remote address of the connection as the client address.
	// When false, Envoy will use the X-Forwarded-For header to determine the client address.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-use-remote-address
	// +optional
	UseRemoteAddress *bool `json:"useRemoteAddress,omitempty"`

	// XffNumTrustedHops is the number of additional ingress proxy hops from the right side of the X-Forwarded-For HTTP header to trust when determining the origin client's IP address.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-xff-num-trusted-hops
	// +kubebuilder:validation:Minimum=0
	// +optional
	XffNumTrustedHops *uint32 `json:"xffNumTrustedHops,omitempty"`

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

	// HealthCheck configures [Envoy health checks](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/health_check/v3/health_check.proto)
	// +optional
	HealthCheck *EnvoyHealthCheck `json:"healthCheck,omitempty"`

	// PreserveHttp1HeaderCase determines whether to preserve the case of HTTP1 request headers.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/header_casing
	// +optional
	PreserveHttp1HeaderCase *bool `json:"preserveHttp1HeaderCase,omitempty"`
}

// AccessLog represents the top-level access log configuration.
type AccessLog struct {
	// Output access logs to local file
	FileSink *FileSink `json:"fileSink,omitempty"`

	// Send access logs to gRPC service
	GrpcService *AccessLogGrpcService `json:"grpcService,omitempty"`

	// Send access logs to an OTel collector
	OpenTelemetry *OpenTelemetryAccessLogService `json:"openTelemetry,omitempty"`

	// Filter access logs configuration
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
	StringFormat string `json:"stringFormat,omitempty"`
	// the format object by which to envoy will emit the logs in a structured way.
	// https://www.envoyproxy.io/docs/envoy/v1.33.0/configuration/observability/access_log/usage#format-dictionaries
	JsonFormat *runtime.RawExtension `json:"jsonFormat,omitempty"`
}

// AccessLogGrpcService represents the gRPC service configuration for access logs.
// Ref: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/access_loggers/grpc/v3/als.proto#envoy-v3-api-msg-extensions-access-loggers-grpc-v3-httpgrpcaccesslogconfig
type AccessLogGrpcService struct {
	CommonAccessLogGrpcService `json:",inline"`

	// Additional request headers to log in the access log
	AdditionalRequestHeadersToLog []string `json:"additionalRequestHeadersToLog,omitempty"`

	// Additional response headers to log in the access log
	AdditionalResponseHeadersToLog []string `json:"additionalResponseHeadersToLog,omitempty"`

	// Additional response trailers to log in the access log
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
	BackendRef *gwv1.BackendRef `json:"backendRef"`

	// The :authority header in the grpc request. If this field is not set, the authority header value will be cluster_name.
	// Note that this authority does not override the SNI. The SNI is provided by the transport socket of the cluster.
	// +optional
	Authority *string `json:"authority,omitempty"`

	// Maximum gRPC message size that is allowed to be received. If a message over this limit is received, the gRPC stream is terminated with the RESOURCE_EXHAUSTED error.
	// Defaults to 0, which means unlimited.
	// +optional
	MaxReceiveMessageLength *uint32 `json:"maxReceiveMessageLength,omitempty"`

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
	NumRetries *uint32 `json:"numRetries,omitempty"`
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
	StringValue *string `json:"stringValue,omitempty"`
	// TODO: Add support for ArrayValue && KvListValue
	// +kubebuilder:validation:items:Type=object
	// +kubebuilder:validation:items:XPreserveUnknownFields
	ArrayValue []AnyValue `json:"arrayValue,omitempty"`
	// +kubebuilder:validation:Type=object
	// +kubebuilder:validation:XPreserveUnknownFields
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
	AndFilter []FilterType `json:"andFilter,omitempty"`
	// Performs a logical "or" operation on the result of each individual filter.
	// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-orfilter
	// +kubebuilder:validation:MinItems=2
	OrFilter []FilterType `json:"orFilter,omitempty"`
}

// FilterType represents the type of filter to apply (only one of these should be set).
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#envoy-v3-api-msg-config-accesslog-v3-accesslogfilter
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type FilterType struct {
	StatusCodeFilter *StatusCodeFilter `json:"statusCodeFilter,omitempty"`
	DurationFilter   *DurationFilter   `json:"durationFilter,omitempty"`
	// Filters for requests that are not health check requests.
	// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-nothealthcheckfilter
	NotHealthCheckFilter bool `json:"notHealthCheckFilter,omitempty"`
	// Filters for requests that are traceable.
	// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-traceablefilter
	TraceableFilter    bool                `json:"traceableFilter,omitempty"`
	HeaderFilter       *HeaderFilter       `json:"headerFilter,omitempty"`
	ResponseFlagFilter *ResponseFlagFilter `json:"responseFlagFilter,omitempty"`
	GrpcStatusFilter   *GrpcStatusFilter   `json:"grpcStatusFilter,omitempty"`
	CELFilter          *CELFilter          `json:"celFilter,omitempty"`
}

// ComparisonFilter represents a filter based on a comparison.
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#config-accesslog-v3-comparisonfilter
type ComparisonFilter struct {
	// +required
	Op Op `json:"op,omitempty"`

	// Value to compare against.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	Value uint32 `json:"value,omitempty"`
}

// Op represents comparison operators.
// +kubebuilder:validation:Enum=EQ;GE;LE
type Op string

const (
	EQ Op = "EQ" // Equal
	GE Op = "GQ" // Greater or equal
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
	Flags []string `json:"flags"`
}

// CELFilter filters requests based on Common Expression Language (CEL).
type CELFilter struct {
	// The CEL expressions to evaluate. AccessLogs are only emitted when the CEL expressions evaluates to true.
	// see: https://www.envoyproxy.io/docs/envoy/v1.33.0/xds/type/v3/cel.proto.html#common-expression-language-cel-proto
	Match string `json:"match"`
}

// GrpcStatusFilter filters gRPC requests based on their response status.
// Based on: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto#enum-config-accesslog-v3-grpcstatusfilter-status
type GrpcStatusFilter struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Items={type=object}
	Statuses []GrpcStatus `json:"statuses,omitempty"`
	Exclude  bool         `json:"exclude,omitempty"`
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
	ClientSampling *uint32 `json:"clientSampling,omitempty"`

	// Target percentage of requests managed by this HTTP connection manager that will be randomly selected for trace generation, if not requested by the client or not forced. Defaults to 100%
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	RandomSampling *uint32 `json:"randomSampling,omitempty"`

	// Target percentage of requests managed by this HTTP connection manager that will be traced after all other sampling checks have been applied (client-directed, force tracing, random sampling). Defaults to 100%
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	OverallSampling *uint32 `json:"overallSampling,omitempty"`

	// Whether to annotate spans with additional data. If true, spans will include logs for stream events. Defaults to false
	// +optional
	Verbose *bool `json:"verbose,omitempty"`

	// Maximum length of the request path to extract and include in the HttpUrl tag. Used to truncate lengthy request paths to meet the needs of a tracing backend. Default: 256
	// +optional
	MaxPathTagLength *uint32 `json:"maxPathTagLength,omitempty"`

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
	ServiceName *string `json:"serviceName"`

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
	EnvironmentResourceDetector *EnvironmentResourceDetectorConfig `json:"environmentResourceDetector,omitempty"`
}

// EnvironmentResourceDetectorConfig specified the EnvironmentResourceDetector
type EnvironmentResourceDetectorConfig struct{}

// Sampler defines the list of supported Samplers
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type Sampler struct {
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
	Path string `json:"path"`
}
