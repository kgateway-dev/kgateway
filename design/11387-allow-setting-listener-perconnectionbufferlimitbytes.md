# EP-11387: Allow setting listener perConnectionBufferLimitBytes

* Issue: [#11387](https://github.com/kgateway-dev/kgateway/issues/11387)

## Background

We want to allow users to set the [perConnectionBufferLimitBytes](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/listener/v3/listener.proto#envoy-v3-api-field-config-listener-v3-listener-per-connection-buffer-limit-bytes). The listener buffer limit, defined in envoy as `per_connection_buffer_limit_bytes
`, controls the max size of read and write buffers for new connections.

## Motivation

When using Envoy as an edge proxy, configuring the listener buffer limit is important, since you could be dealing with untrusted downstreams. By setting the limit to a small number, such as 32KiB, you better guard against potential attacks or misconfigured downstreams that could hog the proxy's resources.

## Goals

Allow setting the listener level buffer limit (perConnectionBufferLimitBytes) for a Gateway.

## Non-Goals

Allow setting the perConnectionBufferLimitBytes for each individual listener on a Gateway (instead, we will apply same limit to all listeners).

## Implementation Details

We'll check the gateway for the annotation `kgateway.dev/per-connection-buffer-limit`, which should specify a value in the [resource quantity format](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/quantity/).

The perConnectionBufferLimitBytes for all listeners on the gateway will be set to this value.

```
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: example-gateway
  annotations:
    kgateway.dev/per-connection-buffer-limit: 64Ki
spec:
  gatewayClassName: example-gateway-class
  listeners:
  - name: http
    protocol: HTTP
    port: 80
  - name: http2
    protocol: HTTP
    port: 3000
```

### Test Plan

unit tests

## Alternatives

We discussed several options for setting perConnectionBufferLimitBytes.
- Creating new policy for listener options
  -  decided against this since it was overkill to create a new policy for one field, and we're unlikely to have other listener level fields even in future
- adding this option to GatewayParameters
  - while this makes sense, it will require a lot of up front work to refactor it out of deployer and into krt collections
  
We decided using an annotation on the gateway made sense and was straightforward to implement.

## Open Questions

Need to make sure this is documented 
