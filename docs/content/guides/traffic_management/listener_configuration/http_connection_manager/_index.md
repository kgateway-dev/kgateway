---
title: HTTP Connection Manager
weight: 10
description: Refine the behavior of Envoy for each listener that you manage with Gloo Edge
---

The HTTP Connection Manager lets you refine the behavior of Envoy for each listener that you manage with Gloo Edge.

---

## Websocket

You can configure the Http Connection Manager on a listener to enable or disable websocket upgrades. See the [Websocket]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/websockets/" %}}) documentation for more. 

## Tracing

One of the fields in the HTTP Connection Manager Plugin is `tracing`. This specifies the listener-specific tracing configuration.

For documentation on configuring and using tracing with Gloo Edge, please see the [tracing setup docs.]({{% versioned_link_path fromRoot="/guides/observability/tracing/" %}})

A tracing upstream or cluster can be specified using `collectorUpstreamRef` or `clusterName` respectively. The tracing configuration fields of the Gateway Custom Resource (CR) are highlighted here:

{{< tabs >}}
{{< tab name="collectorUpstreamRef">}}

{{< highlight yaml "hl_lines=10-20" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
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
          zipkinConfig:
            collectorEndpoint: /api/v2/spans
            collectorEndpointVersion: HTTP_JSON
            collectorUpstreamRef:
              name: zipkin
              namespace: default
status: # collapsed for brevity
{{< /highlight >}}

{{< /tab >}}
{{< tab name="clusterName">}}
{{< highlight yaml "hl_lines=10-18" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
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
          zipkinConfig:
            collectorEndpoint: /api/v2/spans
            collectorEndpointVersion: HTTP_JSON
            clusterName: zipkin
status: # collapsed for brevity
{{< /highlight >}}

{{% notice note %}}
If you provide an invalid clusterName, the error will not show up in Gloo.
However, if you are using Gloo Edge Enterprise you can use our [observability]({{% versioned_link_path fromRoot="/guides/observability" %}}) features to track the `glooe.solo.io/xds/outofsync` statistic
{{% /notice %}}
{{< /tab >}}
{{< /tabs >}}

### Advanced listener configuration

Gloo Edge exposes Envoy's powerful configuration capabilities with the HTTP Connection Manager. The details of these fields can be found [here](https://www.envoyproxy.io/docs/envoy/v1.9.0/configuration/http_conn_man/http_conn_man) and [here](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#envoy-api-msg-core-http1protocoloptions)

Below, see a reference configuration specification to demonstrate the structure of the expected YAML.

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

We recommend that you consult the linked Envoy docs to gain a better understanding of the `httpGateway` options and how you might apply them in your environment.

---

## Next Steps

Two potential settings that might be of interest are the options governing the configuration of [gRPC Web clients]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/grpc_web/" %}}) and [Websockets]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/websockets/" %}}). Please check out the linked guides for more information on how to configure each of these options.