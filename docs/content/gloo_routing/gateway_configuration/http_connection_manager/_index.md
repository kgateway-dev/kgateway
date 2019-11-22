---
title: HTTP Connection Manager
weight: 2
---

The HTTP Connection Manager lets you refine the behavior of Envoy for each listener that you manage with Gloo.

### Tracing

One of the fields in the HTTP Connection Manager Plugin is `tracing`. This specifies the listener-specific tracing configuration.

For notes on configuring and using tracing with Gloo, please see the [tracing setup docs.](../../../observability/tracing/)

The tracing configuration fields of the Gateway CRD are highlighted below.

{{< highlight yaml "hl_lines=8-15" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
spec:
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    options:
      httpConnectionManagerSettings:
        tracing:
          verbose: true
          requestHeadersForTags:
            - path
            - origin
status: # collapsed for brevity
{{< /highlight >}}

### Advanced listener configuration

Gloo exposes Envoy's powerful configuration capabilities with the HTTP Connection Manager. The details of these fields can be found [here](https://www.envoyproxy.io/docs/envoy/v1.9.0/configuration/http_conn_man/http_conn_man) and [here](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/core/protocol.proto#envoy-api-msg-core-http1protocoloptions)

Below, see a reference configuration specification to demonstrate the structure of the expected yaml.

{{< highlight yaml "hl_lines=7-24" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    options:
      httpConnectionManagerSettings:
        skipXffAppend: false
        via: reference-string
        xffNumTrustedHops: 1234
        useRemoteAddress: false
        generateRequestId: false
        proxy100Continue: false
        streamIdleTimeout: 1m2s
        idleTimeout: 1m2s
        maxRequestHeadersKb: 1234
        requestTimeout: 1m2s
        drainTimeout: 1m2s
        delayedCloseTimeout: 1m2s
        serverName: reference-string
        acceptHttp10: false
        defaultHostForHttp10: reference-string
status: # collapsed for brevity
{{< /highlight >}}


