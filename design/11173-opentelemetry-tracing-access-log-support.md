# EP-11173: OpenTelemetry Tracing & Access Log Support

* Issue: [#11173](https://github.com/kgateway-dev/kgateway/issues/11173)
* Issue: [#11226](https://github.com/kgateway-dev/kgateway/issues/11226)

## Background

### OpenTelemetry Tracing Support

Envoy supports [tracing](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/observability/tracing) via [different end-to-end tracers](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/trace/trace) to obtain visibility and track requests as they pass through the API gateway to distributed backends, such as services, databases, or other endpoints.
One such method is via [OpenTelemetry](https://opentelemetry.io/) (OTel). It provides a standardized protocol for reporting traces, and a standardized collector through which to receive metrics. Additionally, OTel supports exporting traces to several distributed tracing platforms, and metrics to [compatible observability backends](https://opentelemetry.io/ecosystem/vendors/). For the full list of supported platforms, see the OTel GitHub repository.
To trace a request, data must be captured from the moment the request is initiated and every time the request is forwarded to another endpoint, or when other microservices are called along the way. When a request is initiated, a trace ID and an initial span (parent span) are created. A span represents an operation that is performed on your request, such as an API call, a database lookup, or a call to an external service. If a request is sent to a service, a child span is created in the trace, capturing all the operations that are performed within the service.
Each operation and span is documented with a timestamp so that you can easily see how long a request was processed by a specific endpoint in your trace. Most tracing platforms have support to visualize the tracing information in a graph so that a user can easily see bottlenecks in the microservices stack.

### OpenTelemetry Access Log Support

Envoy also supports [access logging](https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/access_log/usage), along with various [way to export them ](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/accesslog/accesslog).
Access logs represent all traffic requests that pass through the gateway proxy. The access log entries can be customized to include data from the request, the routing destination, and the response.
Kgateway currently supports writing envoy access logs [to a file](https://kgateway.dev/docs/security/access-logging/#access-log-stdout-filesink). This can be a simple and quick method to get access logs without the need for any external service to receive and store the access logs, however, it is limited by disk space consumption, log rotation, manual analysis and potential security/compliance concerns when storing the logs as an unencrypted file
Kgateway also supports sending access logs to an upstream gRPC service, but this too has its limitations as it does not provide flexibility of adding additional attributes or a custom body/formatters.
[OTel logging](https://opentelemetry.io/docs/specs/otel/logs/) provides a standard for logs to propagate and corelate them with tracers and metrics. It does so based on the [log data model](https://opentelemetry.io/docs/specs/otel/logs/data-model/) that includes processors that operate on logs which supports receiving common log formats. It provides a standardized collector through which to receive logs, process and export them to a backend system for storage, analysis, and visualization.

## Motivation

Within a network, requests flow through multiple services, databases, or other endpoints. This request level telemetry data can be captured from when a request enters a gateway until it leaves to help visualize and understand this flow by tracking each segment of the request as it moves through the system, allowing developers to measure user specific actions, identify performance bottlenecks and errors.
Users are using the envoy OpenTelemetry tracer in production (https://github.com/envoyproxy/envoy/issues/24672#issuecomment-2592932718) and this functionality should be exposed through kgateway.

The [OTel access log sink](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/access_loggers/open_telemetry/v3/logs_service.proto) provides greater flexibility to add metadata to access logs such as [attributes](https://opentelemetry.io/docs/specs/semconv/general/attributes/), [formatters](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/formatter/formatter), and providing a standard way to propagate and record logs.

## Goals

- Users should be able to configure OTel tracing via kgateway
- Users should be able to configure OTel access logging via kgateway
- Support tracing at a Gateway level
- Provide documentation and example configurations for integrating kgateway with popular observability platforms (e.g., Jaeger, Datadog)
- Maintain [existing access logging support](https://kgateway.dev/docs/security/access-logging/)

## Non-Goals

- Deploying an OTel Collector as part of the kgateway Helm chart
- Providing opinionated configurations with specific observability vendors or backends out of the box (integrations with kgateway should be vendor agnostic)
- Support [per-route](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#config-route-v3-tracing) tracing
- Support configuring it globally (via Gateway Parameters)

## Implementation Details

### Configuration

A user should be able to configure an OTel tracing and accessLog config for a Gateway via the `HTTPListenerPolicy`:
```
apiVersion: gateway.kgateway.dev/v1alpha1
kind: HTTPListenerPolicy
  name: tracing-config
spec:
  tracing:
    provider:
      openTelemetry:
        grpcService:
          serviceName: tracing-service
          backendRef:
            name: gateway-proxy-tracing-server
            namespace: default
            port: 8083
          resourceDetectors:
            - EnvironmentResourceDetector: {}
  accessLog:
    - openTelemetry:
        grpcService:
          logName: test-accesslog-service
          backendRef:
            name: gateway-proxy-access-logger
            namespace: default
            port: 8083
          body: '[%START_TIME%] "%PROTOCOL%" %RESPONSE_CODE%\n'
          attributes:
            values:
              - key: "user-agent"
                value:
                  string_value: "%REQ(USER-AGENT)%"
              - key: "duration"
                value:
                  double_value: "%DURATION%"
```

The `http_listener_policy_types.go` will be modified to add the following fields :
```go
type HTTPListenerPolicySpec struct {
    // Tracing contains various settings for Envoy's OpenTelemetry tracer.
    // See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/trace/v3/opentelemetry.proto.html
    Tracing *Tracing `json:"tracing,omitempty"`
}

// Tracing represents the top-level Envoy's tracer.
type Tracing struct {
    // Provider defines the upstream to which envoy sends traces
    // +kubebuilder:validation:Required
    Provider *Provider `json:"provider"`

    // Target percentage of requests managed by this HTTP connection manager that will be force traced if the x-client-trace-id header is set. Defaults to 100%
    ClientSampling *float64 `json:"clientSampling,omitempty"`

    // Target percentage of requests managed by this HTTP connection manager that will be randomly selected for trace generation, if not requested by the client or not forced. Defaults to 100%
    RandomSampling *float64 `json:"randomSampling,omitempty"`

    // Target percentage of requests managed by this HTTP connection manager that will be traced after all other sampling checks have been applied (client-directed, force tracing, random sampling). Defaults to 100%
    OverallSampling *float64 `json:"overallSampling,omitempty"`

    // Whether to annotate spans with additional data. If true, spans will include logs for stream events. Defaults to false
    Verbose *bool `json:"verbose,omitempty"`

    // Maximum length of the request path to extract and include in the HttpUrl tag. Used to truncate lengthy request paths to meet the needs of a tracing backend. Default: 256
    MaxPathTagLength *uint32 `json:"maxPathTagLength,omitempty"`

    // A list of custom tags with unique tag name to create tags for the active span.
    CustomTags []*CustomTag `json:"customTags,omitempty"`

    // Create separate tracing span for each upstream request if true. Defaults to false
    // Link to envoy docs for more info
    SpawnUpstreamSpan *bool `json:"spawnUpstreamSpan,omitempty"`
}

// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type Provider struct {
    // This could be wrapped in a provider struct if future configs need to be added or use cel validation to ensure only one provider can be selected.
    // Tracing contains various settings for Envoy's OTel tracer.
    // +kubebuilder:validation:Required
    OpenTelemetryConfig *OpenTelemetryTracingConfig `json:"openTelemetryConfig,omitempty"`
}

// OpenTelemetryTracingConfig represents the top-level Envoy's OpenTelemetry tracer.
// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/trace/v3/opentelemetry.proto.html
// OpenTelemetryTracingConfig represents the top-level Envoy's OpenTelemetry tracer.
// See here for more information: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/trace/v3/opentelemetry.proto.html
type OpenTelemetryTracingConfig struct {
    // Send traces to the gRPC service
    // +kubebuilder:validation:Required
    GrpcService *CommonGrpcService `json:"grpcService,omitempty"`

    // The name for the service. This will be populated in the ResourceSpan Resource attributes
    // +kubebuilder:validation:Required
    ServiceName string `json:"serviceName,omitempty"`

    // An ordered list of resource detectors. Currently supported values are `EnvironmentResourceDetector`
    ResourceDetectors []*ResourceDetector `json:"resourceDetectors,omitempty"`

    // Specifies the sampler to be used by the OpenTelemetry tracer. This field can be left empty. In this case, the default Envoy sampling decision is used.
    // Currently supported values are `AlwaysOn`
    Sampler *Sampler `json:"sampler,omitempty"`
}

// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type ResourceDetector struct {
	  EnvironmentResourceDetector *EnvironmentResourceDetectorConfig `json:"environmentResourceDetector,omitempty"`
}

type EnvironmentResourceDetectorConfig struct {
}

// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type Sampler struct {
	  AlwaysOn *AlwaysOnConfig `json:"alwaysOnConfig,omitempty"`
}

type AlwaysOnConfig struct{}


type OTelGrpcService struct {
    // The backend gRPC service. Can be any type of supported backend (Kubernetes Service, kgateway Backend, etc..)
    // +kubebuilder:validation:Required
    BackendRef *gwv1.BackendRef `json:"backendRef"`

    // The :authority header in the grpc request. If this field is not set, the authority header value will be cluster_name.
    // Note that this authority does not override the SNI. The SNI is provided by the transport socket of the cluster.
    Authority *string `json:"authority,omitempty"`

    // Indicates the retry policy for re-establishing the gRPC stream.
    // If max interval is not provided, it will be set to ten times the provided base interval
    RetryPolicy *RetryPolicy `json:"retryPolicy,omitempty"`

    // Maximum gRPC message size that is allowed to be received. If a message over this limit is received, the gRPC stream is terminated with the RESOURCE_EXHAUSTED error.
    // Defaults to 0, which means unlimited.
    MaxReceiveMessageLength *uint32 `json:"maxReceiveMessageLength,omitempty"`

    // This provides gRPC client level control over envoy generated headers. If false, the header will be sent but it can be overridden by per stream option. If true, the header will be removed and can not be overridden by per stream option. Default to false.
    SkipEnvoyHeaders *bool `json:"skipEnvoyHeaders,omitempty"`

    // The timeout for the gRPC request. This is the timeout for a specific request
    Timeout *metav1.Duration `json:"timeout,omitempty"`

    // Additional metadata to include in streams initiated to the GrpcService.
    // This can be used for scenarios in which additional ad hoc authorization headers (e.g. x-foo-bar: baz-key) are to be injected
    InitialMetadata []*HeaderValue `json:"initialMetadata,omitempty"`
}

// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type ResourceDetector struct {
    EnvironmentResourceDetector EnvironmentResourceDetector `json:"environmentResourceDetector,omitempty"`
    // New detectors can be added as needed
}

// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type Sampler struct {
    // This is just an example. New samplers can be added
    AlwaysOn AlwaysOn `json:"alwaysOn,omitempty"`
}

type EnvironmentResourceDetector struct {}
```
The existing AccessLog struct will be modified to add support for OTel access logs
```go
type AccessLog struct {
    // Send access logs to an OTel collector
    OpenTelemetryConfig OpenTelemetryAccessLogConfig `json:"openTelemetry,omitempty"`
}

// OpenTelemetryAccessLogConfig represents the OTel configuration for access logs.
type OpenTelemetryAccessLogConfig struct {
    // Send access logs to gRPC service
    // +kubebuilder:validation:Required
    GrpcService *CommonAccessLogGrpcService `json:"grpcService,omitempty"`

    // OpenTelemetry LogResource fields, following Envoy access logging formatting.
    // +kubebuilder:validation:Optional
    Body *string `json:"body,omitempty"`

    // If specified, Envoy will not generate built-in resource labels like log_name, zone_name, cluster_name, node_name.
    // +kubebuilder:validation:Optional
    DisableBuiltinLabels *bool `json:"disableBuiltinLabels,omitempty"`

    // Additional attributes that describe the specific event occurrence.
    // +kubebuilder:validation:Optional
    Attributes *KeyValueList `json:"attributes,omitempty"`
}

// CommonAccessLogGrpcService is the common config shared between grpc access logs and OTel access logs
type CommonAccessLogGrpcService struct {
    // Embedded to avoid breaking changes
    *CommonGrpcService `json:",inline"`

    // name of log stream
    // +kubebuilder:validation:Required
    LogName string `json:"logName"`
}


// CommonGrpcService is the common grpc service config shared amongst OTel grpc configs and grpc access logs configs
type CommonGrpcService struct {
    // The backend gRPC service. Can be any type of supported backend (Kubernetes Service, kgateway Backend, etc..)
    // +kubebuilder:validation:Required
    BackendRef *gwv1.BackendRef `json:"backendRef"`

    // The :authority header in the grpc request. If this field is not set, the authority header value will be cluster_name.
    // Note that this authority does not override the SNI. The SNI is provided by the transport socket of the cluster.
    Authority *string `json:"authority,omitempty"`

    // Maximum gRPC message size that is allowed to be received. If a message over this limit is received, the gRPC stream is terminated with the RESOURCE_EXHAUSTED error.
    // Defaults to 0, which means unlimited.
    MaxReceiveMessageLength *uint32 `json:"maxReceiveMessageLength,omitempty"`

    // This provides gRPC client level control over envoy generated headers. If false, the header will be sent but it can be overridden by per stream option. If true, the header will be removed and can not be overridden by per stream option. Default to false.
    SkipEnvoyHeaders *bool `json:"skipEnvoyHeaders,omitempty"`

    // The timeout for the gRPC request. This is the timeout for a specific request
    Timeout *metav1.Duration `json:"timeout,omitempty"`

    // Additional metadata to include in streams initiated to the GrpcService.
    // This can be used for scenarios in which additional ad hoc authorization headers (e.g. x-foo-bar: baz-key) are to be injected
    InitialMetadata []*HeaderValue `json:"initialMetadata,omitempty"`

    // Indicates the retry policy for re-establishing the gRPC stream.
    // If max interval is not provided, it will be set to ten times the provided base interval
    RetryPolicy RetryPolicy `json:"retryPolicy,omitempty"`
}
```

### Plugin

The existing `httplistenerpolicy` plugin performs the access log translation and can be updated to support OTel access log and tracing.

### Controllers

No new controllers will be required for this feature. The existing plugin will be updated to support this.

### Deployer

The CRDs will be updated with the new fields along with the required validation

### Test Plan

The testing plan will include unit and kubernetes end to end tests to verify the feature

## Alternatives

- The tracing config can be defined in the `GatewayExtension`, however this could lead to invalid configs where a user might inadvertently apply it where it is not supported, eg: an HTTPRoute.

## Open Questions

- Which [resource detectors](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/tracers/opentelemetry/resource_detectors/v3/environment_resource_detector.proto) should be supported ?
- Should HTTP services be supported for tracing (https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/http_service.proto#envoy-v3-api-msg-config-core-v3-httpservice) ?
