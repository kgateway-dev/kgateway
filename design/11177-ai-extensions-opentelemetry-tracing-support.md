# EP-11177: AI Extensions OpenTelemetry Tracing Support

* Issue: [#11177](https://github.com/kgateway-dev/kgateway/issues/11177)

## Background

Currently, AI Gateway has introduced Metrics in the observability domain (Logs, Metrics, Traces). However, when issues occur, Metrics can only tell us 'What' happened, but not 'Where' the issue occurred. The proposal is to implement tracing instrumentation in AI Gateway and provide a way to export data to OTLP-compatible backend storage (Jaeger, Datadog) or a more flexible OTel Collector.

## Motivation

### Goals

- Introduce new Gateway Parameter for configuring [OpenTelemetry tracer](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/trace/v3/opentelemetry.proto.html) and [Span Exporter](https://opentelemetry.io/docs/languages/python/exporters/#usage) to export data to OTel Collector or OTLP-compatible backend storage.
- Implement instrumentation in the [ai extension server](https://github.com/kgateway-dev/kgateway/blob/2bd89e9e19bf017b9a1e86350808656eed6b4b94/python/ai_extension/ext_proc/server.py) and export traces to the configured backend storage.
- Allow user to enable or disable this feature via Helm.
- Provide E2E tests to ensure trace data is completely stored in the backend storage.
- Provide users with a quick start guide.

### Non-Goals

- Integrating OTel Collector as a plugin into Gateway (For kgateway, OTel Collector should be a separate service, and its control should be left to users)

## Implementation Details

### API Changes

Add new tracing field to GatewayParameters, which is an optional feature. When enabled, it will export traces instrumented in the extproc server to OTel Collector or OTLP-compatible backend storage (Jaeger, Datadog).

Here are the key reasons why this configuration should reside within GatewayParameters:

This configuration is essential at server startup. For example, the [OTel TracerProvider](https://github.com/kgateway-dev/kgateway/blob/main/python/ai_extension/ext_proc/server.py#L734) requires these settings for proper initialization.
Unlike Envoy tracing, the extproc server's tracing configuration cannot currently be updated dynamically via xDS.

```go
// AiExtensionTrace defines the tracing configuration for the AI extension
type AiExtensionTrace struct {
	// Enabled controls whether tracing is enabled
	// +kubebuilder:validation:Required
	Enabled *bool `json:"enabled,omitempty"`

	// EndPoint specifies the URL of the OTLP Exporter for traces.
	// Example: "http://my-otel-collector.svc.cluster.local:4317"
	// https://opentelemetry.io/docs/languages/sdk-configuration/otlp-exporter/#otel_exporter_otlp_traces_endpoint
	// +kubebuilder:validation:Required
	EndPoint gwv1.AbsoluteURI `json:"endpoint"`

	// Sampler defines the sampling strategy for OpenTelemetry traces.
	// Sampling helps in reducing the volume of trace data by selectively
	// recording only a subset of traces.
	// https://opentelemetry.io/docs/languages/sdk-configuration/general/#otel_traces_sampler
	// +kubebuilder:validation:option
	Sampler OTelTracesSampler `json:"sampler,omitempty"`

	// OTLPTimeout specifies timeout configurations for OTLP (OpenTelemetry Protocol) exports.
	// It allows setting general and trace-specific timeouts for sending data.
	// https://opentelemetry.io/docs/languages/sdk-configuration/otlp-exporter/#otel_exporter_otlp_traces_timeout
	// +kubebuilder:validation:option
	Timeout time.Duration `json:"timeout,omitempty"`

	// OTLPProtocol specifies the protocol to be used for OTLP exports.
	// This determines how tracing data is serialized and transported (e.g., gRPC, HTTP/Protobuf).
	// https://opentelemetry.io/docs/languages/sdk-configuration/otlp-exporter/#otel_exporter_otlp_traces_protocol
	// +kubebuilder:validation:option
	Protocol OTLPTracesProtocolType `json:"protocol,omitempty"`

	// TransportSecurity controls the TLS (Transport Layer Security) settings when connecting
	// to the tracing server. It determines whether certificate verification should be skipped.
	// +kubebuilder:validation:option
	TransportSecurity OTLPTransportSecurityMode `json:"transportSecurity,omitempty"`
}

// OTelTracesSampler defines the configuration for an OpenTelemetry trace sampler.
// It combines the sampler type with any required arguments for that type.
type OTelTracesSampler struct {
	// SamplerType specifies the type of sampler to use (default value: "parentbased_always_on").
	// Refer to OTelTracesSamplerType for available options.
	// https://opentelemetry.io/docs/languages/sdk-configuration/general/#otel_traces_sampler
	SamplerType OTelTracesSamplerType `json:"type"`
	// SamplerArg provides an argument for the chosen sampler type.
	// For "traceidratio" or "parentbased_traceidratio" samplers: Sampling probability, a number in the [0..1] range,
	// e.g. 0.25. Default is 1.0 if unset.
	// https://opentelemetry.io/docs/languages/sdk-configuration/general/#otel_traces_sampler_arg
	SamplerArg float64 `json:"arg"`
}

// OTLPTracesProtocolType defines the supported protocols for OTLP exporter.
type OTLPTracesProtocolType string

const (
	// OTLPTracesProtocolTypeGrpc specifies OTLP over gRPC protocol.
	// This is typically the most efficient protocol for OpenTelemetry data transfer.
	OTLPTracesProtocolTypeGrpc OTLPTracesProtocolType = "grpc"
	// OTLPTracesProtocolTypeProtobuf specifies OTLP over HTTP with Protobuf serialization.
	// Data is sent via HTTP POST requests with Protobuf message bodies.
	OTLPTracesProtocolTypeProtobuf OTLPTracesProtocolType = "http/protobuf"
	// OTLPTracesProtocolTypeJson specifies OTLP over HTTP with JSON serialization.
	// Data is sent via HTTP POST requests with JSON message bodies.
	OTLPTracesProtocolTypeJson OTLPTracesProtocolType = "http/json"
)

// OTLPTransportSecurityMode defines the transport security options for OTLP connections.
type OTLPTransportSecurityMode string

const (
	// OTLPTransportSecuritySecure enables TLS (client transport security) for OTLP connections.
	// This means the client will verify the server's certificate.
	OTLPTransportSecuritySecure OTLPTransportSecurityMode = "secure"

	// OTLPTransportSecurityInsecure disables TLS for OTLP connections,
	// meaning certificate verification is skipped. This is generally not recommended
	// for production environments due to security risks.
	OTLPTransportSecurityInsecure OTLPTransportSecurityMode = "insecure"
)

// OTelTracesSamplerType defines the available OpenTelemetry trace sampler types.
// These samplers determine which traces are recorded and exported.
type OTelTracesSamplerType string

const (
	// OTelTracesSamplerAlwaysOn enables always-on sampling.
	// All traces will be recorded and exported. Useful for development or low-traffic systems.
	OTelTracesSamplerAlwaysOn OTelTracesSamplerType = "always_on"

	// OTelTracesSamplerAlwaysOff enables always-off sampling.
	// No traces will be recorded or exported. Effectively disables tracing.
	OTelTracesSamplerAlwaysOff OTelTracesSamplerType = "always_off"

	// OTelTracesSamplerTraceidratio enables trace ID ratio based sampling.
	// Traces are sampled based on a configured probability derived from their trace ID.
	OTelTracesSamplerTraceidratio OTelTracesSamplerType = "traceidratio"

	// OTelTracesSamplerParentbasedAlwaysOn enables parent-based always-on sampling.
	// If a parent span exists and is sampled, the child span is also sampled.
	// If no parent, it defers to an "always_on" strategy.
	OTelTracesSamplerParentbasedAlwaysOn OTelTracesSamplerType = "parentbased_always_on"

	// OTelTracesSamplerParentbasedAlwaysOff enables parent-based always-off sampling.
	// If a parent span exists and is not sampled, the child span is also not sampled.
	// If no parent, it defers to an "always_off" strategy.
	OTelTracesSamplerParentbasedAlwaysOff OTelTracesSamplerType = "parentbased_always_off"

	// OTelTracesSamplerParentbasedTraceidratio enables parent-based trace ID ratio sampling.
	// If a parent span exists and is sampled, the child span is also sampled.
	// If no parent, it defers to a "traceidratio" strategy.
	OTelTracesSamplerParentbasedTraceidratio OTelTracesSamplerType = "parentbased_traceidratio"
)
```

### Configuration

This features will be enabled via a helm flag, disabled by default.

   ```shell
   helm upgrade -i -n kgateway-system kgateway _test/kgateway-1.0.0-ci1.tgz --version 1.0.0-ci1 \
     --set image.registry=ghcr.io/kgateway-dev \
     --set gateway.aiExtension.enabled=true \
     --set gateway.aiExtension.tracing.enabled=true \
     --create-namespace
   ```

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayParameters
metadata:
  name: ai-gateway
  namespace: kgateway-system
  labels:	
    app: ai-kgateway
spec:
  kube:
    aiExtension:
      enabled: true
      tracing:
      	enabled: true
      	endpoint: http://otel.collector.svc.cluster.local:4317
      	sampler:
          type: traceidratio
          arg: 1.0
      	timeout: 10000
      	protocol: grpc
        transportSecurity: secure
    service:
      type: LoadBalancer
```

### Plugin

There are two kinds of dynamic resources that need to be transformed and appended to the Envoy snapshot. To enable the collection and export of tracing data within the AI Gateway, the Envoy proxy requires specific configurations for OTel integration.

#### HttpConnectionManager Tracing Configuration

The `HttpConnectionManager` needs to be configured with tracing block, specifying OpenTelemetry as the tracing provider.
```yaml
tracing:
  provider:
    name: envoy.tracers.opentelemetry
    typed_config:
      "@type": type.googleapis.com/envoy.config.trace.v3.OpenTelemetryConfig
      grpc_service:
        envoy_grpc:
          cluster_name: otel-collector.monitoring.svc.cluster.local
        timeout: 0.25s
      service_name: "ai-ext-proc"
      resource_detectors:
        - name: envoy.resource_detectors.environment
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.resource_detectors.environment.v3.EnvironmentConfig
      sampler:
        name: envoy.opentelemetry.samplers.AlwaysOnSampler
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.tracers.open_telemetry
```

When a user configures an `AiExtensionTrace` object within the Gateway parameters, the [HTTP Listener Policy Plugin](https://github.com/kgateway-dev/kgateway/pull/11396/files#diff-2d10e902e14b029ddaf0ec2a348ddfc3d510b8e745213b4dd8c8d530be3e7afaR19) will be responsible for parsing these parameters. 
It then dynamically translates them into the necessary **Envoy HttpConnectionManager tracing configuration** and the corresponding **cluster definitions**. 
This fully dynamic process ensures both flexibility and maintainability for your tracing setup.
#### xDS Cluster for OTLP/gRPC Backend

An xDS Cluster definition is required to specify the upstream backend for the OTLP/gRPC compatible tracing storage (e.g., an OpenTelemetry Collector, Jaeger). This cluster will serve as the destination for the tracing data exported by Envoy.

```yaml
- name: otel-collector.monitoring.svc.cluster.local
  type: STRICT_DNS
  load_assignment:
    cluster_name: otel-collector.monitoring.svc.cluster.local
    endpoints:
      - lb_endpoints:
          - endpoint:
              address:
                socket_address:
                  address: otel-collector.monitoring.svc.cluster.local
                  port_value: 4317

```

### ExtProc Server

There are two main tasks to accomplish:

#### Initial TracerProvider

All configuration parameters can be obtained from `/var/run/tracing/tracing.json` and used to initialize the tracer provider.

```python
class Config(BaseModel):
		service_name: string = Field(default="kgateway-ai-extension")
    grpc: Grpc | None = Field(default=None)
    insecure: bool = Field(default=False)

    def tracer(self) -> Tracer:
        # Initialize No-Op tracer provider default.
        tracer_provider: TracerProvider = NoOpTracerProvider()
        
        if self.grpc is not None and len(self.grpc.host) > 0 and self.grpc.port > 0:
            url = f"{self.grpc.host}:{self.grpc.port}"
            logger.debug(f"tracer publishing to: {url}")
            
            resource = Resource.create(attributes={SERVICE_NAME: self.service_name})
            
            # If gRPC is configured, override the NoOpTracerProvider with a real one.
            tracer_provider = TracerProvider(resource=resource)
            
            # Configure span processor and exporter
            span_processor = BatchSpanProcessor(
                OTLPSpanExporter(endpoint=url, insecure=self.insecure)
            )
            tracer_provider.add_span_processor(span_processor)
        else:
            logger.warning("No gRPC configuration found. Tracing will not be enabled.")
        
        # Set the configured tracer_provider (either real or NoOp) globally.
				trace.set_tracer_provider(tracer_provider)
        return trace.get_tracer(__name__)
```

#### Ensure all request paths are covered

Currently, the main flow, key business logic, and exception handling have basic tracing coverage. To ensure that our generated trace data can be effectively parsed and analyzed by standardized tools (like Jaeger, Grafana, etc.) and interoperate with other OpenTelemetry-compliant systems, we must strictly adhere to the [OpenTelemetry Generative AI Span Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-spans/).

##### *Request Headers*

Add tracing for request header handling with model and provider information:

**Tracing purpose**: Trace the request headers processing flow, capturing initial configuration and model information

**Captured attributes**:

- `gen_ai.system`: LLM provider in use (e.g., "openai", "anthropic")
- `gen_ai.request.model`: Requested model name (e.g., "gpt-4o")

Example:

```python
if one_of == "request_headers":
    with OtelTracer.get().start_as_current_span(
        "handle_request_headers",
        context=ctx,
        attributes={
            "gen_ai.system": handler.llm_provider,
            "gen_ai.request.model": handler.request_model,
        }
```
---

##### *Request Body*

Add detailed tracing for the main request body handling path.

**Tracing purpose**: Trace request body processing, including model parameters and content moderation

**Captured attributes**:

- Model information:
  - `gen_ai.operation.name`: Model name
  - `gen_ai.system`: LLM provider
- Request parameters:
  - `gen_ai.output.type`: Output type
  - `gen_ai.request.choice.count`: Number of generation choices
  - `gen_ai.request.model`: Requested model
  - `gen_ai.request.seed`: Random seed
  - `gen_ai.request.temperature`: Temperature parameter
  - `gen_ai.request.max_tokens`: Maximum tokens
  - `gen_ai.request.stop_sequences`: Stop sequences
- Token information:
  - `ai.tokens.prompt`: Request token count
- Content moderation:
  - `ai.moderation.flagged`: Whether content was flagged
  - `ai.moderation.categories`: Categories that were flagged

Example:
```python
async def handle_request_body(
    self,
    req_body: external_processor_pb2.HttpBody,
    metadict: dict,
    handler: StreamHandler,
    parent_span: trace.Span,
) -> external_processor_pb2.ProcessingResponse:
...
if req_body.end_of_stream:
    with OtelTracer.get().start_as_current_span(
        handler.llm_provider,
        context=trace.set_span_in_context(parent_span),
        attributes={
            "gen_ai.operation.name" : body.get("model", ""),
            "gen_ai.system": handler.llm_provider,
            "gen_ai.output.type": body.get("response_format", {}).get("type", ""),
            "gen_ai.request.choice.count": body.get("n", 0),
            "gen_ai.request.model": handler.request_model,
            "gen_ai.request.seed": body.get("seed", 0),
            "gen_ai.request.frequency_penalty": body.get("frequency_penalty", 0),
            "gen_ai.request.max_tokens": body.get("max_tokens", 0),
            "gen_ai.request.presence_penalty": body.get("presence_penalty", 0),
            "gen_ai.request.stop_sequences": body.get("stop", []),
            "gen_ai.request.temperature": body.get("temperature", 0),
            "gen_ai.request.top_k": body.get("top_k", 0),
            "gen_ai.request.top_p": body.get("top_p", 0),
        }
    ):
        tokens = handler.provider.get_num_tokens_from_body(body)
        parent_span.set_attribute("ai.tokens.prompt", tokens)
    ...
    if handler.req_moderation:
        with OtelTracer.get().start_as_current_span(
            "moderation",
            context=trace.set_span_in_context(parent_span),
        ) as moderation_span:
            moderation_span.set_attribute(
                "gen_ai.operation.provider", handler.req_moderation[0].provider_name
            )
```
---

##### *Webhook*
Add tracing for webhook request handling:

**Tracing purpose**: Trace webhook request processing flow, record latency and results

**Captured attributes**:

- `ai.webhook.host`: Webhook host
- `ai.webhook.forward_headers`: Forwarded headers
- `ai.webhook.latency_ms`: Webhook request latency
- `ai.webhook.result`: Result ("modified" or "rejected")
- `ai.webhook.reject_reason`: Rejection reason (if applicable)

```python
async def handle_request_body_req_webhook(
    self,
    body: dict,
    handler: StreamHandler,
    webhook_cfg: prompt_guard.Webhook,
    parent_span: trace.Span,
) -> external_processor_pb2.ProcessingResponse | None:
    with OtelTracer.get().start_as_current_span(
        "webhook",
        context=trace.set_span_in_context(parent_span),
    ) as webhook_span:
        webhook_span.set_attributes(
            {
                "ai.webhook.host": webhook_cfg.host,
                "ai.webhook.forward_headers": webhook_cfg.forwardHeaders,
            }
        )

        webhook_start_time = time.time()
        try:
            headers = deepcopy(handler.req.headers)
            TraceContextTextMapPropagator().inject(headers)
            response: (
                PromptMessages | RejectAction | None
            ) = await make_request_webhook_request(
                webhook_host=webhook_cfg.host,
                webhook_port=webhook_cfg.port,
                headers=headers,
                promptMessages=handler.provider.construct_request_webhook_request_body(
                    body
                ),
            )
            webhook_span.set_attribute(
                "ai.webhook.latency_ms", (time.time() - webhook_start_time) * 1000
            )

            if isinstance(response, PromptMessages):
                handler.provider.update_request_body_from_webhook(body, response)
                webhook_span.set_attribute("ai.webhook.result", "modified")
                webhook_span.add_event("ai.webhook.prompt.modified")

            if isinstance(response, RejectAction):
                webhook_span.set_attributes(
                    {
                        "ai.webhook.result": "rejected",
                        "ai.webhook.reject_reason": response.reason,
                    }
                )
```

--- 

##### *Regex processing*
Add tracing for regex processing:

**Tracing purpose**: Trace regex expression processing, record rule count and execution time

**Captured attributes**:

- `ai.regex.rules_count`: Number of rules
- `ai.regex.latency_ms`: Processing latency
- `ai.regex.result`: Processing result ("passed" or error)

Example:

```python
def handle_request_body_req_regex(
    self, body: dict, handler: StreamHandler, parent_span: trace.Span
) -> external_processor_pb2.ProcessingResponse | None:
    with OtelTracer.get().start_as_current_span(
        "regex",
        context=trace.set_span_in_context(parent_span),
    ) as regex_span:
        regex_span.set_attribute("ai.regex.rules_count", len(handler.req_regex))

        regex_start_time = time.time()
        # If this raises an exception it means that the action was reject, not mask
        try:
            handler.provider.iterate_str_req_messages(
                body=body, cb=handler.req_regex_transform
            )
            regex_span.set_attributes(
                {
                    "ai.regex.latency_ms": (time.time() - regex_start_time) * 1000,
                    "ai.regex.result": "passed",
                }
            )
```

---
##### *Response Body*

**Tracing purpose**: Trace response body processing, record model output information

- **Captured attributes**:
  - `gen_ai.operation.name`: Operation name
  - `gen_ai.system`: LLM provider
  - `gen_ai.response.id`: Response ID
  - `gen_ai.response.model`: Response model
  - `gen_ai.response.finish_reasons`: Completion reason
  - `gen_ai.usage.output_tokens`: Output token count

Example:

```python
async def handle_response_body(
    self,
    resp_body: external_processor_pb2.HttpBody,
    handler: StreamHandler,
    parent_span: trace.Span,
) -> external_processor_pb2.ProcessingResponse:
  finish_reason = ""

  if isinstance(jsn.get("choices"), list) and len(jsn["choices"]) > 0:
      first_choice = jsn["choices"][0]
      if isinstance(first_choice, dict):
          finish_reason = first_choice.get("finish_reason", "")

  completion_tokens = 0
  if isinstance(jsn.get("usage"), dict):
      completion_tokens = jsn["usage"].get("completion_tokens", 0)

  with OtelTracer.get().start_as_current_span(
  jsn.get("model", ""),
  context=trace.set_span_in_context(parent_span),
  attributes={
      "gen_ai.operation.name": jsn.get("model", ""),
      "gen_ai.system": handler.llm_provider,
      "gen_ai.response.id": jsn.get("id", ""),
      "gen_ai.response.model": jsn.get("model", ""),
      "gen_ai.response.finish_reasons": finish_reason, 
      "gen_ai.usage.output_tokens": completion_tokens,
  }
):
      pass
```

---

##### *Exception Handling*

Enhanced error tracing by recording exceptions properly:

**Tracing purpose**: Ensure all exceptions are properly recorded for debugging

- **Recording method**:
  - `span.record_exception(exc)`: Record exception details
  - `span.set_status(trace.StatusCode.ERROR, str(exc))`: Set error status

- **Covered exceptions**:
  - JSONDecodeError: JSON parsing errors
  - RegexRejection: Regex expression rejection
  - WebhookException: Webhook handling errors
  - General exceptions

Example:
```python
except json.decoder.JSONDecodeError as exc:
    span.record_exception(exc)
    span.set_status(
        trace.StatusCode.ERROR,
        str(exc),
    )
```

```python
except Exception as e:
    span.record_exception(e)
    span.set_status(
        trace.StatusCode.ERROR,
        str(e),
    )
    return error_response(
        handler.req_custom_response, "Error with guardrails webhook", e
    )
```

```python
except RegexRejection as e:
    span.record_exception(e)
    span.set_status(
        trace.StatusCode.ERROR,
        str(e),
    )
    return error_response(
        handler.req_custom_response, "Rejected by guardrails regex", e
    )
```

---

##### OpenAI Request Example and Trace Output Validation:

The following `curl` request can be used to test the above logic:

```bash
curl "localhost:8080/openrouter" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"openai/gpt-4o\",
    \"messages\": [
      {
        \"role\": \"user\",
        \"content\": \"What is the meaning of life?\"
      }
    ],
    \"temperature\": 0.7,
    \"max_tokens\": 150,
    \"n\": 2,
    \"seed\": 123
  }" | jq
```

##### Output Example: OpenAI Request Body Span

```json
{
    "name": "openai/gpt-4o",
    "context": {
        "trace_id": "0x4e4798e9287501bceb734f74b9d25ac9",
        "span_id": "0x3ac5b25e33326e72",
        "trace_state": "[]"
    },
    "kind": "SpanKind.INTERNAL",
    "parent_id": "0xf6a5fac786151c7b",
    "start_time": "2025-06-16T14:22:17.243004Z",
    "end_time": "2025-06-16T14:22:17.331470Z",
    "status": {
		"status_code": "UNSET"
    },
		"attributes": {
        "gen_ai.operation.name": "openai/gpt-4o",
        "gen_ai.system": "openai",
        "gen_ai.output.type": "",
        "gen_ai.request.choice.count": 2,
        "gen_ai.request.model": "openai/gpt-4o",
        "gen_ai.request.seed": 123,
        "gen_ai.request.frequency_penalty": 0,
        "gen_ai.request.max_tokens": 150,
        "gen_ai.request.presence_penalty": 0,
        "gen_ai.request.stop_sequences": [],
        "gen_ai.request.temperature": 0.7,
        "gen_ai.request.top_k": 0,
        "gen_ai.request.top_p": 0
    },
    "events": [],
		"links": [],
    "resource": {
        "attributes": {
            "telemetry.sdk.language": "python",
            "telemetry.sdk.name": "opentelemetry",
            "telemetry.sdk.version": "1.34.1",
            "service.name": "kgateway-ai-extension"
        },
        "schema_url": ""
    }
}
```

##### Output Example: OpenAI Response Body Span

```json
{
    "name": "openai",
    "context": {
        "trace_id": "0xfc1a7624d126b688b463c54378670dea",
        "span_id": "0x2cec89783790f34f",
        "trace_state": "[]"
    },
    "kind": "SpanKind.INTERNAL",
    "parent_id": "0x9c4b36bf0bb4a5ba",
    "start_time": "2025-06-16T14:22:20.205595Z",
    "end_time": "2025-06-16T14:22:20.205613Z",
    "status": {
        "status_code": "UNSET"
    },
    "attributes": {
        "gen_ai.operation.name": "openai/gpt-4o",
        "gen_ai.system": "openai",
        "gen_ai.response.id": "gen-1750083737-01qrIBNrwHLQg2QawfHa",
        "gen_ai.response.model": "openai/gpt-4o",
        "gen_ai.response.finish_reasons": "stop",
        "gen_ai.usage.output_tokens": 133
    },
    "events": [],
    "links": [],
    "resource": {
        "attributes": {
            "telemetry.sdk.language": "python",
            "telemetry.sdk.name": "opentelemetry",
            "telemetry.sdk.version": "1.34.1",
            "service.name": "kgateway-ai-extension"
        },
        "schema_url": ""
    }
}
```

### Test Plan

#### E2E Test

E2E Tests to validate new API changes and configuration, ensuring traces are produced and exported to the correct backend.

------

##### Deploying Tempo (Monolithic Mode)

We enable the trace search feature via `--set tempo.searchEnabled=true`, which is crucial for retrieving trace data through the query API in E2E tests. Additionally, `--set tempo.target=all` ensures Tempo is deployed in **Monolithic mode**. This mode integrates all components (such as data ingestion, storage, and querying) into a single Pod and defaults to using local storage (boltdb-shipper), making it highly suitable for quick setup and E2E testing environments. The command should be executed using the [Helm client](https://github.com/kgateway-dev/kgateway/blob/47e94dba408fdb26e9f2e0d927237896bc621bef/pkg/utils/helmutils/client.go#L57-L60).

```bash
helm install tempo grafana/tempo \
  --namespace ai-test \
  --set tempo.searchEnabled=true \
  --set tempo.target=all
```

##### Use TraceQL to search for traces

All traces are stored in Tempo. We can access the Tempo endpoint and use **TraceQL** to query trace data based on specific conditions. By comparing these results with **mock traces**, we can assert the completeness of the entire tracing pipeline, from trace generation to storage. Our **validation baseline** comes from requests sent to the AI Gateway and data provided by the [mock-ai-provider-server](https://github.com/kgateway-dev/kgateway/blob/47e94dba408fdb26e9f2e0d927237896bc621bef/test/mocks/mock-ai-provider-server/mocks/routing/openai_non_streaming.json#L4).

###### Example: Traces for requests sent to the AI Gateway

This example shows how traces generated by requests to the AI Gateway are stored in Tempo.

```bash
curl -G -s "http://localhost:3200/api/search" \
     --data-urlencode 'q={name="openai"}' \
     --data-urlencode "start=${START_TIME}" \
     --data-urlencode "end=${END_TIME}" \
     --data-urlencode "limit=100" | jq
{
  "traces": [
    {
      "traceID": "45469d52226f30d7076570593c1ed6eb",
      "rootServiceName": "kgateway-ai-extension",
      "rootTraceName": "handle_request_body",
      "startTimeUnixNano": "1750183404673185818",
      "durationMs": 67,
      "spanSet": {
        "spans": [
          {
            "spanID": "3fcf91d4fe019790",
            "name": "openai",
            "startTimeUnixNano": "1750183404673299778",
            "durationNanos": "67613092"
          }
        ],
        "matched": 1
      },
      "spanSets": [
        {
          "spans": [
            {
              "spanID": "3fcf91d4fe019790",
              "name": "openai",
              "startTimeUnixNano": "1750183404673299778",
              "durationNanos": "67613092"
            }
          ],
          "matched": 1
        }
      ],
      "serviceStats": {
        "kgateway-ai-extension": {
          "spanCount": 2
        }
      }
    }
  ],
  "metrics": {
    "inspectedBytes": "53751",
    "completedJobs": 1,
    "totalJobs": 1
  }
}
```

###### Example: Traces for interactions with the Mock AI Provider Server

This example shows how traces related to the [mock-ai-provider-server](https://github.com/kgateway-dev/kgateway/blob/47e94dba408fdb26e9f2e0d927237896bc621bef/test/mocks/mock-ai-provider-server/mocks/routing/openai_non_streaming.json#L4) are stored in Tempo.

```bash
curl -G -s "http://localhost:3200/api/search" \
     --data-urlencode 'q={name="openai/gpt-4o"}' \
     --data-urlencode "start=${START_TIME}" \
     --data-urlencode "end=${END_TIME}" \
     --data-urlencode "limit=100" | jq
{
  "traces": [
    {
      "traceID": "2f4840e4691d88e52f9597cc2be89976",
      "rootServiceName": "kgateway-ai-extension",
      "rootTraceName": "handle_response_body",
      "startTimeUnixNano": "1750408230186355047",
      "durationMs": 1,
      "spanSet": {
        "spans": [
          {
            "spanID": "89594cca2f72a814",
            "name": "openai/gpt-4o",
            "startTimeUnixNano": "1750408230186455880",
            "durationNanos": "6875"
          }
        ],
        "matched": 1
      },
      "spanSets": [
        {
          "spans": [
            {
              "spanID": "89594cca2f72a814",
              "name": "openai/gpt-4o",
              "startTimeUnixNano": "1750408230186455880",
              "durationNanos": "6875"
            }
          ],
          "matched": 1
        }
      ],
      "serviceStats": {
        "kgateway-ai-extension": {
          "spanCount": 2
        }
      }
    },
    {
      "traceID": "5e2f810464667814309b1d82ea48f4dd",
      "rootServiceName": "kgateway-ai-extension",
      "rootTraceName": "handle_response_body",
      "startTimeUnixNano": "1750408020256219799",
      "durationMs": 2,
      "spanSet": {
        "spans": [
          {
            "spanID": "bf3f77aec7d8390b",
            "name": "openai/gpt-4o",
            "startTimeUnixNano": "1750408020256385298",
            "durationNanos": "9291"
          }
        ],
        "matched": 1
      },
      "spanSets": [
        {
          "spans": [
            {
              "spanID": "bf3f77aec7d8390b",
              "name": "openai/gpt-4o",
              "startTimeUnixNano": "1750408020256385298",
              "durationNanos": "9291"
            }
          ],
          "matched": 1
        }
      ],
      "serviceStats": {
        "kgateway-ai-extension": {
          "spanCount": 2
        }
      }
    }
  ],
  "metrics": {
    "inspectedBytes": "55490",
    "completedJobs": 1,
    "totalJobs": 1
  }
}
```

#### Unit Tests 

Unit Tests to verify that spans are generated for all critical paths.

## Alternatives

## Open Questions
