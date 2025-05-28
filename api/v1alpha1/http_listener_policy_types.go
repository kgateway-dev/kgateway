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

type HTTPListenerPolicySpec struct {
	// TargetRefs specifies the target resources by reference to attach the policy to.
	// +optional
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	TargetRefs []LocalPolicyTargetReference `json:"targetRefs,omitempty"`

	// TargetSelectors specifies the target selectors to select resources to attach the policy to.
	// +optional
	TargetSelectors []LocalPolicyTargetSelector `json:"targetSelectors,omitempty"`

	// AccessLoggingConfig contains various settings for Envoy's access logging service.
	// See here for more information: https://www.envoyproxy.io/docs/envoy/v1.33.0/api-v3/config/accesslog/v3/accesslog.proto
	// +kubebuilder:validation:Items={type=object}
	AccessLog []AccessLog `json:"accessLog,omitempty"`
}

// AccessLog represents the top-level access log configuration.
type AccessLog struct {
	// Output access logs to local file
	FileSink *FileSink `json:"fileSink,omitempty"`

	// Send access logs to gRPC service
	GrpcService *GrpcService `json:"grpcService,omitempty"`

	// Filter access logs configuration
	Filter *AccessLogFilter `json:"filter,omitempty"`
}

// FileSink represents the file sink configuration for access logs.
// +kubebuilder:validation:XValidation:message="only one of 'StringFormat' or 'JsonFormat' may be set",rule="(has(self.stringFormat) && !has(self.jsonFormat)) || (!has(self.stringFormat) && has(self.jsonFormat))"
type FileSink struct {
	// the file path to which the file access logging service will sink
	// +kubebuilder:validation:Required
	Path string `json:"path"`
	// the format string by which envoy will format the log lines
	// https://www.envoyproxy.io/docs/envoy/v1.33.0/configuration/observability/access_log/usage#format-strings
	StringFormat string `json:"stringFormat,omitempty"`
	// the format object by which to envoy will emit the logs in a structured way.
	// https://www.envoyproxy.io/docs/envoy/v1.33.0/configuration/observability/access_log/usage#format-dictionaries
	JsonFormat *runtime.RawExtension `json:"jsonFormat,omitempty"`
}

// GrpcService represents the gRPC service configuration for access logs.
type GrpcService struct {
	// name of log stream
	// +kubebuilder:validation:Required
	LogName string `json:"logName"`

	// The backend gRPC service. Can be any type of supported backend (Kubernetes Service, kgateway Backend, etc..)
	// +kubebuilder:validation:Required
	BackendRef *gwv1.BackendRef `json:"backendRef"`

	// Additional request headers to log in the access log
	AdditionalRequestHeadersToLog []string `json:"additionalRequestHeadersToLog,omitempty"`

	// Additional response headers to log in the access log
	AdditionalResponseHeadersToLog []string `json:"additionalResponseHeadersToLog,omitempty"`

	// Additional response trailers to log in the access log
	AdditionalResponseTrailersToLog []string `json:"additionalResponseTrailersToLog,omitempty"`
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
	// +kubebuilder:validation:Required
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
	// +kubebuilder:validation:Required
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
