# EP-11818: kgateway + agentgateway tracing integration

* Issue: [#11818](https://github.com/kgateway-dev/kgateway/issues/11818)

## Background

Currently, agentgateway supports OpenTelemetry tracing, but it is configured statically via a ConfigMap at startup. This configuration is applied globally, meaning tracing is either enabled for all traffic with a single configuration or disabled entirely. This approach lacks the flexibility to apply different tracing strategies to different listeners or to enable/disable tracing at runtime without a pod restart.

The kgateway control plane already supports dynamic policy attachment for other features, and the HTTPListenerPolicy CRD provides a user-facing API for configuring listener-level behavior, including tracing for the Envoy data plane.

## Motivation

The goal of this proposal is to refactor the existing tracing implementation in agentgateway from a static, startup-time configuration to a dynamic, policy-driven model. This will allow operators to enable, disable, and configure tracing behavior on a per-listener basis at runtime, using policies applied in Kubernetes.

## Goals

* Refactor the agentgateway data plane to support multiple, dynamically configured OpenTelemetry tracer instances.
* Introduce a mechanism in the kgateway control plane to translate a user-facing policy into an agentgateway-specific xDS resource for tracing.
* Enable per-listener tracing configurations, including service name, sampling, OTLP endpoint, and trace attributes.
* Ensure the solution is robust, performant, and aligns with the existing policy and xDS architecture of both kgateway and agentgateway.
* Provide a clear test plan for validating the dynamic configuration updates and trace data generation.

## Non-Goals

* Implementing tracing on a per-route or per-request basis. The initial scope is listener-level granularity.
* Adding support for tracing providers other than OpenTelemetry.

## Implementation Details

### Sample YAML API

Using the new `AgentgatewayPolicy` CRD introduced in kgateway PR #12723, tracing configuration is part of the `frontend` section, which applies to Gateway-level settings.

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: AgentgatewayPolicy
metadata:
  name: gateway-tracing-policy
  namespace: default
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-gateway
  frontend:
    tracing:
      serviceName: "my-gateway-service"
      backendRef:
        group: ""
        kind: Service
        name: otel-collector
        namespace: observability
        port: 4317
      protocol: GRPC
      attributes:
        add:
          http.request.id: 'request.headers["x-request-id"] | "unknown"'
          http.method: "request.method"
          http.host: "request.host"
          gateway.listener: "listener.name"
```

### API Structure

The `AgentgatewayPolicy` CRD provides three main sections:
- `backend`: Settings for connecting to destination backends
- `frontend`: Settings for handling incoming traffic (including tracing)
- `traffic`: Settings for processing traffic

Tracing configuration lives in the `frontend` section since it applies to how the gateway handles incoming requests and generates observability data.

**Key API Fields:**

```go
type AgentTracing struct {
    // ServiceName sets the service name reported in traces
    ServiceName string `json:"serviceName"`
    
    // BackendRef references the OTLP server (Service or Backend)
    BackendRef gwv1.BackendObjectReference `json:"backendRef"`
    
    // Protocol specifies OTLP protocol variant (HTTP or GRPC)
    Protocol TracingProtocol `json:"protocol"`
    
    // Attributes specifies custom trace attributes as CEL expressions
    Attributes *AgentLogTracingFields `json:"attributes,omitempty"`
    
    // RandomSampling - CEL expression for random sampling (0.0-1.0 or bool)
    RandomSampling *CELExpression `json:"randomSampling,omitempty"`
    
    // ClientSampling - CEL expression for client sampling (0.0-1.0 or bool)
    ClientSampling *CELExpression `json:"clientSampling,omitempty"`
}

type AgentLogTracingFields struct {
    // Remove lists default fields to exclude from traces
    Remove []TinyString `json:"remove,omitempty"`
    
    // Add specifies additional key-value pairs (CEL expressions)
    Add map[string]CELExpression `json:"add,omitempty"`
}
```

### Control Plane Implementation (kgateway)

The control plane translation happens in `kgateway/pkg/agentgateway/plugins/frontend_policies.go`, specifically in the `translateFrontendTracing` function.

**Translation Flow:**
1. User creates `AgentgatewayPolicy` with `spec.frontend.tracing` field
2. kgateway AgentgatewayPolicy controller detects the policy
3. `translateFrontendTracing()` converts the Go API types to protobuf
4. Protobuf `Policy` message is sent via xDS to agentgateway
5. agentgateway receives and instantiates the `Tracer`


**Target Resolution:**
The policy target is determined by the `targetRefs` field:
```go
// Gateway-level tracing
targetRefs:
- kind: Gateway
  name: my-gateway
  # Applies to all listeners on this gateway

// Listener-level tracing (using sectionName)
targetRefs:
- kind: Gateway
  name: my-gateway
  sectionName: https-listener
  # Applies only to specific listener
```

This translates to:
- Gateway target → `PolicyTarget::Gateway` in agentgateway
- Gateway with sectionName → `PolicyTarget::Listener` in agentgateway

### Data Plane Implementation (agentgateway)

The implementation can be broken down into four key stages:
1.  Defining the `Tracing` policy structure.
2.  Dynamically creating `Tracer` instances when configuration is received.
3.  Using the dynamic `Tracer` during request processing.
4.  Enhancing the OTLP exporter to use `agentgateway`'s own networking stack.

#### 1. Protobuf and Core Type Definition (`resource.proto` & `types/agent.rs`)

In `crates/agentgateway/proto/resource.proto`, we will add a `Tracing` message to the `PolicySpec` `oneof`. This makes it a first-class policy type that the control plane can send and the data plane can understand.

   ```protobuf
   message PolicySpec {
     oneof kind {
       // ... existing policies
       Tracing tracing = 14;
     }
   }

   message Tracing {
     // The service name to be reported to the tracing backend.
     string service_name = 1;
     // The backend where the OTel collector is running.
     // This is resolved by kgateway into a concrete service reference.
     BackendReference provider_backend = 2;
     // A list of custom attributes to add to spans.
     repeated TracingAttribute attributes = 3;
   }

   message TracingAttribute {
       string name = 1;
       // The value is a CEL expression that will be evaluated at request time.
       string value = 2;
   }
   ```

**Note:** This protobuf represents the **agentgateway-specific** tracing configuration that gets sent via xDS after the control plane processes the `HTTPListenerPolicy.spec.tracing.agentgateway` field.

This protobuf definition will have corresponding Rust structs automatically generated, which we will use in the next steps.

#### 2. Dynamic Tracer Instantiation (The `xDS` Ingestion Path)

This is the core change based on reviewer feedback, moving away from a separate cache. We will modify the `xDS` ingestion path in `crates/agentgateway/src/types/agent_xds.rs`.

When a `Policy` message with the `tracing` kind is received, we will not just copy the data; we will **instantiate the OpenTelemetry `Tracer` right there**.

**Conceptual Code:**

```rust
// In crates/agentgateway/src/types/agent.rs
// We'll define a new struct to hold the COMPILED tracer.
#[derive(Clone, Debug, PartialEq)]
pub struct TracingPolicy {
    pub config: TracingConfig, // The deserialized config
    pub tracer: Arc<opentelemetry_sdk::trace::Tracer>, // The live, compiled tracer
}

// In crates/agentgateway/src/types/agent_xds.rs
// Inside the TryFrom implementation for Policy...
...
    Some(proto::agent::policy_spec::Kind::Tracing(t)) => {
        // 1. Convert the protobuf `t` into our internal `TracingConfig` struct.
        let tracing_config = TracingConfig::try_from(t)?;

        // 2. Call a function (refactored from the old static telemetry/trc.rs)
        //    to create a live tracer instance from this specific config.
        //    This function sets up the OTLP exporter and pipeline.
        let tracer_instance = create_tracer_from_config(&tracing_config);

        // 3. Store both the config and the live tracer in our new policy struct.
        let policy_spec = PolicySpec::Tracing(TracingPolicy {
            config: tracing_config,
            tracer: Arc::new(tracer_instance),
        });
        Ok((policy_spec, target))
    }
...
```

This approach reuses the existing policy store, creates the tracer only when the configuration changes, and efficiently shares the compiled tracer across concurrent requests using an `Arc`.

#### 3. Proxy Integration (The Request Path)

This addresses the feedback on how to use the new dynamic tracer, correcting the initial `tower` middleware assumption.

**Location:** `crates/agentgateway/src/proxy/httpproxy.rs`

We will replace the static tracer assignment with a dynamic lookup from the policy store.

**Conceptual Code:**

```rust
// In crates/agentgateway/src/proxy/httpproxy.rs, inside the main request handler...

// After the listener for the request has been determined...
let listener = selected_listener; // The listener for the current request.

// Look up the tracing policy for this listener or its gateway.
let tracing_policy = inputs.stores.read_policies()
    .get_policy_for_target::<TracingPolicy>(&PolicyTarget::Listener(listener.key.clone()))
    .or_else(|| inputs.stores.read_policies()
        .get_policy_for_target::<TracingPolicy>(&PolicyTarget::Gateway(listener.gateway_name.clone())));

// Get the tracer. If a policy exists, use its pre-compiled tracer.
// If not, use a no-op tracer that does nothing.
let tracer = if let Some(policy) = tracing_policy {
    // Clone the Arc, which is a cheap reference count bump.
    policy.tracer.clone()
} else {
    // Fallback to a tracer that performs no operations.
    opentelemetry::global::noop_tracer_provider().tracer("noop")
};

// The rest of the function now uses this `tracer` variable to start spans.
let span = tracer.start("request");
// ...
```

This makes tracing entirely controlled by the presence of a policy. The request handling logic remains clean, as it interacts with a standard `Tracer` interface, whether it's a real one or a no-op.

#### 4. Exporter Enhancement (Using `PolicyClient`)

To allow `agentgateway`'s own networking policies to apply to telemetry traffic, we will enhance the OTLP exporter.

The default OTLP exporter opens a direct TCP connection to the collector, bypassing `agentgateway`'s control. To fix this, we will implement a custom `tonic` `Channel` that funnels its requests through `agentgateway`'s internal `PolicyClient`.

**Conceptual Steps:**
1.  **Create a `PolicyClient` Service:** Implement a struct that conforms to `tower::Service<http::Request>`.
2.  The `call` method of this service will pass the HTTP/2 request intended for the OTel collector to the `PolicyClient`, which handles routing, load balancing, and policy enforcement (like TLS).
3.  **Build a Custom Channel:** Use this service to create a `tonic::transport::Channel`.
4.  **Inject into OTLP Exporter:** When creating the tracer in `create_tracer_from_config`, configure the OTLP exporter to use this custom channel.

This approach unifies policy enforcement, enabling features like `BackendTLSPolicy` to secure the connection to the OTel collector and making the entire system more consistent and robust.

## Alternatives

*   **HTTPListenerPolicy with agentgateway field**: Initially proposed to extend HTTPListenerPolicy with a oneof field for agentgateway vs envoy tracing. This was the approach before AgentgatewayPolicy was introduced. Rejected in favor of using the new dedicated AgentgatewayPolicy CRD which provides cleaner separation and follows the backend/frontend/traffic model.

*   **Separate AgentGatewayObservabilityPolicy CRD**: Considered creating a dedicated CRD just for observability (tracing, metrics, logging). Rejected because the new AgentgatewayPolicy already provides the right structure via its `frontend` section, avoiding unnecessary CRD proliferation.

*   **Direct reuse of existing HTTPListenerPolicy tracing field**: This was rejected because the existing Envoy-specific fields (integer-based sampling, Envoy gRPC configuration) are not compatible with agentgateway's CEL-based approach and PolicyClient integration.
