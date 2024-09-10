---
title: About external processing
weight: 40
description: Learn about what external processing is, how it works, and how to enable it in Gloo Gateway. 
---

Envoy offers multiple filters that you can use to manage, monitor, and secure traffic to your apps. Although Envoy is extensible via C++ and WebAssembly modules, it might not be practical to implement these extensions for all of your apps. You might also have very specific requirements for how to process a request or response to allow traffic routing between different types of apps, such as adding specific headers to new and legacy apps. 

With external processing, you can implement an external processing server that can read and modify all aspects of an HTTP request or response, such as headers, body, and trailers. This gives you the flexibility to apply your requirements to all types of apps, without the need to run WebAssembly or other custom scripts. 

{{% notice note %}}
External processing is an Enterprise-only feature. 
{{% /notice %}}

{{% notice warning %}}
Envoy's external processing filter is considered a work in progress and has an unknown security posture. Use caution when using this feature in production environments. For more information, see the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_proc_filter#external-processing).
{{% /notice %}}

### How it works

The following diagram shows an example for how request header manipulation works when an external processing server is used. 

<figure><img src="{{% versioned_link_path fromRoot="/img/extproc.svg" %}}">
<figcaption style="text-align:center;font-style:italic">External processing for request headers</figcaption></figure>

1. The downstream service sends a request with headers to the Envoy gateway. 
2. The gateway extracts the header information and sends it to the external processing server. 
3. The external processing server modifies, adds, or removes the request headers. 
4. The modified request headers are sent back to the gateway. 
5. The modified headers are added to the request.
6. The request is forwarded to the upstream application. 

## ExtProc server considerations

The ExtProc server is a gRPC interface that must be able to respond to events in the lifecycle of an HTTP request. When the ExtProc filter is enabled in Gloo Gateway and a request or response is received on the gateway, the filter communicates with the ExtProc server by using bidirectional gRPC streams.

To implement your own ExtProc server, make sure that you follow [Envoy's technical specification for an external processor](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_proc/v3/ext_proc.proto#extensions-filters-http-ext-proc-v3-externalprocessor). You can also follow the [Header manipulation example]({{% versioned_link_path fromRoot="/guides/traffic_management/extproc/header-manipulation/" %}}) to try out ExtProc in Gloo Gateway with a sample ExtProc server.

{{% notice warning %}}
In Gloo Gateway version 1.17.0, the Gloo Gateway extProc filter implementation was changed to comply with the latest extProc implementation in Envoy. Previously, request and response attributes were included only in a [header processing request](https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/ext_proc/v3/external_processor.proto#service-ext-proc-v3-httpheaders), and were therefore sent to the extProc server only when request header processing messages were configured to be sent. Starting in Gloo Gateway version 1.17.0, the Gloo extProc filter sends request and response attributes as part of the top-level [processing request](https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/ext_proc/v3/external_processor.proto#service-ext-proc-v3-processingrequest). That way, attributes can be processed on the first processing request regardless of its type.  </br></br>

If you implemented your extProc server to expect request and response attributes as part of the HTTP header processing request, you must change this implementation to read attributes from the top-level processing request instead. </br></br>

For more information, see the [extProc proto definition](https://github.com/envoyproxy/envoy/blob/main/api/envoy/service/ext_proc/v3/external_processor.proto) in Envoy. 
{{% /notice %}}

## Enable ExtProc in Gloo Gateway

You can enable ExtProc for all requests and responses that the gateway processes by using the [Settings]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/settings.proto.sk/" %}}) custom resource. Alternatively, you can enable ExtProc for a specific gateway listener, virtual host, or route. 

Example configuration to add to the `default` Settings resource: 

```yaml
extProc: 
  grpcService: 
    extProcServerRef: 
      name: default-ext-proc-grpc-4444
      namespace: gloo-system
  filterStage: 
    stage: AuthZStage
    predicate: After
  failureModeAllow: false
  allowModeOverride: false
  processingMode: 
    requestHeaderMode: SEND
    responseHeaderMode: SKIP
```

