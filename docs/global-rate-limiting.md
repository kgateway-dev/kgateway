# Global Rate Limiting in kgateway

Global rate limiting allows you to apply distributed, consistent rate limits across multiple instances of your gateway. Unlike local rate limiting, which operates independently on each gateway instance, global rate limiting uses a central service to coordinate rate limits.

## Overview

Global rate limiting in kgateway is powered by [Envoy's rate limiting service protocol](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rate_limit_filter) and delegates rate limit decisions to an external service that implements this protocol. This approach provides several benefits:

- **Coordinated rate limiting** across multiple gateway instances
- **Centralized rate limit management** with shared counters
- **Dynamic descriptor-based rate limits** that can consider multiple request attributes
- **Consistent user experience** regardless of which gateway instance receives the request

## How It Works

1. When a request arrives at kgateway, the gateway extracts descriptor information based on your policy configuration
2. kgateway sends these descriptors to your rate limit service
3. The rate limit service applies configured limits for those descriptors and returns a decision
4. kgateway either allows or denies the request based on the service's decision

## Architecture

The global rate limiting feature consists of three components:

1. **TrafficPolicy with rateLimit.global** - Configures which descriptors to extract and send to the rate limit service
2. **GatewayExtension** - References the rate limit service implementation
3. **Rate Limit Service** - An external service that implements the Envoy Rate Limit protocol and contains the actual rate limit values

## Deployment

### 1. Deploy the Rate Limit Service

kgateway integrates with any service that implements the Envoy Rate Limit gRPC protocol. For your convenience, we provide an example deployment using the official Envoy rate limit service in the [test/kubernetes/e2e/features/rate_limit/testdata](../test/kubernetes/e2e/features/rate_limit/testdata) directory.

```bash
kubectl apply -f test/kubernetes/e2e/features/rate_limit/testdata/rate-limit-server.yaml
```

### 2. Configure the Rate Limit Service

The actual rate limit values (requests per unit time) must be configured in the rate limit service, not in kgateway's policies. For example, using the [Envoy Rate Limit](https://github.com/envoyproxy/ratelimit) service, you would configure limits in its configuration file:

```yaml
# ratelimit-config.yaml
domain: api-gateway
descriptors:
  - key: remote_address
    rate_limit:
      unit: minute
      requests_per_unit: 1
  - key: path
    value: /path1
    rate_limit:
      unit: minute
      requests_per_unit: 1
  - key: X-User-ID
    rate_limit:
      unit: minute
      requests_per_unit: 1
  - key: service
    value: premium-api
    rate_limit:
      unit: minute
      requests_per_unit: 2
```

### 3. Create a GatewayExtension

The GatewayExtension resource connects your kgateway installation with the rate limit service:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayExtension
metadata:
  name: global-ratelimit
  namespace: kgateway-system
spec:
  type: RateLimit
  rateLimit:
    grpcService:
      backendRef:
        name: ratelimit
        namespace: kgateway-system
        port: 8081
    domain: "api-gateway"
    timeout: "100ms"  # Optional timeout for rate limit service calls
    failOpen: false   # Optional: when true, requests proceed if the rate limit service is unavailable
```

### 4. Create TrafficPolicies with Global Rate Limiting

Apply rate limits to your routes using the TrafficPolicy resource:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: ip-rate-limit
  namespace: kgateway-system
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: test-route-1
  rateLimit:
    global:
      descriptors:
      - entries:
        - type: RemoteAddress
      extensionRef:
        name: global-ratelimit
```

## Configuration Options

### TrafficPolicy.spec.rateLimit.global

| Field | Description | Required |
|-------|-------------|----------|
| descriptors | Define the dimensions for rate limiting | Yes |
| extensionRef | Reference to a GatewayExtension for the rate limit service | Yes |

### GatewayExtension.spec.rateLimit

| Field | Description | Required |
|-------|-------------|----------|
| grpcService | Configuration for connecting to the gRPC rate limit service | Yes |
| domain | Domain identity for the rate limit service | Yes |
| timeout | Timeout for rate limit service calls (e.g., "100ms") | No |
| failOpen | When true, requests continue if the rate limit service is unavailable | No (defaults to false) |

### Rate Limit Descriptors

Descriptors define the dimensions for rate limiting. Each descriptor consists of one or more entries that help categorize and count requests:

```yaml
descriptors:
- entries:
  - type: RemoteAddress
  - type: Generic
    generic:
      key: "custom_key"
      value: "custom_value"
- entries:
  - type: Header
    header: "X-User-ID"
  - type: Path
```

### Descriptor Entry Types

| Type | Description | Additional Fields |
|------|-------------|-------------------|
| RemoteAddress | Uses the client's IP address as the descriptor value | None |
| Path | Uses the request path as the descriptor value | None |
| Header | Extracts the descriptor value from a request header | `header`: The name of the header to extract |
| Generic | Uses a static key-value pair | `generic.key`: The descriptor key<br>`generic.value`: The static value |

## Rate Limit Response Headers

When rate limiting is enabled, kgateway adds the following headers to responses:

| Header | Description | Example |
|--------|-------------|---------|
| x-ratelimit-limit | The rate limit ceiling for the given request | `10, 10;w=60` (10 requests per 60 seconds) |
| x-ratelimit-remaining | The number of requests left for the time window | `5` (5 requests remaining) |
| x-ratelimit-reset | The time in seconds until the rate limit resets | `30` (rate limit resets in 30 seconds) |
| x-envoy-ratelimited | Present when the request is rate limited | `true` |

These headers help clients understand their current rate limit status and adapt their behavior accordingly.

## Examples

### Rate Limiting by IP Address

Limit requests based on the client's IP address:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: ip-rate-limit
  namespace: kgateway-system
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: test-route-1
  rateLimit:
    global:
      descriptors:
      - entries:
        - type: RemoteAddress
      extensionRef:
        name: global-ratelimit
```

### Rate Limiting by User ID (from header)

Limit requests based on a user ID header:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: user-rate-limit
  namespace: kgateway-system
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: test-route-1
  rateLimit:
    global:
      descriptors:
      - entries:
        - type: Header
          header: "X-User-ID"
      extensionRef:
        name: global-ratelimit
```

### Combined Rate Limiting

Apply different limits based on both path and user ID:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: combined-rate-limit
  namespace: kgateway-system
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: test-route-1
  rateLimit:
    global:
      descriptors:
      - entries:
        - type: Path
        - type: Header
          header: "user-id"
      extensionRef:
        name: global-ratelimit
```

## Combining Local and Global Rate Limiting

kgateway allows you to use both local and global rate limiting in the same TrafficPolicy:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: combined-rate-limit
  namespace: kgateway-system
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: test-route-1
  rateLimit:
    local:
      tokenBucket:
        maxTokens: 5
        tokensPerFill: 1
        fillInterval: "1s"
    global:
      descriptors:
      - entries:
        - type: Generic
          generic:
            key: "service"
            value: "premium-api"
      extensionRef:
        name: global-ratelimit
```