# EP-11188: Fault Injection Support

- Issue: [#11188](https://github.com/kgateway-dev/kgateway/issues/11188)

## Background

Fault injection allows users to simulate failure conditions (latency, HTTP/gRPC errors, slow connections) for chaos engineering and resiliency testing. In production environments, services inevitably encounter degraded dependencies, network delays, and upstream failures. Without a way to proactively test against these conditions, teams only discover weaknesses during real incidents.

Adding fault injection to kgateway enables users to validate retry policies, timeout configurations, and fallback behavior directly at the gateway layer before failures happen in production. By providing this as a native TrafficPolicy feature, users can target specific routes or gateways with controlled faults without modifying application code or deploying separate testing infrastructure.

## Motivation

### Goals

- Add fault injection support to `TrafficPolicy` for delay injection, abort injection, and response rate limiting
- Support both HTTP and gRPC abort status codes
- Support percentage based fault injection to control the fraction of affected requests
- Support `maxActiveFaults` to limit concurrent active faults
- Support disabling fault injection to override policies inherited from higher levels in the config hierarchy
- Leverage Envoy `envoy.filters.http.fault` filter for the underlying implementation

### Non-Goals

- Exposing all Envoy fault filter fields (runtime key overrides, header matchers, downstream node filtering, upstream cluster filtering)
- Header controlled dynamic fault values (e.g., `x-envoy-fault-delay-request`) in the initial implementation
- Creating a standalone FaultInjectionPolicy CRD

## Implementation Details

### Design: Extend TrafficPolicy CRD

Rather than creating a new standalone CRD, this design extends the existing TrafficPolicy CRD with a `FaultInjection` field. This is consistent with how other traffic manipulation features (CORS, rate limiting, retries, timeouts) are already implemented in kgateway.

Rationale:

- TrafficPolicy already handles per-route and per-gateway traffic manipulation
- Reuses the existing policy attachment mechanism (`targetRefs`/`targetSelectors`)
- Reuses existing merge, status reporting, and validation implementations
- Users already manage traffic behavior through TrafficPolicy

### API Changes

The `TrafficPolicySpec` struct in `api/v1alpha1/kgateway/traffic_policy_types.go` is extended:

```go
type TrafficPolicySpec struct {
    // ... existing fields ...

    // FaultInjection configures fault injection for chaos engineering and
    // resiliency testing. Supports delay injection, abort injection,
    // and response rate limiting.
    // +optional
    FaultInjection *FaultInjectionPolicy `json:"faultInjection,omitempty"`
}
```

The following types are added:

```go
// FaultInjectionPolicy configures fault injection for testing service resiliency.
// At least one of delay, abort, responseRateLimit, or disable must be specified.
//
// +kubebuilder:validation:AtLeastOneOf=delay;abort;responseRateLimit;disable
type FaultInjectionPolicy struct {
    // Delay injects latency into requests before forwarding upstream.
    // +optional
    Delay *FaultDelay `json:"delay,omitempty"`

    // Abort injects HTTP or gRPC errors to terminate requests early.
    // +optional
    Abort *FaultAbort `json:"abort,omitempty"`

    // ResponseRateLimit limits the response body data rate to simulate
    // slow or degraded upstream connections.
    // +optional
    ResponseRateLimit *FaultResponseRateLimit `json:"responseRateLimit,omitempty"`

    // MaxActiveFaults limits the number of concurrent active faults.
    // When this limit is reached, new requests will not have faults injected.
    // If not specified or set to 0, the number of active faults is unlimited.
    // To effectively disable fault injection, use the Disable field instead.
    // +optional
    // +kubebuilder:validation:Minimum=0
    MaxActiveFaults *uint32 `json:"maxActiveFaults,omitempty"`

    // Disable the fault injection filter.
    // Can be used to disable fault injection policies applied at a higher level
    // in the config hierarchy.
    // +optional
    Disable *shared.PolicyDisable `json:"disable,omitempty"`
}

// FaultDelay configures latency injection.
type FaultDelay struct {
    // FixedDelay is the duration to delay before forwarding the request.
    // +required
    // +kubebuilder:validation:XValidation:rule="matches(self, '^([0-9]{1,5}(h|m|s|ms)){1,4}$')",message="invalid duration value"
    // +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1ms')",message="must be at least 1ms"
    // +kubebuilder:validation:XValidation:rule="duration(self) <= duration('1h')",message="must not exceed 1h"
    FixedDelay metav1.Duration `json:"fixedDelay"`

    // Percentage of requests to inject the delay on. Defaults to 100.
    // +optional
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=100
    // +kubebuilder:default=100
    Percentage *int32 `json:"percentage,omitempty"`
}

// FaultAbort configures request abort injection.
//
// +kubebuilder:validation:ExactlyOneOf=httpStatus;grpcStatus
type FaultAbort struct {
    // HttpStatus is the HTTP status code to return when aborting a request.
    // +optional
    // +kubebuilder:validation:Minimum=200
    // +kubebuilder:validation:Maximum=599
    HttpStatus *int32 `json:"httpStatus,omitempty"`

    // GrpcStatus is the gRPC status code to return when aborting a request.
    // +optional
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=16
    GrpcStatus *int32 `json:"grpcStatus,omitempty"`

    // Percentage of requests to abort. Defaults to 100.
    // +optional
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=100
    // +kubebuilder:default=100
    Percentage *int32 `json:"percentage,omitempty"`
}

// FaultResponseRateLimit configures response body rate limiting to simulate
// slow upstream connections.
type FaultResponseRateLimit struct {
    // KbitsPerSecond limits the response rate to the specified kilobits per second.
    // +required
    // +kubebuilder:validation:Minimum=1
    KbitsPerSecond uint64 `json:"kbitsPerSecond"`

    // Percentage of requests to apply the rate limit on. Defaults to 100.
    // +optional
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=100
    // +kubebuilder:default=100
    Percentage *int32 `json:"percentage,omitempty"`
}
```

### Envoy Configuration

The `envoy.filters.http.fault` HTTP filter is added to the Envoy listener's HTTP connection manager `http_filters` chain only when at least one route or virtual host on that listener has fault injection configured. When present, the listener-level filter is configured with an empty `HTTPFault` proto and is globally disabled, so it is a no-op by default.

Actual fault behavior is configured per-route or per-virtual-host using `typed_per_filter_config`, which enables the filter and supplies delay, abort, or rate limit settings for individual routes that opt in to fault injection. When the TrafficPolicy targets a Gateway, the fault configuration is applied at the virtual host level. When it targets an HTTPRoute, it is applied at the route level. Routes and virtual hosts without fault injection configured are unaffected.

**HTTP filter at the listener level (conditionally inserted, globally disabled):**

```json
{
  "name": "envoy.filters.http.fault",
  "typed_config": {
    "@type": "type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault"
  }
}
```

**Per-route `typed_per_filter_config` (enables and configures faults for a specific route):**

```json
"routes": [
    {
        "match": { "prefix": "/fault" },
        "route": { "cluster": "backend_cluster" },
        "typed_per_filter_config": {
            "envoy.filters.http.fault": {
                "@type": "type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault",
                "delay": {
                    "fixed_delay": "3s",
                    "percentage": { "numerator": 50 }
                },
                "abort": {
                    "http_status": 503,
                    "percentage": { "numerator": 25 }
                },
                "max_active_faults": 100
            }
        }
    },
    {
        "match": { "prefix": "/" },
        "route": { "cluster": "backend_cluster" }
    }
]
```

### Plugin

The implementation is added to the traffic policy plugin in `pkg/kgateway/extensions2/plugins/trafficpolicy/`:

- The IR struct is extended with fault injection fields, translated as close to Envoy protos as possible
- `NewGatewayTranslationPass` applies the fault filter to the listener and per-route configuration during translation
- No new plugin is needed; the existing trafficpolicy plugin handles the new field

### Controllers

No new controllers are required. Existing TrafficPolicy controllers handle the new field automatically through the standard reconciliation process.

### Translator and Proxy Syncer

The translator needs updates to:

1. **Filter chain construction:** Conditionally add the `envoy.filters.http.fault` filter (globally disabled) to the HTTP filter chain when any route or virtual host on the listener has fault injection configured
2. **Per-route/per-vhost configuration:** Apply fault injection configuration via `typed_per_filter_config` at the route level (HTTPRoute target) or virtual host level (Gateway target)
3. **Policy merge:** Support merging fault injection policies across different attachment levels (Gateway, Listener, Route)

### Example Usage

**Inject 500ms delay on 50% of requests:**

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: delay-injection
  namespace: default
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: my-app
  faultInjection:
    delay:
      fixedDelay: 500ms
      percentage: 50
```

**Abort 10% of requests with HTTP 503:**

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: abort-injection
  namespace: default
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: my-app
  faultInjection:
    abort:
      httpStatus: 503
      percentage: 10
```

**Combined delay and abort with max active faults:**

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: combined-faults
  namespace: default
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: my-app
  faultInjection:
    delay:
      fixedDelay: 200ms
      percentage: 25
    abort:
      httpStatus: 500
      percentage: 5
    maxActiveFaults: 100
```

**Abort gRPC requests with UNAVAILABLE status:**

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: grpc-abort
  namespace: default
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: my-grpc-service
  faultInjection:
    abort:
      grpcStatus: 14
      percentage: 10
```

**Simulate slow upstream with response rate limiting:**

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: slow-response
  namespace: default
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: my-app
  faultInjection:
    responseRateLimit:
      kbitsPerSecond: 64
      percentage: 100
```

### Conditional Fault Injection via Gateway API

The Envoy fault filter's `headers` field allows activating faults only when specific request headers are present. This use case is already achievable without exposing the fault filter `headers` field, by using Gateway API HTTPRoute header matching combined with TrafficPolicy attachment:

```yaml
# HTTPRoute that only matches requests with the x-fault-test header
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: fault-test-route
  namespace: default
spec:
  parentRefs:
    - name: my-gateway
  rules:
    - matches:
        - headers:
            - name: x-fault-test
              value: "true"
      backendRefs:
        - name: my-app
          port: 8080
---
# Attach fault injection only to the header-matched route
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: fault-on-header
  namespace: default
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: fault-test-route
  faultInjection:
    abort:
      httpStatus: 503
      percentage: 100
```

### Test Plan

#### Unit Tests

- Test IR translation from CRD types to Envoy fault filter protos
- Test percentage conversion to Envoy's `FractionalPercent` format
- Test validation logic for fault injection fields (at-least-one-of, exactly-one-of)

#### Translator Tests

- YAML golden tests in `pkg/kgateway/translator/gateway/gateway_translator_test.go` covering:
  - Delay-only fault injection
  - Abort-only fault injection (HTTP and gRPC status codes)
  - Response rate limit only
  - Combined delay + abort
  - Max active faults configuration
  - Fault injection with percentage values
  - Fault injection disabled via `disable` field

#### End-to-End Tests

- Test delay injection increases response latency
- Test HTTP abort returns configured status code
- Test gRPC abort returns configured status code
- Test percentage-based fault injection applies to approximate fraction of requests
- Test response rate limiting reduces throughput
- Test fault injection can be disabled

## Alternatives

### Alternative 1: Standalone FaultInjectionPolicy CRD

Create a separate FaultInjectionPolicy CRD instead of extending TrafficPolicy.

**Pros:** Cleaner separation of concerns, dedicated status reporting.

**Cons:** Duplicates the entire policy attachment and merge infrastructure. Adds another CRD for users to manage. All other traffic manipulation features live in TrafficPolicy already.

**Decision:** Extend TrafficPolicy for consistency with existing patterns.

### Alternative 2: Expose All Envoy Fault Filter Fields

Expose every field from `HTTPFault` including runtime key overrides, header matchers, downstream node filtering, and upstream cluster filtering.

**Pros:** Full Envoy feature coverage.

**Cons:** Many of these fields are Envoy-internal operational knobs. Runtime keys are not Kubernetes-native. Header matchers duplicate Gateway API HTTPRoute matching. The API surface becomes unnecessarily complex.

**Decision:** Expose the core user-facing fields (delay, abort, response rate limit, max active faults, percentage). Advanced Envoy-specific fields can be added later if there is demand.

## Open Questions

- Should header controlled dynamic fault values (e.g., `x-envoy-fault-delay-request`) be supported in a follow up? This would allow callers to dynamically control fault behavior per-request without configuration changes.
