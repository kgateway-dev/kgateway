# HTTPListenerPolicy Additional Envoy Fields

This document describes the additional Envoy HTTPConnectionManager fields that have been added to the HTTPListenerPolicy API.

## Overview

The HTTPListenerPolicy now supports four additional Envoy HTTPConnectionManager configuration fields that were previously missing from the v1.x API:

- `useRemoteAddress`
- `xffNumTrustedHops`
- `serverHeaderTransformation`
- `streamIdleTimeout`

These fields allow fine-grained control over Envoy's HTTP connection management behavior.

## Field Descriptions

### useRemoteAddress

**Type:** `*bool` (optional)

**Description:** Determines whether to use the remote address for the original client. When `true`, Envoy will use the remote address of the connection as the client address. When `false`, Envoy will use the X-Forwarded-For header to determine the client address.

**Default:** `false` (uses X-Forwarded-For header)

**Example:**
```yaml
spec:
  useRemoteAddress: true
```

### xffNumTrustedHops

**Type:** `*uint32` (optional)

**Description:** The number of additional ingress proxy hops from the right side of the X-Forwarded-For HTTP header to trust when determining the origin client's IP address.

**Validation:** Minimum value of 0

**Example:**
```yaml
spec:
  xffNumTrustedHops: 2
```

### serverHeaderTransformation

**Type:** `*ServerHeaderTransformation` (optional)

**Description:** Determines how the server header is transformed in HTTP responses.

**Values:**
- `OVERWRITE`: Overwrites the server header
- `APPEND_IF_ABSENT`: Appends to the server header if it's not present
- `PASS_THROUGH`: Passes through the server header unchanged

**Example:**
```yaml
spec:
  serverHeaderTransformation: OVERWRITE
```

### streamIdleTimeout

**Type:** `*metav1.Duration` (optional)

**Description:** The idle timeout for HTTP streams. This controls how long Envoy will wait for activity on an HTTP stream before timing it out.

**Example:**
```yaml
spec:
  streamIdleTimeout: 30s
```

## Complete Example

Here's a complete example showing all the new fields:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: HTTPListenerPolicy
metadata:
  name: http-listener-policy-example
  namespace: default
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: my-gateway
  # Use remote address for client identification
  useRemoteAddress: true
  # Trust 2 additional proxy hops in X-Forwarded-For
  xffNumTrustedHops: 2
  # Overwrite server headers
  serverHeaderTransformation: OVERWRITE
  # Set 30-second idle timeout
  streamIdleTimeout: 30s
```

## Envoy Configuration

These fields are translated to the corresponding Envoy HTTPConnectionManager configuration:

- `useRemoteAddress` → `use_remote_address`
- `xffNumTrustedHops` → `xff_num_trusted_hops`
- `serverHeaderTransformation` → `server_header_transformation`
- `streamIdleTimeout` → `stream_idle_timeout`

## References

For more information about these Envoy fields, see the [Envoy HTTPConnectionManager documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto). 